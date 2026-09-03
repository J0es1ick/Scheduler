#!/bin/sh

. /usr/local/lib/scheduler-backup-generation-lib.sh

: "${DATABASE_HOST:=postgres}"
: "${DATABASE_PORT:=5432}"
: "${DATABASE_USER:=postgres}"
: "${DATABASE_NAME:=scheduler}"
: "${DATABASE_SSLMODE:=prefer}"
: "${DEPLOYMENT_ENV:=development}"
: "${BACKUP_INTERVAL_SECONDS:=86400}"
: "${BACKUP_MAX_AGE_SECONDS:=$((BACKUP_INTERVAL_SECONDS + 3600))}"
: "${BACKUP_RETENTION_DAYS:=14}"
: "${BACKUP_RETRY_SECONDS:=300}"
: "${BACKUP_OFFSITE_DIRECTORY:=}"
: "${BACKUP_OFFSITE_SENTINEL_FILE:=${BACKUP_OFFSITE_DIRECTORY}/.scheduler-offsite}"
: "${BACKUP_AGE_RECIPIENT:=}"
: "${BACKUP_REQUIRE_OFFSITE:=false}"
: "${BACKUP_DIRECTORY:=/backups}"
: "${BACKUP_STATE_DIRECTORY:=/tmp}"
: "${BACKUP_OFFSITE_RETENTION_MODE:=filesystem}"
: "${BACKUP_OFFSITE_RETENTION_DAYS:=90}"
: "${BACKUP_OFFSITE_MIN_GENERATIONS:=7}"
: "${BACKUP_OFFSITE_MAX_BYTES:=0}"
: "${BACKUP_OFFSITE_MAX_AGE_SECONDS:=$BACKUP_MAX_AGE_SECONDS}"

local_success_marker="${BACKUP_STATE_DIRECTORY}/last-backup-success"
offsite_success_marker="${BACKUP_STATE_DIRECTORY}/last-offsite-backup-success"
last_local_backup=""

offsite_requested() {
  [ "$BACKUP_REQUIRE_OFFSITE" = "true" ] ||
    [ -n "$BACKUP_OFFSITE_DIRECTORY" ] ||
    [ -n "$BACKUP_AGE_RECIPIENT" ]
}

validate_bounded_integer() {
  setting_name="$1"
  setting_value="$2"
  setting_min="$3"
  setting_max="$4"
  case "$setting_value" in
    ''|*[!0-9]*)
      echo "$setting_name must be an integer between $setting_min and $setting_max" >&2
      return 1
      ;;
  esac
  if [ "${#setting_value}" -gt 10 ] ||
    [ "$setting_value" -lt "$setting_min" ] ||
    [ "$setting_value" -gt "$setting_max" ]; then
    echo "$setting_name must be an integer between $setting_min and $setting_max" >&2
    return 1
  fi
}

validate_backup_settings() {
  validate_bounded_integer BACKUP_INTERVAL_SECONDS "$BACKUP_INTERVAL_SECONDS" 1 86400 || return 1
  validate_bounded_integer BACKUP_MAX_AGE_SECONDS "$BACKUP_MAX_AGE_SECONDS" 1 604800 || return 1
  validate_bounded_integer BACKUP_RETENTION_DAYS "$BACKUP_RETENTION_DAYS" 1 3650 || return 1
  validate_bounded_integer BACKUP_RETRY_SECONDS "$BACKUP_RETRY_SECONDS" 1 86400 || return 1
  validate_bounded_integer BACKUP_OFFSITE_MAX_AGE_SECONDS "$BACKUP_OFFSITE_MAX_AGE_SECONDS" 1 604800 || return 1
  if [ "$BACKUP_MAX_AGE_SECONDS" -le "$BACKUP_INTERVAL_SECONDS" ] ||
    [ "$BACKUP_OFFSITE_MAX_AGE_SECONDS" -le "$BACKUP_INTERVAL_SECONDS" ]; then
    echo "backup maximum ages must exceed BACKUP_INTERVAL_SECONDS" >&2
    return 1
  fi
  if [ "$DEPLOYMENT_ENV" = "production" ] && {
    [ "$BACKUP_INTERVAL_SECONDS" -lt 60 ] ||
      [ "$BACKUP_MAX_AGE_SECONDS" -gt 172800 ] ||
      [ "$BACKUP_OFFSITE_MAX_AGE_SECONDS" -gt 172800 ];
  }; then
    echo "production backup interval or maximum age is outside the allowed range" >&2
    return 1
  fi
}

probe_offsite_access() {
  probe_partial="${BACKUP_OFFSITE_DIRECTORY}/scheduler-write-probe-$(date -u +%s)-$$.partial"
  probe_final="${probe_partial%.partial}.ready"
  rm -f "$probe_partial" "$probe_final"
  if ! printf '%s\n' scheduler-offsite-write-probe > "$probe_partial"; then
    rm -f "$probe_partial" "$probe_final"
    echo "off-host backup destination is not writable" >&2
    return 1
  fi
  sync "$probe_partial"
  if ! mv "$probe_partial" "$probe_final" ||
    [ "$(cat "$probe_final" 2>/dev/null || true)" != "scheduler-offsite-write-probe" ] ||
    ! rm -f "$probe_final" || [ -e "$probe_final" ]; then
    rm -f "$probe_partial" "$probe_final"
    echo "off-host backup destination does not support finalize/read/delete" >&2
    return 1
  fi
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
  if [ ! -f "$BACKUP_OFFSITE_SENTINEL_FILE" ]; then
    echo "off-host backup sentinel is unavailable" >&2
    return 1
  fi
  if ! age --recipient "$BACKUP_AGE_RECIPIENT" </dev/null >/dev/null 2>&1; then
    echo "age recipient is invalid" >&2
    return 1
  fi
  case "$BACKUP_OFFSITE_RETENTION_MODE" in
    external|filesystem) ;;
    *)
      echo "BACKUP_OFFSITE_RETENTION_MODE must be external or filesystem" >&2
      return 1
      ;;
  esac
  case "$BACKUP_OFFSITE_RETENTION_DAYS" in
    ''|*[!0-9]*|0)
      echo "BACKUP_OFFSITE_RETENTION_DAYS must be a positive integer" >&2
      return 1
      ;;
  esac
  case "$BACKUP_OFFSITE_MIN_GENERATIONS" in
    ''|*[!0-9]*|0)
      echo "BACKUP_OFFSITE_MIN_GENERATIONS must be a positive integer" >&2
      return 1
      ;;
  esac
  case "$BACKUP_OFFSITE_MAX_BYTES" in
    ''|*[!0-9]*)
      echo "BACKUP_OFFSITE_MAX_BYTES must be a non-negative integer" >&2
      return 1
      ;;
  esac
  if [ "${#BACKUP_OFFSITE_MAX_BYTES}" -gt 18 ]; then
    echo "BACKUP_OFFSITE_MAX_BYTES is too large" >&2
    return 1
  fi
  validate_bounded_integer BACKUP_OFFSITE_RETENTION_DAYS "$BACKUP_OFFSITE_RETENTION_DAYS" 1 3650 || return 1
  validate_bounded_integer BACKUP_OFFSITE_MIN_GENERATIONS "$BACKUP_OFFSITE_MIN_GENERATIONS" 1 10000 || return 1
  probe_offsite_access || return 1
}

complete_offsite_generations() {
  valid_backup_candidates "$BACKUP_OFFSITE_DIRECTORY" 'scheduler-*.dump.age' oldest
}

offsite_scheduler_files() {
  find "$BACKUP_OFFSITE_DIRECTORY" -maxdepth 1 -type f \
    \( -name 'scheduler-*.dump.age' -o -name 'scheduler-*.dump.age.sha256' \
       -o -name 'scheduler-*.dump.age.partial' -o -name 'scheduler-*.dump.age.sha256.partial' \
       -o -name 'scheduler-write-probe-*.partial' -o -name 'scheduler-write-probe-*.ready' \) \
    -print | sort
}

incomplete_offsite_files() {
  while IFS= read -r scheduler_file; do
    [ -n "$scheduler_file" ] || continue
    case "$scheduler_file" in
      *.partial|*.ready)
        printf '%s\n' "$scheduler_file"
        ;;
      *.dump.age.sha256)
        [ -s "${scheduler_file%.sha256}" ] || printf '%s\n' "$scheduler_file"
        ;;
      *.dump.age)
        if ! verify_backup_checksum "$scheduler_file"; then
          printf '%s\n' "$scheduler_file"
          [ ! -e "${scheduler_file}.sha256" ] || printf '%s\n' "${scheduler_file}.sha256"
        fi
        ;;
    esac
  done <<EOF
$(offsite_scheduler_files)
EOF
}

collect_offsite_stats() {
  offsite_generation_count=0
  offsite_total_bytes=0
  if ! offsite_requested || [ ! -d "$BACKUP_OFFSITE_DIRECTORY" ]; then
    return 0
  fi
  while IFS= read -r backup_archive; do
    [ -n "$backup_archive" ] || continue
    offsite_generation_count=$((offsite_generation_count + 1))
  done <<EOF
$(complete_offsite_generations)
EOF
  while IFS= read -r scheduler_file; do
    [ -n "$scheduler_file" ] || continue
    file_size="$(stat -c %s "$scheduler_file")" || return 1
    offsite_total_bytes=$((offsite_total_bytes + file_size))
  done <<EOF
$(offsite_scheduler_files)
EOF
}

remove_offsite_generation() {
  backup_archive="$1"
  [ -s "$backup_archive" ] && [ -s "${backup_archive}.sha256" ] || return 0
  rm -f "${backup_archive}.sha256" || return 1
  rm -f "$backup_archive" || return 1
}

run_offsite_retention() {
  offsite_requested || return 0
  [ "$BACKUP_OFFSITE_RETENTION_MODE" = "filesystem" ] || return 0

  while IFS= read -r orphan_file; do
    [ -n "$orphan_file" ] || continue
    if [ -n "$(find "$orphan_file" -prune -mtime "+${BACKUP_OFFSITE_RETENTION_DAYS}" -print)" ]; then
      rm -f "$orphan_file" || {
        echo "off-host orphan retention failed" >&2
        return 1
      }
    fi
  done <<EOF
$(incomplete_offsite_files)
EOF

  collect_offsite_stats || return 1
  remaining="$offsite_generation_count"
  while IFS= read -r backup_archive; do
    [ -n "$backup_archive" ] || continue
    [ "$remaining" -gt "$BACKUP_OFFSITE_MIN_GENERATIONS" ] || break
    if [ -n "$(find "$backup_archive" -prune -mtime "+${BACKUP_OFFSITE_RETENTION_DAYS}" -print)" ]; then
      remove_offsite_generation "$backup_archive" || {
        echo "off-host backup retention failed" >&2
        return 1
      }
      remaining=$((remaining - 1))
    fi
  done <<EOF
$(complete_offsite_generations)
EOF

  if [ "$BACKUP_OFFSITE_MAX_BYTES" -gt 0 ]; then
    collect_offsite_stats || return 1
    while [ "$offsite_total_bytes" -gt "$BACKUP_OFFSITE_MAX_BYTES" ]; do
      oldest_orphan="$(incomplete_offsite_files | head -n 1)"
      if [ -n "$oldest_orphan" ]; then
        rm -f "$oldest_orphan" || {
          echo "off-host orphan quota retention failed" >&2
          return 1
        }
      elif [ "$offsite_generation_count" -gt "$BACKUP_OFFSITE_MIN_GENERATIONS" ]; then
        oldest="$(complete_offsite_generations | head -n 1)"
        [ -n "$oldest" ] || break
        remove_offsite_generation "$oldest" || {
          echo "off-host backup quota retention failed" >&2
          return 1
        }
      else
        echo "off-host backup quota is smaller than protected generations" >&2
        return 1
      fi
      collect_offsite_stats || return 1
    done
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
      collect_offsite_stats || return 1
      printf 'scheduler_backup_last_success_timestamp_seconds{kind="offsite"} %s\n' "$offsite_timestamp"
      printf 'scheduler_backup_max_age_seconds{kind="offsite"} %s\n' "${BACKUP_OFFSITE_MAX_AGE_SECONDS:-90000}"
      printf 'scheduler_backup_generations{kind="offsite"} %s\n' "$offsite_generation_count"
      printf 'scheduler_backup_min_generations{kind="offsite"} %s\n' "$BACKUP_OFFSITE_MIN_GENERATIONS"
      printf 'scheduler_backup_storage_bytes{kind="offsite"} %s\n' "$offsite_total_bytes"
      printf 'scheduler_backup_storage_limit_bytes{kind="offsite"} %s\n' "$BACKUP_OFFSITE_MAX_BYTES"
    fi
  } > "${metrics}.partial" || return 1
  mv "${metrics}.partial" "$metrics" || return 1
}

create_local_backup() {
  timestamp="$(date -u +%Y%m%dT%H%M%SZ)" || return 1
  sequence=0
  target="${BACKUP_DIRECTORY}/scheduler-${timestamp}_$(printf '%06d' "$sequence").dump"
  while [ -e "$target" ] || [ -e "${target}.partial" ]; do
    sequence=$((sequence + 1))
    target="${BACKUP_DIRECTORY}/scheduler-${timestamp}_$(printf '%06d' "$sequence").dump"
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
  if ! run_local_retention; then
    echo "local backup retention failed" >&2
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

  if ! verify_backup_checksum "$encrypted"; then
    rm -f "$encrypted" "$encrypted_checksum" "$encrypted_tmp" "$checksum_tmp"
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
  if ! sync "$remote_encrypted" "$remote_checksum"; then
    echo "could not durably finalize off-host backup" >&2
    return 1
  fi
  if ! verify_backup_checksum "$remote_encrypted"; then
    rm -f "$remote_encrypted" "$remote_checksum"
    echo "off-host backup failed post-upload verification" >&2
    return 1
  fi
  if ! run_offsite_retention; then
    echo "off-host backup lifecycle failed" >&2
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
