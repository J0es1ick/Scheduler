$ErrorActionPreference = "Stop"
$scriptPath = Join-Path $PSScriptRoot "rotate-local-secrets.ps1"
$tempRoot = Join-Path ([IO.Path]::GetTempPath()) ("scheduler-secret-test-" + [Guid]::NewGuid().ToString("N"))

function Read-TestEnvironment([string]$Path) {
    $values = @{}
    foreach ($line in Get-Content -LiteralPath $Path -Encoding UTF8) {
        if ($line -match '^([A-Z0-9_]+)=(.*)$') {
            $values[$Matches[1]] = $Matches[2]
        }
    }
    return $values
}

function Write-TestEnvironment(
    [string]$Path,
    [string]$SuperuserPassword,
    [string]$LegacyPassword,
    [hashtable]$Overrides = @{}
) {
    $values = [ordered]@{
        POSTGRES_SUPERUSER = "postgres"
        POSTGRES_SUPERUSER_PASSWORD = $SuperuserPassword
        DATABASE_USER = "scheduler_bot"
        DATABASE_PASSWORD = $LegacyPassword
        DATABASE_MIGRATOR_USER = "scheduler_migrator"
        DATABASE_MIGRATOR_PASSWORD = "migrator-database-password-123456"
        DATABASE_BOT_USER = "scheduler_bot"
        DATABASE_BOT_PASSWORD = $LegacyPassword
        DATABASE_ADMIN_USER = "scheduler_admin"
        DATABASE_ADMIN_PASSWORD = "admin-database-password-123456"
        DATABASE_PARSER_USER = "scheduler_parser"
        DATABASE_PARSER_PASSWORD = "parser-database-password-123456"
        DATABASE_PRIVACY_USER = "scheduler_privacy"
        DATABASE_PRIVACY_PASSWORD = "privacy-database-password-123456"
        DATABASE_SITE_USER = "scheduler_site"
        DATABASE_SITE_PASSWORD = "site-database-password-123456"
        DATABASE_BACKUP_USER = "scheduler_backup"
        DATABASE_BACKUP_PASSWORD = "backup-database-password-123456"
        DATABASE_RESTORE_USER = "scheduler_restore"
        DATABASE_RESTORE_PASSWORD = "restore-database-password-123456"
        ADMIN_ACCESS_TOKEN = "CHANGE_ME_TO_A_LONG_RANDOM_VALUE"
        ADMIN_METRICS_TOKEN = "CHANGE_ME_TO_A_SEPARATE_METRICS_TOKEN"
    }
    foreach ($key in $Overrides.Keys) {
        $values[$key] = $Overrides[$key]
    }
    $lines = foreach ($entry in $values.GetEnumerator()) {
        "$($entry.Key)=$($entry.Value)"
    }
    [IO.File]::WriteAllLines($path, $lines, (New-Object Text.UTF8Encoding $false))
}

function Assert-InitializedDatabaseCredentials([string]$Name, [hashtable]$Values) {
    $passwords = @(
        $Values.POSTGRES_SUPERUSER_PASSWORD,
        $Values.DATABASE_MIGRATOR_PASSWORD,
        $Values.DATABASE_BOT_PASSWORD,
        $Values.DATABASE_ADMIN_PASSWORD,
        $Values.DATABASE_PARSER_PASSWORD,
        $Values.DATABASE_PRIVACY_PASSWORD,
        $Values.DATABASE_SITE_PASSWORD,
        $Values.DATABASE_BACKUP_PASSWORD,
        $Values.DATABASE_RESTORE_PASSWORD
    )
    if (($passwords | Where-Object { $_.Length -lt 24 -or $_ -match '(?i)CHANGE_ME|PASTE_' }).Count -ne 0) {
        throw "$Name produced an invalid database password"
    }
    if (($passwords | Select-Object -Unique).Count -ne $passwords.Count) {
        throw "$Name produced duplicate database passwords"
    }
    if ($Values.DATABASE_USER -ne $Values.DATABASE_BOT_USER -or $Values.DATABASE_PASSWORD -ne $Values.DATABASE_BOT_PASSWORD) {
        throw "$Name did not synchronize compatibility database credentials"
    }
}

function Invoke-InitializeCase([string]$Name, [string]$SuperuserPassword, [string]$LegacyPassword) {
    $path = Join-Path $tempRoot ($Name + ".env")
    Write-TestEnvironment $path $SuperuserPassword $LegacyPassword
    & $scriptPath -EnvironmentPath $path -InitializeDatabaseCredentials
    $values = Read-TestEnvironment $path
    Assert-InitializedDatabaseCredentials $Name $values
    return $values
}

function Invoke-RejectedCase(
    [string]$Name,
    [hashtable]$Overrides = @{},
    [string]$SuperuserPassword = "superuser-password-123456789",
    [switch]$Initialize,
    [switch]$Rotate
) {
    $path = Join-Path $tempRoot ($Name + ".env")
    Write-TestEnvironment $path $SuperuserPassword "runtime-password-123456789" $Overrides
    $before = [IO.File]::ReadAllText($path)
    $rejected = $false
    try {
        if ($Initialize -and $Rotate) {
            & $scriptPath -EnvironmentPath $path -InitializeDatabaseCredentials -RotateDatabasePassword
        } elseif ($Initialize) {
            & $scriptPath -EnvironmentPath $path -InitializeDatabaseCredentials
        } elseif ($Rotate) {
            & $scriptPath -EnvironmentPath $path -RotateDatabasePassword
        } else {
            & $scriptPath -EnvironmentPath $path
        }
    } catch {
        $rejected = $true
    }
    if (-not $rejected) {
        throw "$Name accepted an invalid configuration"
    }
    if ([IO.File]::ReadAllText($path) -ne $before) {
        throw "$Name changed the environment before rejecting invalid roles"
    }
}

function Invoke-RotationOutcomeCase([string]$Mode) {
    $caseDirectory = Join-Path $tempRoot $Mode
    New-Item -ItemType Directory -Path $caseDirectory | Out-Null
    $path = Join-Path $caseDirectory '.env'
    Write-TestEnvironment $path "superuser-password-123456789" "runtime-password-123456789"
    $before = [IO.File]::ReadAllText($path)
    $mock = @{ Probes = 0; Committed = $false; Mode = $Mode }
    function docker {
        $global:LASTEXITCODE = 0
        if ($args[0] -eq 'ps') { return 'scheduler-postgres' }
        if ($args -contains '-i') {
            $sql = $input | Out-String
            if ($sql -notmatch 'COMMIT;') { throw 'Rotation did not send COMMIT' }
            $mock.Committed = $true
            if ($mock.Mode -eq 'exception-after-commit') { throw 'Connection lost after COMMIT' }
            $global:LASTEXITCODE = 1
            return
        }
        if ($args -contains 'SELECT 1') {
            $mock.Probes++
            if ($args -notcontains '-h' -or $args -notcontains '127.0.0.1') {
                throw 'Verification must use TCP password authentication'
            }
            if ($mock.Mode -eq 'unknown-outcome') {
                $global:LASTEXITCODE = 1
                return
            }
            return '1'
        }
        return 't'
    }
    $failure = $null
    try {
        & $scriptPath -EnvironmentPath $path -RotateDatabasePassword
    } catch {
        $failure = $_
    }
    $pending = @(Get-ChildItem -LiteralPath $caseDirectory -Force -Filter '.scheduler-env-rotation-*.tmp')
    if (-not $mock.Committed -or $mock.Probes -eq 0) {
        throw "$Mode did not exercise post-COMMIT verification"
    }
    if ($Mode -eq 'unknown-outcome') {
        if ($null -eq $failure -or $failure.Exception.Message -notmatch 'outcome is unknown') {
            throw 'Uncertain rotation was not reported correctly'
        }
        if ([IO.File]::ReadAllText($path) -ne $before -or $pending.Count -ne 1) {
            throw 'Uncertain rotation changed the old environment or deleted recovery credentials'
        }
        $recovery = Read-TestEnvironment $pending[0].FullName
        Assert-InitializedDatabaseCredentials $Mode $recovery
        if ($failure.Exception.Message -notlike "*$($pending[0].FullName)*") {
            throw 'Uncertain rotation did not report the recovery path'
        }
    } else {
        if ($null -ne $failure -or $mock.Probes -ne 9 -or $pending.Count -ne 0) {
            throw "$Mode did not verify and install all credentials: $failure"
        }
        Assert-InitializedDatabaseCredentials $Mode (Read-TestEnvironment $path)
        if ([IO.File]::ReadAllText($path) -eq $before) {
            throw 'Confirmed post-COMMIT rotation did not install the new environment'
        }
    }
}

function Invoke-DefaultPreservationCase {
    $path = Join-Path $tempRoot "default-preserves-database.env"
    Write-TestEnvironment $path "superuser-password-123456789" "runtime-password-123456789"
    $before = Read-TestEnvironment $path
    & $scriptPath -EnvironmentPath $path
    $after = Read-TestEnvironment $path
    $databaseKeys = @(
        "POSTGRES_SUPERUSER", "POSTGRES_SUPERUSER_PASSWORD", "DATABASE_USER", "DATABASE_PASSWORD",
        "DATABASE_MIGRATOR_USER", "DATABASE_MIGRATOR_PASSWORD", "DATABASE_BOT_USER", "DATABASE_BOT_PASSWORD",
        "DATABASE_ADMIN_USER", "DATABASE_ADMIN_PASSWORD", "DATABASE_PARSER_USER", "DATABASE_PARSER_PASSWORD",
        "DATABASE_PRIVACY_USER", "DATABASE_PRIVACY_PASSWORD", "DATABASE_SITE_USER", "DATABASE_SITE_PASSWORD",
        "DATABASE_BACKUP_USER", "DATABASE_BACKUP_PASSWORD", "DATABASE_RESTORE_USER", "DATABASE_RESTORE_PASSWORD"
    )
    foreach ($key in $databaseKeys) {
        if ($after[$key] -ne $before[$key]) {
            throw "default mode changed $key"
        }
    }
    if ($after.ADMIN_ACCESS_TOKEN -eq $before.ADMIN_ACCESS_TOKEN -or $after.ADMIN_METRICS_TOKEN -eq $before.ADMIN_METRICS_TOKEN) {
        throw "default mode did not rotate admin and metrics tokens"
    }
}

try {
    New-Item -ItemType Directory -Path $tempRoot | Out-Null
    Invoke-DefaultPreservationCase
    $legacy = "legacy-runtime-password-123456"
    $placeholder = Invoke-InitializeCase "placeholder-root" "CHANGE_ME_TO_A_POSTGRES_SUPERUSER_PASSWORD" $legacy
    if ($placeholder.POSTGRES_SUPERUSER_PASSWORD -eq $legacy) {
        throw "placeholder superuser password reused the runtime password"
    }
    $shared = "shared-database-password-123456"
    $duplicate = Invoke-InitializeCase "duplicate-root" $shared $shared
    if ($duplicate.POSTGRES_SUPERUSER_PASSWORD -ne $shared -or $duplicate.DATABASE_BOT_PASSWORD -eq $shared) {
        throw "existing superuser password was not kept distinct from the runtime password"
    }
    Invoke-RejectedCase "placeholder-superuser-name" @{ POSTGRES_SUPERUSER = "CHANGE_ME" }
    Invoke-RejectedCase "duplicate-runtime-role" @{ DATABASE_ADMIN_USER = "scheduler_bot" }
    Invoke-RejectedCase "runtime-equals-superuser" @{ DATABASE_BOT_USER = "postgres" }
    Invoke-RejectedCase "placeholder-runtime-default" @{ DATABASE_PARSER_PASSWORD = "CHANGE_ME_TO_A_PARSER_PASSWORD" }
    Invoke-RejectedCase "conflicting-modes" @{} -Initialize -Rotate
    Invoke-RotationOutcomeCase 'unknown-outcome'
    Invoke-RotationOutcomeCase 'lost-commit-response'
    Invoke-RotationOutcomeCase 'exception-after-commit'
    if ((Get-ChildItem -LiteralPath $tempRoot -Force -Filter ".scheduler-env-rotation-*").Count -ne 0) {
        throw "atomic environment replacement left temporary secret files"
    }
} finally {
    if (Test-Path -LiteralPath $tempRoot) {
        if (-not ([IO.Path]::GetFullPath($tempRoot)).StartsWith([IO.Path]::GetTempPath(), [StringComparison]::OrdinalIgnoreCase)) {
            throw 'Refusing to remove a directory outside the test temporary root'
        }
        Remove-Item -LiteralPath $tempRoot -Recurse -Force
    }
}

Write-Host "Local secret rotation tests passed."
