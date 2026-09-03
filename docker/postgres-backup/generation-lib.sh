#!/bin/sh

backup_archive_digest() {
  archive="$1"
  sha256sum "$archive" | awk '{ print $1 }'
}

verify_backup_checksum() {
  archive="$1"
  checksum="${archive}.sha256"
  [ -s "$archive" ] && [ -s "$checksum" ] || return 1
  expected="$(awk 'NR == 1 { print $1 }' "$checksum")"
  case "$expected" in
    *[!0-9a-fA-F]*|'') return 1 ;;
  esac
  [ "${#expected}" -eq 64 ] || return 1
  actual="$(backup_archive_digest "$archive")" || return 1
  [ "$actual" = "$expected" ]
}

backup_generation_sort_key() {
  archive_name="$(basename "$1")"
  generation_stem="${archive_name%%.dump*}"
  generation_stem="${generation_stem#scheduler-}"
  generation_timestamp="$(printf '%.16s' "$generation_stem")"
  generation_digits="$(printf '%s' "$generation_timestamp" | tr -d 'TZ')"
  case "$generation_timestamp" in
    ????????T??????Z) ;;
    *)
      printf '00000000T000000Z00000000000000000000|%s\n' "$archive_name"
      return 0
      ;;
  esac
  case "$generation_digits" in
    ''|*[!0-9]*)
      printf '00000000T000000Z00000000000000000000|%s\n' "$archive_name"
      return 0
      ;;
  esac
  generation_suffix="${generation_stem#"$generation_timestamp"}"
  case "$generation_suffix" in
    '') generation_sequence=0 ;;
    _*|-*) generation_sequence="${generation_suffix#?}" ;;
    *) generation_sequence= ;;
  esac
  case "$generation_sequence" in
    ''|*[!0-9]*)
      printf '00000000T000000Z00000000000000000000|%s\n' "$archive_name"
      return 0
      ;;
  esac
  if [ "${#generation_sequence}" -gt 20 ]; then
    printf '00000000T000000Z00000000000000000000|%s\n' "$archive_name"
    return 0
  fi
  generation_sequence_key="$(printf '%020s' "$generation_sequence" | tr ' ' 0)"
  printf '%s%s|%s\n' "$generation_timestamp" "$generation_sequence_key" "$archive_name"
}

ordered_backup_archives() {
  backup_directory="$1"
  backup_pattern="$2"
  backup_order="$3"
  ordering_file="${TMPDIR:-/tmp}/scheduler-backup-order-$$-${backup_order}"
  : > "$ordering_file" || return 1
  while IFS= read -r ordered_archive; do
    [ -n "$ordered_archive" ] || continue
    ordering_line="$(backup_generation_sort_key "$ordered_archive")" || {
      rm -f "$ordering_file"
      return 1
    }
    printf '%s|%s\n' "${ordering_line%%|*}" "$ordered_archive" >> "$ordering_file" || {
      rm -f "$ordering_file"
      return 1
    }
  done <<EOF
$(find "$backup_directory" -maxdepth 1 -type f -name "$backup_pattern" -print)
EOF
  if [ "$backup_order" = newest ]; then
    LC_ALL=C sort -r "$ordering_file"
  else
    LC_ALL=C sort "$ordering_file"
  fi | cut -d '|' -f 2-
  ordering_status=$?
  rm -f "$ordering_file"
  return "$ordering_status"
}

valid_backup_candidates() {
  candidate_directory="$1"
  candidate_pattern="$2"
  candidate_order="${3:-newest}"
  while IFS= read -r candidate_archive; do
    [ -n "$candidate_archive" ] || continue
    if verify_backup_checksum "$candidate_archive"; then
      printf '%s\n' "$candidate_archive"
    else
      echo "skipping invalid backup generation: $(basename "$candidate_archive")" >&2
    fi
  done <<EOF
$(ordered_backup_archives "$candidate_directory" "$candidate_pattern" "$candidate_order")
EOF
}

select_newest_valid_backup() {
  candidate_directory="$1"
  candidate_pattern="$2"
  selected_archive="$(valid_backup_candidates "$candidate_directory" "$candidate_pattern" newest | sed -n '1p')"
  [ -n "$selected_archive" ] || return 1
  printf '%s\n' "$selected_archive"
}

write_backup_receipt() {
  receipt_path="$1"
  receipt_archive="$2"
  receipt_digest="$(backup_archive_digest "$receipt_archive")" || return 1
  receipt_name="$(basename "$receipt_archive")"
  receipt_temporary="${receipt_path}.partial-$$"
  receipt_directory="$(dirname "$receipt_path")"
  [ -d "$receipt_directory" ] || return 1
  old_umask="$(umask)"
  umask 077
  if ! printf 'scheduler-backup-v1|%s|%s\n' "$receipt_name" "$receipt_digest" > "$receipt_temporary"; then
    umask "$old_umask"
    rm -f "$receipt_temporary"
    return 1
  fi
  umask "$old_umask"
  sync "$receipt_temporary" || {
    rm -f "$receipt_temporary"
    return 1
  }
  mv "$receipt_temporary" "$receipt_path"
}

read_backup_receipt() {
  receipt_path="$1"
  [ -s "$receipt_path" ] || return 1
  [ "$(wc -l < "$receipt_path" | tr -d ' ')" -eq 1 ] || return 1
  receipt_version="$(awk -F '|' 'NR == 1 { print $1 }' "$receipt_path")"
  receipt_archive_name="$(awk -F '|' 'NR == 1 { print $2 }' "$receipt_path")"
  receipt_archive_digest="$(awk -F '|' 'NR == 1 { print $3 }' "$receipt_path")"
  receipt_extra="$(awk -F '|' 'NR == 1 { print $4 }' "$receipt_path")"
  [ "$receipt_version" = scheduler-backup-v1 ] && [ -z "$receipt_extra" ] || return 1
  case "$receipt_archive_name" in
    scheduler-*.dump|scheduler-*.dump.age|scheduler-*.dump.enc) ;;
    *) return 1 ;;
  esac
  case "$receipt_archive_name" in
    *[!A-Za-z0-9_.-]*) return 1 ;;
  esac
  case "$receipt_archive_digest" in
    *[!0-9a-fA-F]*|'') return 1 ;;
  esac
  [ "${#receipt_archive_digest}" -eq 64 ]
}

select_receipt_backup() {
  receipt_directory="$1"
  receipt_pattern="$2"
  receipt_path="$3"
  read_backup_receipt "$receipt_path" || return 1
  case "$receipt_archive_name" in
    $receipt_pattern) ;;
    *) return 1 ;;
  esac
  receipt_selected_archive="${receipt_directory}/${receipt_archive_name}"
  verify_backup_checksum "$receipt_selected_archive" || return 1
  receipt_actual_digest="$(backup_archive_digest "$receipt_selected_archive")" || return 1
  [ "$receipt_actual_digest" = "$receipt_archive_digest" ] || return 1
  printf '%s\n' "$receipt_selected_archive"
}

select_first_usable_backup() {
  usable_candidates_file="$1"
  usable_callback="$2"
  selected_backup_kind=
  selected_backup_archive=
  while IFS='|' read -r usable_kind usable_archive; do
    [ -n "$usable_kind" ] && [ -n "$usable_archive" ] || continue
    if "$usable_callback" "$usable_kind" "$usable_archive"; then
      selected_backup_kind="$usable_kind"
      selected_backup_archive="$usable_archive"
      return 0
    fi
    if [ "${backup_selection_fatal:-false}" = true ]; then
      return 2
    fi
    echo "skipping unrestorable backup generation: $(basename "$usable_archive")" >&2
  done < "$usable_candidates_file"
  return 1
}
