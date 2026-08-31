#!/bin/sh

: "${DATABASE_HOST:=postgres}"
: "${DATABASE_PORT:=5432}"
: "${DATABASE_USER:=postgres}"
: "${DATABASE_NAME:=scheduler}"
: "${DATABASE_SSLMODE:=prefer}"
: "${DEPLOYMENT_ENV:=development}"
: "${BACKUP_RETENTION_DAYS:=14}"
: "${BACKUP_OFFSITE_DIRECTORY:=}"
: "${BACKUP_AGE_RECIPIENT:=}"
: "${BACKUP_REQUIRE_OFFSITE:=false}"
: "${BACKUP_DIRECTORY:=/backups}"
: "${BACKUP_STATE_DIRECTORY:=/tmp}"

local_success_marker="${BACKUP_STATE_DIRECTORY}/last-backup-success"
offsite_success_marker="${BACKUP_STATE_DIRECTORY}/last-offsite-backup-success"
last_local_backup=""

offsite_requested() {
  [ "$BACKUP_REQUIRE_OFFSITE" = "true" ] ||
    [ -n "$BACKUP_OFFSITE_DIRECTORY" ] ||
    [ -n "$BACKUP_AGE_RECIPIENT" ]
}

validate_offsite() {
  case "$BACKUP_REQUIRE_OFFSITE" in
    true|false) ;;
    *)
      echo "BACKUP_REQUIRE_OFFSITE must be true or false" >&2
      return 1
      ;;
  esac
  if [ "$DEPLOYMENT_ENV" = "production" ] && [ "$BACKUP_REQUIRE_OFFSITE" != "true" ]; then
    echo "BACKUP_REQUIRE_OFFSITE must be true in production" >&2
    return 1
  fi
  if ! offsite_requested; then
    return 0
  fi
  if [ -z "$BACKUP_OFFSITE_DIRECTORY" ] || [ -z "$BACKUP_AGE_RECIPIENT" ]; then
    echo "both BACKUP_OFFSITE_DIRECTORY and BACKUP_AGE_RECIPIENT are required" >&2
    return 1
  fi
  if [ ! -d "$BACKUP_OFFSITE_DIRECTORY" ]; then
    echo "off-host backup destination is unavailable" >&2
    return 1
  fi
  if ! age --recipient "$BACKUP_AGE_RECIPIENT" </dev/null >/dev/null 2>&1; then
    echo "age recipient is invalid" >&2
    return 1
  fi
}

run_age() {
  age --recipient "$BACKUP_AGE_RECIPIENT" "$@"
}

write_success_marker() {
  marker="$1"
  marker_tmp="${marker}.partial"
  printf '%s\n' "$(date -u +%s)" > "$marker_tmp" || return 1
  mv "$marker_tmp" "$marker" || return 1
  write_backup_metrics || return 1
}

write_backup_metrics() {
  local_timestamp=0
  offsite_timestamp=0
  [ ! -s "$local_success_marker" ] || local_timestamp="$(cat "$local_success_marker")"
  [ ! -s "$offsite_success_marker" ] || offsite_timestamp="$(cat "$offsite_success_marker")"
  metrics="${BACKUP_DIRECTORY}/scheduler-backup.prom"
  {
    printf 'scheduler_backup_last_success_timestamp_seconds{kind="local"} %s\n' "$local_timestamp"
    printf 'scheduler_backup_max_age_seconds{kind="local"} %s\n' "${BACKUP_MAX_AGE_SECONDS:-90000}"
    if offsite_requested; then
      printf 'scheduler_backup_last_success_timestamp_seconds{kind="offsite"} %s\n' "$offsite_timestamp"
      printf 'scheduler_backup_max_age_seconds{kind="offsite"} %s\n' "${BACKUP_OFFSITE_MAX_AGE_SECONDS:-90000}"
    fi
  } > "${metrics}.partial" || return 1
  mv "${metrics}.partial" "$metrics" || return 1
}

create_local_backup() {
  timestamp="$(date -u +%Y%m%dT%H%M%SZ)" || return 1
  target="${BACKUP_DIRECTORY}/scheduler-${timestamp}.dump"
  sequence=0
  while [ -e "$target" ] || [ -e "${target}.partial" ]; do
    sequence=$((sequence + 1))
    target="${BACKUP_DIRECTORY}/scheduler-${timestamp}-${sequence}.dump"
  done
  temporary="${target}.partial"
  checksum="${target}.sha256"
  checksum_tmp="${checksum}.partial"

  if ! pg_dump \
    --host "$DATABASE_HOST" \
    --port "$DATABASE_PORT" \
    --username "$DATABASE_USER" \
    --dbname "$DATABASE_NAME" \
    --format custom \
    --compress 6 \
    --no-owner \
    --no-acl \
    --file "$temporary"; then
    rm -f "$temporary" "$checksum_tmp"
    echo "database dump failed" >&2
    return 1
  fi
  if ! mv "$temporary" "$target"; then
    rm -f "$temporary" "$checksum_tmp"
    echo "could not finalize local dump" >&2
    return 1
  fi
  if ! (cd "$BACKUP_DIRECTORY" && sha256sum "$(basename "$target")" > "$(basename "$checksum_tmp")"); then
    rm -f "$target" "$checksum_tmp"
    echo "could not checksum local dump" >&2
    return 1
  fi
  if ! mv "$checksum_tmp" "$checksum"; then
    rm -f "$target" "$checksum_tmp"
    echo "could not finalize local checksum" >&2
    return 1
  fi
  if ! write_success_marker "$local_success_marker"; then
    echo "could not record local backup success" >&2
    return 1
  fi
  last_local_backup="$target"
  echo "local backup completed: $(basename "$target")"
  return 0
}

run_local_retention() {
  find "$BACKUP_DIRECTORY" -maxdepth 1 -type f \
    \( -name 'scheduler-*.dump' -o -name 'scheduler-*.dump.sha256' \
       -o -name 'scheduler-*.dump.age' -o -name 'scheduler-*.dump.age.sha256' \
       -o -name 'scheduler-*.dump.enc' -o -name 'scheduler-*.dump.enc.sha256' \
       -o -name 'scheduler-*.partial' \) \
    -mtime "+${BACKUP_RETENTION_DAYS}" -delete || {
      echo "local backup retention failed" >&2
      return 1
    }
}

upload_offsite_backup() {
  target="$1"
  validate_offsite || return 1
  encrypted="${target}.age"
  encrypted_checksum="${encrypted}.sha256"
  encrypted_tmp="${encrypted}.partial"
  checksum_tmp="${encrypted_checksum}.partial"
  remote_encrypted="${BACKUP_OFFSITE_DIRECTORY}/$(basename "$encrypted")"
  remote_checksum="${BACKUP_OFFSITE_DIRECTORY}/$(basename "$encrypted_checksum")"
  remote_encrypted_tmp="${remote_encrypted}.partial"
  remote_checksum_tmp="${remote_checksum}.partial"

  if [ ! -s "$encrypted" ]; then
    rm -f "$encrypted_tmp" "$checksum_tmp"
    if ! run_age --output "$encrypted_tmp" "$target"; then
      rm -f "$encrypted_tmp" "$checksum_tmp"
      echo "off-host backup encryption failed" >&2
      return 1
    fi
    if ! mv "$encrypted_tmp" "$encrypted"; then
      rm -f "$encrypted_tmp" "$checksum_tmp"
      echo "could not finalize encrypted backup" >&2
      return 1
    fi
    if ! (cd "$BACKUP_DIRECTORY" && sha256sum "$(basename "$encrypted")" > "$(basename "$checksum_tmp")"); then
      rm -f "$encrypted" "$checksum_tmp"
      echo "could not checksum encrypted backup" >&2
      return 1
    fi
    if ! mv "$checksum_tmp" "$encrypted_checksum"; then
      rm -f "$encrypted" "$checksum_tmp"
      echo "could not finalize encrypted checksum" >&2
      return 1
    fi
  fi

  rm -f "$remote_encrypted_tmp" "$remote_checksum_tmp"
  if ! cp "$encrypted" "$remote_encrypted_tmp"; then
    rm -f "$remote_encrypted_tmp" "$remote_checksum_tmp"
    echo "off-host backup upload failed" >&2
    return 1
  fi
  if ! cp "$encrypted_checksum" "$remote_checksum_tmp"; then
    rm -f "$remote_encrypted_tmp" "$remote_checksum_tmp"
    echo "off-host checksum upload failed" >&2
    return 1
  fi
  if ! mv "$remote_encrypted_tmp" "$remote_encrypted"; then
    rm -f "$remote_encrypted_tmp" "$remote_checksum_tmp"
    echo "could not finalize off-host backup" >&2
    return 1
  fi
  if ! mv "$remote_checksum_tmp" "$remote_checksum"; then
    rm -f "$remote_checksum_tmp"
    echo "could not finalize off-host checksum" >&2
    return 1
  fi
  if ! write_success_marker "$offsite_success_marker"; then
    echo "could not record off-host backup success" >&2
    return 1
  fi
  rm -f "$encrypted" "$encrypted_checksum"
  echo "off-host backup completed: $(basename "$remote_encrypted")"
  return 0
}
