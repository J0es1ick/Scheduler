$ErrorActionPreference = "Stop"

docker compose exec -T backup /usr/local/bin/scheduler-verify-backup
if ($LASTEXITCODE -ne 0) {
    throw "Backup restore verification failed"
}
