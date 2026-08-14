#!/bin/sh
set -eu

: "${BACKUP_INTERVAL_SECONDS:=86400}"
: "${BACKUP_MAX_AGE_SECONDS:=$((BACKUP_INTERVAL_SECONDS + 3600))}"
: "${BACKUP_OFFSITE_DIRECTORY:=}"
: "${BACKUP_ENCRYPTION_PASSPHRASE_FILE:=}"
: "${BACKUP_OFFSITE_MAX_AGE_SECONDS:=$BACKUP_MAX_AGE_SECONDS}"

marker=/tmp/last-backup-success
if [ ! -s "$marker" ]; then
  echo "no successful backup marker" >&2
  exit 1
fi

last_success="$(cat "$marker")"
case "$last_success" in
  ''|*[!0-9]*)
    echo "invalid backup marker" >&2
    exit 1
    ;;
esac

age="$(( $(date -u +%s) - last_success ))"
if [ "$age" -lt 0 ] || [ "$age" -gt "$BACKUP_MAX_AGE_SECONDS" ]; then
  echo "latest successful backup is stale: ${age}s" >&2
  exit 1
fi

if [ -n "$BACKUP_OFFSITE_DIRECTORY" ] || [ -n "$BACKUP_ENCRYPTION_PASSPHRASE_FILE" ]; then
  offsite_marker=/tmp/last-offsite-backup-success
  if [ ! -s "$offsite_marker" ]; then
    echo "no successful off-host backup marker" >&2
    exit 1
  fi
  offsite_success="$(cat "$offsite_marker")"
  case "$offsite_success" in
    ''|*[!0-9]*)
      echo "invalid off-host backup marker" >&2
      exit 1
      ;;
  esac
  offsite_age="$(( $(date -u +%s) - offsite_success ))"
  if [ "$offsite_age" -lt 0 ] || [ "$offsite_age" -gt "$BACKUP_OFFSITE_MAX_AGE_SECONDS" ]; then
    echo "latest successful off-host backup is stale: ${offsite_age}s" >&2
    exit 1
  fi
fi

exit 0
