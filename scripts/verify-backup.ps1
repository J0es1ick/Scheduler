$ErrorActionPreference = "Stop"

$envPath = Join-Path (Resolve-Path (Join-Path $PSScriptRoot "..")).Path ".env"
if (-not (Test-Path -LiteralPath $envPath)) {
    throw ".env was not found"
}
$values = @{}
foreach ($line in Get-Content -LiteralPath $envPath -Encoding UTF8) {
    if ($line -match '^([A-Z0-9_]+)=(.*)$') {
        $values[$Matches[1]] = $Matches[2]
    }
}
$restoreUser = if ($values.DATABASE_RESTORE_USER) { $values.DATABASE_RESTORE_USER } else { "scheduler_restore" }
$restorePassword = $values.DATABASE_RESTORE_PASSWORD
if (-not $restorePassword) {
    throw "DATABASE_RESTORE_PASSWORD is missing"
}

$previousUser = $env:DATABASE_USER
$previousPassword = $env:DATABASE_PASSWORD
try {
    $env:DATABASE_USER = $restoreUser
    $env:DATABASE_PASSWORD = $restorePassword
    docker compose exec -T `
        -e DATABASE_USER `
        -e DATABASE_PASSWORD `
        backup /usr/local/bin/scheduler-verify-backup
    if ($LASTEXITCODE -ne 0) {
        throw "Backup restore verification failed"
    }
} finally {
    $env:DATABASE_USER = $previousUser
    $env:DATABASE_PASSWORD = $previousPassword
}
