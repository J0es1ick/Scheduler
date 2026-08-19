#!/bin/sh
set -eu

: "${DATABASE_HOST:=postgres}"
: "${DATABASE_PORT:=5432}"
: "${DATABASE_USER:=postgres}"
: "${DATABASE_NAME:=scheduler}"
: "${MIGRATIONS_PATH:=/app/migrations}"
: "${BACKUP_VERIFY_OFFSITE:=false}"
: "${BACKUP_OFFSITE_DIRECTORY:=}"
: "${BACKUP_ENCRYPTION_PASSPHRASE_FILE:=}"
export PGPASSWORD="${DATABASE_PASSWORD:?DATABASE_PASSWORD is required}"

decrypted_backup=""
if [ "$BACKUP_VERIFY_OFFSITE" = "true" ]; then
  if [ ! -d "$BACKUP_OFFSITE_DIRECTORY" ] || [ ! -r "$BACKUP_ENCRYPTION_PASSPHRASE_FILE" ]; then
    echo "off-host backup destination or encryption secret is unavailable" >&2
    exit 1
  fi
  encrypted="$(find "$BACKUP_OFFSITE_DIRECTORY" -maxdepth 1 -type f -name 'scheduler-*.dump.enc' -print | sort | tail -n 1)"
  if [ -z "$encrypted" ] || [ ! -f "${encrypted}.sha256" ]; then
    echo "no complete encrypted off-host backup found" >&2
    exit 1
  fi
  (cd "$BACKUP_OFFSITE_DIRECTORY" && sha256sum -c "$(basename "${encrypted}.sha256")") || exit 1
  decrypted_backup="/tmp/offsite-restore-verify-$$.dump"
  openssl enc -d -aes-256-cbc -pbkdf2 \
    -pass "file:${BACKUP_ENCRYPTION_PASSPHRASE_FILE}" \
    -in "$encrypted" -out "$decrypted_backup" || exit 1
  backup="$decrypted_backup"
else
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
  (cd /backups && sha256sum -c "$(basename "$checksum")") || exit 1
fi

verify_db="${DATABASE_NAME}_restore_verify_$(date -u +%s)"
cleanup() {
  dropdb --if-exists --force -h "$DATABASE_HOST" -p "$DATABASE_PORT" -U "$DATABASE_USER" "$verify_db" >/dev/null 2>&1 || true
  if [ -n "$decrypted_backup" ]; then
    rm -f "$decrypted_backup"
  fi
}
trap cleanup EXIT INT TERM

createdb -h "$DATABASE_HOST" -p "$DATABASE_PORT" -U "$DATABASE_USER" "$verify_db"
pg_restore \
  --exit-on-error \
  --no-owner \
  --no-privileges \
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

expected=/tmp/expected-migrations
restored=/tmp/restored-migrations
: > "$expected"
for migration in "$MIGRATIONS_PATH"/*.up.sql; do
  name="$(basename "$migration")"
  digest="$(sha256sum "$migration" | awk '{print $1}')"
  printf '%s|%s\n' "$name" "$digest" >> "$expected"
done
has_migration_checksums="$(psql -h "$DATABASE_HOST" -p "$DATABASE_PORT" -U "$DATABASE_USER" -d "$verify_db" -tAc \
  "SELECT EXISTS (
     SELECT 1
     FROM information_schema.columns
     WHERE table_schema=current_schema()
       AND table_name='schema_migrations'
       AND column_name='checksum'
   )")"
if [ "$has_migration_checksums" = "t" ]; then
  psql -h "$DATABASE_HOST" -p "$DATABASE_PORT" -U "$DATABASE_USER" -d "$verify_db" -tA \
    -F '|' -c 'SELECT name, checksum FROM schema_migrations ORDER BY name' > "$restored"
else
  expected_names=/tmp/expected-migration-names
  cut -d '|' -f 1 "$expected" > "$expected_names"
  psql -h "$DATABASE_HOST" -p "$DATABASE_PORT" -U "$DATABASE_USER" -d "$verify_db" -tA \
    -c 'SELECT name FROM schema_migrations ORDER BY name' > "$restored"
  expected="$expected_names"
fi
if ! cmp -s "$expected" "$restored"; then
  echo "restored schema does not match the migrations shipped with this release" >&2
  diff -u "$expected" "$restored" >&2 || true
  exit 1
fi

echo "restore verification completed: $(basename "$backup"), migrations=$migrations"
