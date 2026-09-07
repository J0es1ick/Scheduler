#!/bin/sh
set -eu

: "${DATABASE_HOST:=postgres}"
: "${DATABASE_PORT:=5432}"
: "${DATABASE_USER:=scheduler_restore}"
: "${DATABASE_NAME:=scheduler}"
: "${DATABASE_SSLMODE:=prefer}"
: "${MIGRATIONS_PATH:=/app/migrations}"
: "${BACKUP_RESTORE_OFFSITE:=true}"
: "${BACKUP_OFFSITE_DIRECTORY:=}"
: "${BACKUP_OFFSITE_SENTINEL_FILE:=${BACKUP_OFFSITE_DIRECTORY}/.scheduler-offsite}"
: "${BACKUP_AGE_IDENTITY_FILE:=}"
: "${BACKUP_RESTORE_RECEIPT_FILE:=}"

target="${RESTORE_TARGET_DATABASE:?RESTORE_TARGET_DATABASE is required}"
case "$target" in
  ''|[0-9]*|*[!A-Za-z0-9_]*)
    echo "RESTORE_TARGET_DATABASE must be a simple PostgreSQL database name" >&2
    exit 1
    ;;
esac
if [ "${#target}" -gt 63 ] || [ "$target" = "$DATABASE_NAME" ]; then
  echo "RESTORE_TARGET_DATABASE must be distinct from the current database and at most 63 characters" >&2
  exit 1
fi

export PGPASSWORD="${DATABASE_PASSWORD:?DATABASE_PASSWORD is required}"
export PGSSLMODE="$DATABASE_SSLMODE"
. /usr/local/lib/scheduler-restore-lib.sh

restore_candidates="${TMPDIR:-/tmp}/scheduler-restore-candidates-$$"
created=false
backup_selection_fatal=false
: > "$restore_candidates"

cleanup() {
  cleanup_staged_backup || true
  if [ "$created" = true ]; then
    dropdb --if-exists --force --host "$DATABASE_HOST" --port "$DATABASE_PORT" --username "$DATABASE_USER" "$target" >/dev/null 2>&1 || true
  fi
  rm -f "$restore_candidates"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

append_restore_candidates() {
  restore_kind="$1"
  restore_directory="$2"
  restore_pattern="$3"
  if [ -n "$BACKUP_RESTORE_RECEIPT_FILE" ]; then
    restore_receipt_archive="$(select_receipt_backup "$restore_directory" "$restore_pattern" "$BACKUP_RESTORE_RECEIPT_FILE" || true)"
    if [ -z "$restore_receipt_archive" ]; then
      echo "backup verification receipt does not match an available generation" >&2
      return 1
    fi
    printf '%s|%s\n' "$restore_kind" "$restore_receipt_archive" >> "$restore_candidates"
    return 0
  fi
  while IFS= read -r restore_archive; do
    [ -n "$restore_archive" ] || continue
    printf '%s|%s\n' "$restore_kind" "$restore_archive" >> "$restore_candidates"
  done <<EOF
$(valid_backup_candidates "$restore_directory" "$restore_pattern" newest)
EOF
}

case "$BACKUP_RESTORE_OFFSITE" in
  true)
    if [ ! -d "$BACKUP_OFFSITE_DIRECTORY" ] || [ ! -f "$BACKUP_OFFSITE_SENTINEL_FILE" ] || [ ! -s "$BACKUP_AGE_IDENTITY_FILE" ]; then
      echo "off-host backup destination or age identity is unavailable" >&2
      exit 1
    fi
    append_restore_candidates age "$BACKUP_OFFSITE_DIRECTORY" 'scheduler-*.dump.age' || exit 1
    ;;
  false)
    append_restore_candidates plain /backups 'scheduler-*.dump' || exit 1
    ;;
  *)
    echo "BACKUP_RESTORE_OFFSITE must be true or false" >&2
    exit 1
    ;;
esac

if [ ! -s "$restore_candidates" ]; then
  echo "no checksum-valid backup generation found" >&2
  exit 1
fi

restore_candidate() {
  candidate_kind="$1"
  candidate_archive="$2"
  stage_backup_archive "$candidate_archive" "$BACKUP_RESTORE_RECEIPT_FILE" || return 1
  if ! createdb --host "$DATABASE_HOST" --port "$DATABASE_PORT" --username "$DATABASE_USER" "$target"; then
    backup_selection_fatal=true
    return 1
  fi
  created=true
  if restore_backup_into_database "$candidate_kind" "$staged_backup_archive" "$target" &&
    validate_restored_scheduler_database "$target" "$MIGRATIONS_PATH"; then
    return 0
  fi
  if ! dropdb --if-exists --force --host "$DATABASE_HOST" --port "$DATABASE_PORT" --username "$DATABASE_USER" "$target"; then
    backup_selection_fatal=true
    return 1
  fi
  created=false
  return 1
}

restore_exists="$(psql --host "$DATABASE_HOST" --port "$DATABASE_PORT" --username "$DATABASE_USER" \
  --dbname postgres --tuples-only --no-align --command="SELECT 1 FROM pg_database WHERE datname='$target'")"
if [ -n "$restore_exists" ]; then
  echo "restore target database already exists: $target" >&2
  exit 1
fi

if select_first_usable_backup "$restore_candidates" restore_candidate; then
  :
else
  selection_status=$?
  if [ "$selection_status" -eq 2 ]; then
    echo "backup restore could not safely continue" >&2
  else
    echo "no restorable backup generation found" >&2
  fi
  exit 1
fi

created=false
printf 'backup restored into %s from %s; run postgres-bootstrap, migrator and preflight --database before cutover\n' \
  "$target" "$(basename "$selected_backup_archive")"
