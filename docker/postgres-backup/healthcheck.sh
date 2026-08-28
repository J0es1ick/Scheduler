#!/bin/sh
set -eu

: "${BACKUP_INTERVAL_SECONDS:=86400}"
: "${BACKUP_MAX_AGE_SECONDS:=$((BACKUP_INTERVAL_SECONDS + 3600))}"
: "${DEPLOYMENT_ENV:=development}"
: "${BACKUP_OFFSITE_DIRECTORY:=}"
: "${BACKUP_AGE_RECIPIENT:=}"
: "${BACKUP_REQUIRE_OFFSITE:=false}"
: "${BACKUP_OFFSITE_MAX_AGE_SECONDS:=$BACKUP_MAX_AGE_SECONDS}"

case "$BACKUP_REQUIRE_OFFSITE" in
  true|false) ;;
  *)
    echo "BACKUP_REQUIRE_OFFSITE must be true or false" >&2
    exit 1
    ;;
esac

if [ "$DEPLOYMENT_ENV" = "production" ] && [ "$BACKUP_REQUIRE_OFFSITE" != "true" ]; then
  echo "BACKUP_REQUIRE_OFFSITE must be true in production" >&2
  exit 1
fi

offsite_requested=false
if [ "$BACKUP_REQUIRE_OFFSITE" = "true" ] ||
  [ -n "$BACKUP_OFFSITE_DIRECTORY" ] ||
  [ -n "$BACKUP_AGE_RECIPIENT" ]; then
  offsite_requested=true
  if [ -z "$BACKUP_OFFSITE_DIRECTORY" ] || [ ! -d "$BACKUP_OFFSITE_DIRECTORY" ] ||
    [ -z "$BACKUP_AGE_RECIPIENT" ]; then
    echo "off-host backup destination or age recipient is unavailable" >&2
    exit 1
  fi
  if ! age --recipient "$BACKUP_AGE_RECIPIENT" </dev/null >/dev/null 2>&1; then
    echo "age recipient is invalid" >&2
    exit 1
  fi
fi

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

if [ "$offsite_requested" = "true" ]; then
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
