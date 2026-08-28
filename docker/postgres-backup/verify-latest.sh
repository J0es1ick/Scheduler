#!/bin/sh
set -eu

: "${DATABASE_HOST:=postgres}"
: "${DATABASE_PORT:=5432}"
: "${DATABASE_USER:=postgres}"
: "${DATABASE_NAME:=scheduler}"
: "${DATABASE_SSLMODE:=prefer}"
: "${MIGRATIONS_PATH:=/app/migrations}"
: "${BACKUP_VERIFY_OFFSITE:=false}"
: "${BACKUP_OFFSITE_DIRECTORY:=}"
: "${BACKUP_AGE_IDENTITY_FILE:=}"
: "${BACKUP_ENCRYPTION_PASSPHRASE_FILE:=}"
: "${BACKUP_ALLOW_LEGACY_AES_CBC:=false}"
export PGPASSWORD="${DATABASE_PASSWORD:?DATABASE_PASSWORD is required}"
export PGSSLMODE="$DATABASE_SSLMODE"

decrypted_backup=""
cleanup() {
  if [ -n "${verify_db:-}" ]; then
    dropdb --if-exists --force -h "$DATABASE_HOST" -p "$DATABASE_PORT" -U "$DATABASE_USER" "$verify_db" >/dev/null 2>&1 || true
  fi
  if [ -n "$decrypted_backup" ]; then
    rm -f "$decrypted_backup"
  fi
}
trap cleanup EXIT INT TERM

if [ "$BACKUP_VERIFY_OFFSITE" = "true" ]; then
  if [ ! -d "$BACKUP_OFFSITE_DIRECTORY" ]; then
    echo "off-host backup destination is unavailable" >&2
    exit 1
  fi
  encrypted="$(find "$BACKUP_OFFSITE_DIRECTORY" -maxdepth 1 -type f -name 'scheduler-*.dump.age' -print | sort | tail -n 1)"
  legacy=false
  if [ -z "$encrypted" ]; then
    case "$BACKUP_ALLOW_LEGACY_AES_CBC" in
      true)
        encrypted="$(find "$BACKUP_OFFSITE_DIRECTORY" -maxdepth 1 -type f -name 'scheduler-*.dump.enc' -print | sort | tail -n 1)"
        legacy=true
        ;;
      false) ;;
      *)
        echo "BACKUP_ALLOW_LEGACY_AES_CBC must be true or false" >&2
        exit 1
        ;;
    esac
  fi
  if [ -z "$encrypted" ] || [ ! -f "${encrypted}.sha256" ]; then
    echo "no complete encrypted off-host backup found" >&2
    exit 1
  fi
  (cd "$BACKUP_OFFSITE_DIRECTORY" && sha256sum -c "$(basename "${encrypted}.sha256")") || exit 1
  decrypted_backup="/tmp/offsite-restore-verify-$$.dump"
  if [ "$legacy" = "true" ]; then
    if [ ! -s "$BACKUP_ENCRYPTION_PASSPHRASE_FILE" ]; then
      echo "legacy backup passphrase is unavailable" >&2
      exit 1
    fi
    echo "warning: decrypting a legacy unauthenticated AES-CBC backup" >&2
    openssl enc -d -aes-256-cbc -pbkdf2 \
      -pass "file:${BACKUP_ENCRYPTION_PASSPHRASE_FILE}" \
      -in "$encrypted" -out "$decrypted_backup" || exit 1
  else
    if [ ! -s "$BACKUP_AGE_IDENTITY_FILE" ]; then
      echo "age identity is unavailable" >&2
      exit 1
    fi
    age --decrypt --identity "$BACKUP_AGE_IDENTITY_FILE" --output "$decrypted_backup" "$encrypted" || exit 1
  fi
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
