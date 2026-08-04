#!/bin/sh
set -eu

: "${DATABASE_HOST:=postgres}"
: "${DATABASE_PORT:=5432}"
: "${DATABASE_USER:=postgres}"
: "${DATABASE_NAME:=scheduler}"
: "${BACKUP_INTERVAL_SECONDS:=86400}"
: "${BACKUP_RETENTION_DAYS:=14}"

export PGPASSWORD="${DATABASE_PASSWORD:?DATABASE_PASSWORD is required}"

until pg_isready -h "$DATABASE_HOST" -p "$DATABASE_PORT" -U "$DATABASE_USER" -d "$DATABASE_NAME" >/dev/null 2>&1; do
  sleep 2
done

while true; do
  timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
  target="/backups/scheduler-${timestamp}.dump"
  temporary="${target}.partial"

  if pg_dump \
    --host "$DATABASE_HOST" \
    --port "$DATABASE_PORT" \
    --username "$DATABASE_USER" \
    --dbname "$DATABASE_NAME" \
    --format custom \
    --compress 6 \
    --no-owner \
    --file "$temporary"; then
    mv "$temporary" "$target"
    sha256sum "$target" > "${target}.sha256"
    date -u +%s > /tmp/last-backup-success
    find /backups -type f \( -name 'scheduler-*.dump' -o -name 'scheduler-*.dump.sha256' \) -mtime "+${BACKUP_RETENTION_DAYS}" -delete
    echo "backup completed: $(basename "$target")"
  else
    rm -f "$temporary"
    echo "backup failed" >&2
  fi

  sleep "$BACKUP_INTERVAL_SECONDS"
done
