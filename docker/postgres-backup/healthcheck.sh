#!/bin/sh
set -eu

: "${BACKUP_INTERVAL_SECONDS:=86400}"
: "${BACKUP_MAX_AGE_SECONDS:=$((BACKUP_INTERVAL_SECONDS + 3600))}"

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

exit 0
