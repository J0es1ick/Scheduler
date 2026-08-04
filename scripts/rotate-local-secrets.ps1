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

$lines = [System.Collections.Generic.List[string]](Get-Content -LiteralPath $envPath -Encoding UTF8)
$current = Read-Environment $lines
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
    $databaseUser = if ($current.DATABASE_USER) { $current.DATABASE_USER } else { "postgres" }
    $databaseName = if ($current.DATABASE_NAME) { $current.DATABASE_NAME } else { "scheduler" }
    $oldPassword = $current.DATABASE_PASSWORD
    if (-not $oldPassword) {
        throw "DATABASE_PASSWORD is missing"
    }
    $newDatabasePassword = New-Secret 36
    $escapedUser = $databaseUser.Replace('"', '""')
    $escapedPassword = $newDatabasePassword.Replace("'", "''")
    $sql = "ALTER ROLE `"$escapedUser`" WITH PASSWORD '$escapedPassword';"
    $sql | docker exec -i -e "PGPASSWORD=$oldPassword" scheduler-postgres psql -v ON_ERROR_STOP=1 -U $databaseUser -d $databaseName
    if ($LASTEXITCODE -ne 0) {
        throw "PostgreSQL rejected the password rotation"
    }
    Set-EnvironmentValue $lines "DATABASE_PASSWORD" $newDatabasePassword
}

$utf8 = New-Object Text.UTF8Encoding $false
[IO.File]::WriteAllLines($envPath, $lines, $utf8)
Write-Host "Rotated local admin and metrics tokens$(if ($RotateDatabasePassword) { ', plus the PostgreSQL password' })."
Write-Host "The Telegram token was intentionally left untouched; rotate it in BotFather and replace BOT_TOKEN separately."
