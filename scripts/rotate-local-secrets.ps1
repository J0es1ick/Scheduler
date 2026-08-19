param(
    [switch]$RotateDatabasePassword,
    [switch]$EnableLocalAccess
)

$ErrorActionPreference = "Stop"
$envPath = Join-Path (Resolve-Path (Join-Path $PSScriptRoot "..")).Path ".env"
if (-not (Test-Path -LiteralPath $envPath)) {
    throw ".env was not found. Copy .env.example first."
}

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

$lines = [System.Collections.Generic.List[string]](Get-Content -LiteralPath $envPath -Encoding UTF8)
$current = Read-Environment $lines
$newAdminToken = New-Secret 36
$newMetricsToken = New-Secret 36

$databaseRoles = @(
    @{ UserKey = "DATABASE_MIGRATOR_USER"; PasswordKey = "DATABASE_MIGRATOR_PASSWORD"; DefaultUser = "scheduler_migrator" },
    @{ UserKey = "DATABASE_BOT_USER"; PasswordKey = "DATABASE_BOT_PASSWORD"; DefaultUser = "scheduler_bot" },
    @{ UserKey = "DATABASE_ADMIN_USER"; PasswordKey = "DATABASE_ADMIN_PASSWORD"; DefaultUser = "scheduler_admin" },
    @{ UserKey = "DATABASE_SITE_USER"; PasswordKey = "DATABASE_SITE_PASSWORD"; DefaultUser = "scheduler_site" },
    @{ UserKey = "DATABASE_BACKUP_USER"; PasswordKey = "DATABASE_BACKUP_PASSWORD"; DefaultUser = "scheduler_backup" },
    @{ UserKey = "DATABASE_RESTORE_USER"; PasswordKey = "DATABASE_RESTORE_PASSWORD"; DefaultUser = "scheduler_restore" }
)

if (Test-Placeholder $current.POSTGRES_SUPERUSER) {
    $current.POSTGRES_SUPERUSER = if (-not (Test-Placeholder $current.DATABASE_USER)) { $current.DATABASE_USER } else { "postgres" }
    Set-EnvironmentValue $lines "POSTGRES_SUPERUSER" $current.POSTGRES_SUPERUSER
}
if (Test-Placeholder $current.POSTGRES_SUPERUSER_PASSWORD) {
    $current.POSTGRES_SUPERUSER_PASSWORD = if (-not (Test-Placeholder $current.DATABASE_PASSWORD)) { $current.DATABASE_PASSWORD } else { New-Secret 36 }
    Set-EnvironmentValue $lines "POSTGRES_SUPERUSER_PASSWORD" $current.POSTGRES_SUPERUSER_PASSWORD
}
foreach ($role in $databaseRoles) {
    if (-not $current[$role.UserKey]) {
        $current[$role.UserKey] = $role.DefaultUser
        Set-EnvironmentValue $lines $role.UserKey $role.DefaultUser
    }
    if (Test-Placeholder $current[$role.PasswordKey]) {
        $current[$role.PasswordKey] = New-Secret 36
        Set-EnvironmentValue $lines $role.PasswordKey $current[$role.PasswordKey]
    }
}
$current.DATABASE_USER = $current.DATABASE_BOT_USER
$current.DATABASE_PASSWORD = $current.DATABASE_BOT_PASSWORD
Set-EnvironmentValue $lines "DATABASE_USER" $current.DATABASE_USER
Set-EnvironmentValue $lines "DATABASE_PASSWORD" $current.DATABASE_PASSWORD

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
    $superuser = if ($current.POSTGRES_SUPERUSER) { $current.POSTGRES_SUPERUSER } else { "postgres" }
    $oldSuperuserPassword = $current.POSTGRES_SUPERUSER_PASSWORD
    if (-not $oldSuperuserPassword) {
        throw "POSTGRES_SUPERUSER_PASSWORD is missing"
    }

    $newBotDatabasePassword = $null
    foreach ($role in $databaseRoles) {
        $user = if ($current[$role.UserKey]) { $current[$role.UserKey] } else { $role.DefaultUser }
        $newPassword = New-Secret 36
        $escapedRoleLiteral = $user.Replace("'", "''")
        $roleExists = docker exec -e "PGPASSWORD=$oldSuperuserPassword" scheduler-postgres `
            psql -v ON_ERROR_STOP=1 -U $superuser -d $databaseName -tAc "SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname='$escapedRoleLiteral')"
        if ($LASTEXITCODE -ne 0) {
            throw "Could not inspect PostgreSQL role $user"
        }
        if ($roleExists.Trim() -eq "t") {
            $escapedUser = $user.Replace('"', '""')
            $escapedPassword = $newPassword.Replace("'", "''")
            $sql = "ALTER ROLE `"$escapedUser`" WITH PASSWORD '$escapedPassword';"
            $sql | docker exec -i -e "PGPASSWORD=$oldSuperuserPassword" scheduler-postgres psql -v ON_ERROR_STOP=1 -U $superuser -d $databaseName
            if ($LASTEXITCODE -ne 0) {
                throw "PostgreSQL rejected password rotation for $user"
            }
        }
        Set-EnvironmentValue $lines $role.PasswordKey $newPassword
        $current[$role.PasswordKey] = $newPassword
        if ($role.PasswordKey -eq "DATABASE_BOT_PASSWORD") {
            $newBotDatabasePassword = $newPassword
        }
    }

    $newSuperuserPassword = New-Secret 36
    $escapedSuperuser = $superuser.Replace('"', '""')
    $escapedSuperuserPassword = $newSuperuserPassword.Replace("'", "''")
    $sql = "ALTER ROLE `"$escapedSuperuser`" WITH PASSWORD '$escapedSuperuserPassword';"
    $sql | docker exec -i -e "PGPASSWORD=$oldSuperuserPassword" scheduler-postgres psql -v ON_ERROR_STOP=1 -U $superuser -d $databaseName
    if ($LASTEXITCODE -ne 0) {
        throw "PostgreSQL rejected the superuser password rotation"
    }
    Set-EnvironmentValue $lines "POSTGRES_SUPERUSER_PASSWORD" $newSuperuserPassword
    Set-EnvironmentValue $lines "DATABASE_USER" $current.DATABASE_BOT_USER
    Set-EnvironmentValue $lines "DATABASE_PASSWORD" $newBotDatabasePassword
}

$utf8 = New-Object Text.UTF8Encoding $false
[IO.File]::WriteAllLines($envPath, $lines, $utf8)
Write-Host "Prepared per-service database credentials and rotated local admin and metrics tokens$(if ($RotateDatabasePassword) { ', including passwords of existing PostgreSQL roles' })."
Write-Host "The Telegram token was intentionally left untouched; rotate it in BotFather and replace BOT_TOKEN separately."
