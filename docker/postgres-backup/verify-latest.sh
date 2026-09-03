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
: "${BACKUP_OFFSITE_SENTINEL_FILE:=${BACKUP_OFFSITE_DIRECTORY}/.scheduler-offsite}"
: "${BACKUP_AGE_IDENTITY_FILE:=}"
: "${BACKUP_ENCRYPTION_PASSPHRASE_FILE:=}"
: "${BACKUP_ALLOW_LEGACY_AES_CBC:=false}"
: "${BACKUP_VERIFY_RECEIPT_FILE:=}"

export PGPASSWORD="${DATABASE_PASSWORD:?DATABASE_PASSWORD is required}"
export PGSSLMODE="$DATABASE_SSLMODE"
. /usr/local/lib/scheduler-restore-lib.sh

verify_candidates="${TMPDIR:-/tmp}/scheduler-verify-candidates-$$"
verify_db=
verify_created=false
backup_selection_fatal=false
: > "$verify_candidates"

cleanup() {
  if [ "$verify_created" = true ] && [ -n "$verify_db" ]; then
    dropdb --if-exists --force --host "$DATABASE_HOST" --port "$DATABASE_PORT" --username "$DATABASE_USER" "$verify_db" >/dev/null 2>&1 || true
  fi
  rm -f "$verify_candidates"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

append_candidates() {
  append_kind="$1"
  append_directory="$2"
  append_pattern="$3"
  while IFS= read -r append_archive; do
    [ -n "$append_archive" ] || continue
    printf '%s|%s\n' "$append_kind" "$append_archive" >> "$verify_candidates"
  done <<EOF
$(valid_backup_candidates "$append_directory" "$append_pattern" newest)
EOF
}

case "$BACKUP_VERIFY_OFFSITE" in
  true)
    if [ ! -d "$BACKUP_OFFSITE_DIRECTORY" ] || [ ! -f "$BACKUP_OFFSITE_SENTINEL_FILE" ] || [ ! -s "$BACKUP_AGE_IDENTITY_FILE" ]; then
      echo "off-host backup destination or age identity is unavailable" >&2
      exit 1
    fi
    append_candidates age "$BACKUP_OFFSITE_DIRECTORY" 'scheduler-*.dump.age'
    case "$BACKUP_ALLOW_LEGACY_AES_CBC" in
      true)
        if [ -s "$BACKUP_ENCRYPTION_PASSPHRASE_FILE" ]; then
          append_candidates legacy "$BACKUP_OFFSITE_DIRECTORY" 'scheduler-*.dump.enc'
        fi
        ;;
      false) ;;
      *)
        echo "BACKUP_ALLOW_LEGACY_AES_CBC must be true or false" >&2
        exit 1
        ;;
    esac
    ;;
  false)
    append_candidates plain /backups 'scheduler-*.dump'
    ;;
  *)
    echo "BACKUP_VERIFY_OFFSITE must be true or false" >&2
    exit 1
    ;;
esac

if [ ! -s "$verify_candidates" ]; then
  echo "no checksum-valid backup generation found" >&2
  exit 1
fi

verify_candidate() {
  verify_kind="$1"
  verify_archive="$2"
  verify_random="$(od -An -N4 -tu4 /dev/urandom | tr -d ' ')"
  verify_db="scheduler_verify_$(date -u +%s)_$$_${verify_random}"
  if ! createdb --host "$DATABASE_HOST" --port "$DATABASE_PORT" --username "$DATABASE_USER" "$verify_db"; then
    backup_selection_fatal=true
    return 1
  fi
  verify_created=true
  if restore_backup_into_database "$verify_kind" "$verify_archive" "$verify_db" &&
    validate_restored_scheduler_database "$verify_db" "$MIGRATIONS_PATH"; then
    verify_result=0
  else
    verify_result=1
  fi
  if ! dropdb --if-exists --force --host "$DATABASE_HOST" --port "$DATABASE_PORT" --username "$DATABASE_USER" "$verify_db"; then
    backup_selection_fatal=true
    return 1
  fi
  verify_created=false
  verify_db=
  return "$verify_result"
}

if select_first_usable_backup "$verify_candidates" verify_candidate; then
  :
else
  selection_status=$?
  if [ "$selection_status" -eq 2 ]; then
    echo "backup verification could not safely continue" >&2
  else
    echo "no restorable backup generation found" >&2
  fi
  exit 1
fi

if [ -n "$BACKUP_VERIFY_RECEIPT_FILE" ]; then
  if ! write_backup_receipt "$BACKUP_VERIFY_RECEIPT_FILE" "$selected_backup_archive"; then
    echo "could not write backup verification receipt" >&2
    exit 1
  fi
fi

selected_digest="$(backup_archive_digest "$selected_backup_archive")"
printf 'verified backup generation: %s\n' "$(basename "$selected_backup_archive")"
printf 'verified backup sha256: %s\n' "$selected_digest"
if [ -n "$BACKUP_VERIFY_RECEIPT_FILE" ]; then
  printf 'verification receipt written: %s\n' "$BACKUP_VERIFY_RECEIPT_FILE"
fi
printf 'restore verification completed: migrations=%s\n' "$validated_migration_count"
