param(
    [switch]$RotateDatabasePassword,
    [switch]$InitializeDatabaseCredentials,
    [switch]$EnableLocalAccess,
    [string]$EnvironmentPath = ""
)

$ErrorActionPreference = "Stop"
if ($RotateDatabasePassword -and $InitializeDatabaseCredentials) {
    throw "InitializeDatabaseCredentials and RotateDatabasePassword cannot be used together"
}
$envPath = if ($EnvironmentPath) { $EnvironmentPath } else { Join-Path (Resolve-Path (Join-Path $PSScriptRoot "..")).Path ".env" }
if (-not (Test-Path -LiteralPath $envPath)) {
    throw ".env was not found. Copy .env.example first."
}
$envPath = (Resolve-Path -LiteralPath $envPath).Path

function New-Secret([int]$Bytes = 32) {
    $buffer = New-Object byte[] $Bytes
    $generator = [Security.Cryptography.RandomNumberGenerator]::Create()
    try {
        $generator.GetBytes($buffer)
    } finally {
        $generator.Dispose()
    }
    return [Convert]::ToBase64String($buffer).TrimEnd("=").Replace("+", "-").Replace("/", "_")
}

function Read-Environment([string[]]$Lines) {
    $values = @{}
    foreach ($line in $Lines) {
        if ($line -match '^([A-Z0-9_]+)=(.*)$') {
            $values[$Matches[1]] = $Matches[2]
        }
    }
    return $values
}

function Set-EnvironmentValue([System.Collections.Generic.List[string]]$Lines, [string]$Name, [string]$Value) {
    for ($index = 0; $index -lt $Lines.Count; $index++) {
        if ($Lines[$index] -match "^$([Regex]::Escape($Name))=") {
            $Lines[$index] = "$Name=$Value"
            return
        }
    }
    $Lines.Add("$Name=$Value")
}

function Test-Placeholder([string]$Value) {
    return [string]::IsNullOrWhiteSpace($Value) -or $Value -match '(?i)CHANGE_ME|PASTE_'
}

function Test-DatabaseSecret([string]$Value) {
    return -not (Test-Placeholder $Value) -and $Value.Length -ge 24
}

function Test-DatabaseRole([string]$Value) {
    return -not (Test-Placeholder $Value) -and $Value -match '^[A-Za-z_][A-Za-z0-9_]{0,62}$'
}

function Assert-DatabaseRole(
    [string]$SettingName,
    [string]$Value,
    [System.Collections.Generic.HashSet[string]]$UsedRoles
) {
    if (-not (Test-DatabaseRole $Value)) {
        throw "$SettingName must contain an explicit valid PostgreSQL role name"
    }
    if (-not $UsedRoles.Add($Value)) {
        throw "$SettingName must be distinct from the superuser and every other runtime role"
    }
}

function Assert-ExistingDatabaseCredentials([hashtable]$Values, [object[]]$Roles) {
    $usedSecrets = [System.Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
    $credentials = @(@{ Name = "POSTGRES_SUPERUSER_PASSWORD"; Value = $Values.POSTGRES_SUPERUSER_PASSWORD })
    foreach ($role in $Roles) {
        $credentials += @{ Name = $role.PasswordKey; Value = $Values[$role.PasswordKey] }
    }
    foreach ($credential in $credentials) {
        if (-not (Test-DatabaseSecret $credential.Value)) {
            throw "$($credential.Name) must contain an existing database secret of at least 24 characters"
        }
        if (-not $usedSecrets.Add($credential.Value)) {
            throw "$($credential.Name) must be distinct from every other database password"
        }
    }
    if (-not (Test-DatabaseRole $Values.DATABASE_USER)) {
        throw "DATABASE_USER must contain a valid PostgreSQL role name"
    }
    if (-not (Test-DatabaseSecret $Values.DATABASE_PASSWORD)) {
        throw "DATABASE_PASSWORD must contain an existing database secret of at least 24 characters"
    }
}

function New-UniqueSecret([System.Collections.Generic.HashSet[string]]$Used) {
    do {
        $candidate = New-Secret 36
    } while (-not $Used.Add($candidate))
    return $candidate
}

function New-PendingEnvironment(
    [string]$TargetPath,
    [System.Collections.Generic.List[string]]$Lines,
    [Text.Encoding]$Encoding
) {
    $directory = Split-Path -Parent $TargetPath
    $pending = Join-Path $directory (".scheduler-env-rotation-" + [Guid]::NewGuid().ToString("N") + ".tmp")
    [IO.File]::WriteAllLines($pending, $Lines, $Encoding)
    $stream = [IO.File]::Open($pending, [IO.FileMode]::Open, [IO.FileAccess]::ReadWrite, [IO.FileShare]::Read)
    try {
        $stream.Flush($true)
    } finally {
        $stream.Dispose()
    }
    return $pending
}

function Install-PendingEnvironment([string]$PendingPath, [string]$TargetPath) {
    $backup = Join-Path (Split-Path -Parent $TargetPath) (".scheduler-env-rotation-" + [Guid]::NewGuid().ToString("N") + ".bak")
    try {
        [IO.File]::Replace($PendingPath, $TargetPath, $backup, $true)
    } catch {
        throw "Could not atomically install the prepared environment. New credentials remain in $PendingPath. Do not rerun rotation until the file is recovered. $($_.Exception.Message)"
    }
    try {
        Remove-Item -LiteralPath $backup -Force
    } catch {
        throw "The new environment is installed, but the old secret backup remains in $backup and must be deleted securely. $($_.Exception.Message)"
    }
}

$lines = [System.Collections.Generic.List[string]](Get-Content -LiteralPath $envPath -Encoding UTF8)
$current = Read-Environment $lines
$original = Read-Environment $lines
$utf8 = New-Object Text.UTF8Encoding $false

$databaseRoles = @(
    @{ UserKey = "DATABASE_MIGRATOR_USER"; PasswordKey = "DATABASE_MIGRATOR_PASSWORD"; DefaultUser = "scheduler_migrator" },
    @{ UserKey = "DATABASE_BOT_USER"; PasswordKey = "DATABASE_BOT_PASSWORD"; DefaultUser = "scheduler_bot" },
    @{ UserKey = "DATABASE_ADMIN_USER"; PasswordKey = "DATABASE_ADMIN_PASSWORD"; DefaultUser = "scheduler_admin" },
    @{ UserKey = "DATABASE_PARSER_USER"; PasswordKey = "DATABASE_PARSER_PASSWORD"; DefaultUser = "scheduler_parser" },
    @{ UserKey = "DATABASE_PRIVACY_USER"; PasswordKey = "DATABASE_PRIVACY_PASSWORD"; DefaultUser = "scheduler_privacy" },
    @{ UserKey = "DATABASE_SITE_USER"; PasswordKey = "DATABASE_SITE_PASSWORD"; DefaultUser = "scheduler_site" },
    @{ UserKey = "DATABASE_BACKUP_USER"; PasswordKey = "DATABASE_BACKUP_PASSWORD"; DefaultUser = "scheduler_backup" },
    @{ UserKey = "DATABASE_RESTORE_USER"; PasswordKey = "DATABASE_RESTORE_PASSWORD"; DefaultUser = "scheduler_restore" }
)

$usedDatabaseRoles = [System.Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
[void]$usedDatabaseRoles.Add("scheduler_public_reader")
Assert-DatabaseRole "POSTGRES_SUPERUSER" $current.POSTGRES_SUPERUSER $usedDatabaseRoles
$resolvedDatabaseRoles = @{}
foreach ($role in $databaseRoles) {
    $roleName = $current[$role.UserKey]
    if ([string]::IsNullOrWhiteSpace($roleName) -and $InitializeDatabaseCredentials) {
        $roleName = $role.DefaultUser
    }
    Assert-DatabaseRole $role.UserKey $roleName $usedDatabaseRoles
    $resolvedDatabaseRoles[$role.UserKey] = $roleName
}
foreach ($role in $databaseRoles) {
    $current[$role.UserKey] = $resolvedDatabaseRoles[$role.UserKey]
    if ($InitializeDatabaseCredentials) {
        Set-EnvironmentValue $lines $role.UserKey $current[$role.UserKey]
    }
}

if ($InitializeDatabaseCredentials) {
    $usedDatabaseSecrets = [System.Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
    if (-not (Test-DatabaseSecret $current.POSTGRES_SUPERUSER_PASSWORD)) {
        $current.POSTGRES_SUPERUSER_PASSWORD = New-UniqueSecret $usedDatabaseSecrets
        Set-EnvironmentValue $lines "POSTGRES_SUPERUSER_PASSWORD" $current.POSTGRES_SUPERUSER_PASSWORD
    } else {
        [void]$usedDatabaseSecrets.Add($current.POSTGRES_SUPERUSER_PASSWORD)
    }
    foreach ($role in $databaseRoles) {
        if (-not (Test-DatabaseSecret $current[$role.PasswordKey]) -or -not $usedDatabaseSecrets.Add($current[$role.PasswordKey])) {
            $current[$role.PasswordKey] = New-UniqueSecret $usedDatabaseSecrets
            Set-EnvironmentValue $lines $role.PasswordKey $current[$role.PasswordKey]
        }
    }
    $current.DATABASE_USER = $current.DATABASE_BOT_USER
    $current.DATABASE_PASSWORD = $current.DATABASE_BOT_PASSWORD
    Set-EnvironmentValue $lines "DATABASE_USER" $current.DATABASE_USER
    Set-EnvironmentValue $lines "DATABASE_PASSWORD" $current.DATABASE_PASSWORD
} elseif ($RotateDatabasePassword) {
    if (-not (Test-DatabaseSecret $original.POSTGRES_SUPERUSER_PASSWORD)) {
        throw "POSTGRES_SUPERUSER_PASSWORD must contain the current database secret before rotation"
    }
} else {
    Assert-ExistingDatabaseCredentials $current $databaseRoles
}

$newAdminToken = New-Secret 36
$newMetricsToken = New-Secret 36
Set-EnvironmentValue $lines "ADMIN_ACCESS_TOKEN" $newAdminToken
Set-EnvironmentValue $lines "ADMIN_METRICS_TOKEN" $newMetricsToken
if ($EnableLocalAccess) {
    Set-EnvironmentValue $lines "ADMIN_ACCESS_LOGIN_ENABLED" "true"
    Set-EnvironmentValue $lines "ADMIN_COOKIE_SECURE" "false"
}

if ($RotateDatabasePassword) {
    $container = docker ps --filter "name=^/scheduler-postgres$" --format "{{.Names}}"
    if ($LASTEXITCODE -ne 0 -or $container -ne "scheduler-postgres") {
        throw "scheduler-postgres must be running before its password can be rotated"
    }
    $databaseName = if ($current.DATABASE_NAME) { $current.DATABASE_NAME } else { "scheduler" }
    $superuser = $current.POSTGRES_SUPERUSER
    $oldSuperuserPassword = $original.POSTGRES_SUPERUSER_PASSWORD
    if (-not $oldSuperuserPassword) {
        throw "POSTGRES_SUPERUSER_PASSWORD is missing"
    }

    $rotatedDatabaseSecrets = [System.Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
    $rotations = [System.Collections.Generic.List[object]]::new()
    foreach ($role in $databaseRoles) {
        $user = if ($current[$role.UserKey]) { $current[$role.UserKey] } else { $role.DefaultUser }
        $newPassword = New-UniqueSecret $rotatedDatabaseSecrets
        $escapedRoleLiteral = $user.Replace("'", "''")
        $roleExists = docker exec -e "PGPASSWORD=$oldSuperuserPassword" scheduler-postgres `
            psql -v ON_ERROR_STOP=1 -U $superuser -d $databaseName -tAc "SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname='$escapedRoleLiteral')"
        if ($LASTEXITCODE -ne 0) {
            throw "Could not inspect PostgreSQL role $user"
        }
        $rotations.Add([PSCustomObject]@{
            User = $user
            PasswordKey = $role.PasswordKey
            NewPassword = $newPassword
            Exists = $roleExists.Trim() -eq "t"
        })
    }

    $newSuperuserPassword = New-UniqueSecret $rotatedDatabaseSecrets
    foreach ($rotation in $rotations) {
        Set-EnvironmentValue $lines $rotation.PasswordKey $rotation.NewPassword
        $current[$rotation.PasswordKey] = $rotation.NewPassword
    }
    Set-EnvironmentValue $lines "POSTGRES_SUPERUSER_PASSWORD" $newSuperuserPassword
    Set-EnvironmentValue $lines "DATABASE_USER" $current.DATABASE_BOT_USER
    Set-EnvironmentValue $lines "DATABASE_PASSWORD" $current.DATABASE_BOT_PASSWORD
    $pendingEnvironmentPath = New-PendingEnvironment $envPath $lines $utf8

    $sqlStatements = [System.Collections.Generic.List[string]]::new()
    $sqlStatements.Add("BEGIN;")
    foreach ($rotation in $rotations) {
        if (-not $rotation.Exists) {
            continue
        }
        $escapedUser = $rotation.User.Replace('"', '""')
        $escapedPassword = $rotation.NewPassword.Replace("'", "''")
        $sqlStatements.Add("ALTER ROLE `"$escapedUser`" WITH PASSWORD '$escapedPassword';")
    }
    $escapedSuperuser = $superuser.Replace('"', '""')
    $escapedSuperuserPassword = $newSuperuserPassword.Replace("'", "''")
    $sqlStatements.Add("ALTER ROLE `"$escapedSuperuser`" WITH PASSWORD '$escapedSuperuserPassword';")
    $sqlStatements.Add("COMMIT;")
    ($sqlStatements -join [Environment]::NewLine) | docker exec -i -e "PGPASSWORD=$oldSuperuserPassword" scheduler-postgres psql -v ON_ERROR_STOP=1 -U $superuser -d $databaseName
    if ($LASTEXITCODE -ne 0) {
        Remove-Item -LiteralPath $pendingEnvironmentPath -Force -ErrorAction SilentlyContinue
        throw "PostgreSQL rejected password rotation; the transaction was rolled back"
    }
    Install-PendingEnvironment $pendingEnvironmentPath $envPath
} else {
    $pendingEnvironmentPath = New-PendingEnvironment $envPath $lines $utf8
    Install-PendingEnvironment $pendingEnvironmentPath $envPath
}

if ($InitializeDatabaseCredentials) {
    Write-Host "Initialized per-service database credentials and rotated local admin and metrics tokens."
} elseif ($RotateDatabasePassword) {
    Write-Host "Rotated existing PostgreSQL role passwords together with the environment, admin and metrics tokens."
} else {
    Write-Host "Rotated local admin and metrics tokens; database credentials were validated and left unchanged."
}
Write-Host "The Telegram token was intentionally left untouched; rotate it in BotFather and replace BOT_TOKEN separately."
if ($RotateDatabasePassword) {
    Write-Host "Recreate bot, parser-worker, privacy-worker, admin, site and backup immediately so they load the new credentials, then verify readiness."
}
