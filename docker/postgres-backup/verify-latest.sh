#!/bin/sh
set -eu

: "${DATABASE_HOST:=postgres}"
: "${DATABASE_PORT:=5432}"
: "${DATABASE_USER:=postgres}"
: "${DATABASE_NAME:=scheduler}"
export PGPASSWORD="${DATABASE_PASSWORD:?DATABASE_PASSWORD is required}"

backup="$(find /backups -maxdepth 1 -type f -name 'scheduler-*.dump' -print | sort | tail -n 1)"
if [ -z "$backup" ]; then
  echo "no backup found" >&2
  exit 1
fi

checksum="${backup}.sha256"
if [ ! -f "$checksum" ]; then
  echo "checksum is missing for $(basename "$backup")" >&2
  exit 1
fi
(cd /backups && sha256sum -c "$(basename "$checksum")")

verify_db="${DATABASE_NAME}_restore_verify_$(date -u +%s)"
cleanup() {
  dropdb --if-exists --force -h "$DATABASE_HOST" -p "$DATABASE_PORT" -U "$DATABASE_USER" "$verify_db" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

createdb -h "$DATABASE_HOST" -p "$DATABASE_PORT" -U "$DATABASE_USER" "$verify_db"
pg_restore \
  --exit-on-error \
  --no-owner \
  --host "$DATABASE_HOST" \
  --port "$DATABASE_PORT" \
  --username "$DATABASE_USER" \
  --dbname "$verify_db" \
  "$backup"

migrations="$(psql -h "$DATABASE_HOST" -p "$DATABASE_PORT" -U "$DATABASE_USER" -d "$verify_db" -tAc 'SELECT COUNT(*) FROM schema_migrations')"
users_table="$(psql -h "$DATABASE_HOST" -p "$DATABASE_PORT" -U "$DATABASE_USER" -d "$verify_db" -tAc "SELECT to_regclass('public.users') IS NOT NULL")"
if [ "${migrations:-0}" -lt 1 ] || [ "$users_table" != "t" ]; then
  echo "restored database failed validation" >&2
  exit 1
fi

echo "restore verification completed: $(basename "$backup"), migrations=$migrations"
