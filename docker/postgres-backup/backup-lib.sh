#!/bin/sh

: "${DATABASE_HOST:=postgres}"
: "${DATABASE_PORT:=5432}"
: "${DATABASE_USER:=postgres}"
: "${DATABASE_NAME:=scheduler}"
: "${BACKUP_RETENTION_DAYS:=14}"
: "${BACKUP_OFFSITE_DIRECTORY:=}"
: "${BACKUP_ENCRYPTION_PASSPHRASE_FILE:=}"
: "${BACKUP_DIRECTORY:=/backups}"
: "${BACKUP_STATE_DIRECTORY:=/tmp}"

local_success_marker="${BACKUP_STATE_DIRECTORY}/last-backup-success"
offsite_success_marker="${BACKUP_STATE_DIRECTORY}/last-offsite-backup-success"
last_local_backup=""

offsite_requested() {
  [ -n "$BACKUP_OFFSITE_DIRECTORY" ] || [ -n "$BACKUP_ENCRYPTION_PASSPHRASE_FILE" ]
}

validate_offsite() {
  if ! offsite_requested; then
    return 0
  fi
  if [ -z "$BACKUP_OFFSITE_DIRECTORY" ] || [ -z "$BACKUP_ENCRYPTION_PASSPHRASE_FILE" ]; then
    echo "both BACKUP_OFFSITE_DIRECTORY and BACKUP_ENCRYPTION_PASSPHRASE_FILE are required" >&2
    return 1
  fi
  if [ ! -r "$BACKUP_ENCRYPTION_PASSPHRASE_FILE" ] || [ ! -d "$BACKUP_OFFSITE_DIRECTORY" ]; then
    echo "off-host backup destination or encryption secret is unavailable" >&2
    return 1
  fi
}

write_success_marker() {
  marker="$1"
  marker_tmp="${marker}.partial"
  printf '%s\n' "$(date -u +%s)" > "$marker_tmp" || return 1
  mv "$marker_tmp" "$marker" || return 1
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
  encrypted="${target}.enc"
  encrypted_checksum="${encrypted}.sha256"
  encrypted_tmp="${encrypted}.partial"
  checksum_tmp="${encrypted_checksum}.partial"
  remote_encrypted="${BACKUP_OFFSITE_DIRECTORY}/$(basename "$encrypted")"
  remote_checksum="${BACKUP_OFFSITE_DIRECTORY}/$(basename "$encrypted_checksum")"
  remote_encrypted_tmp="${remote_encrypted}.partial"
  remote_checksum_tmp="${remote_checksum}.partial"

  if [ ! -s "$encrypted" ]; then
    rm -f "$encrypted_tmp" "$checksum_tmp"
    if ! openssl enc -aes-256-cbc -salt -pbkdf2 \
      -pass "file:${BACKUP_ENCRYPTION_PASSPHRASE_FILE}" \
      -in "$target" -out "$encrypted_tmp"; then
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
