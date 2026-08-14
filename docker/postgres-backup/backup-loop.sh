#!/bin/sh
set -eu

: "${DATABASE_HOST:=postgres}"
: "${DATABASE_PORT:=5432}"
: "${DATABASE_USER:=postgres}"
: "${DATABASE_NAME:=scheduler}"
: "${BACKUP_INTERVAL_SECONDS:=86400}"
: "${BACKUP_RETENTION_DAYS:=14}"
: "${BACKUP_RETRY_SECONDS:=300}"

export PGPASSWORD="${DATABASE_PASSWORD:?DATABASE_PASSWORD is required}"

. /usr/local/lib/scheduler-backup-lib.sh

until pg_isready -h "$DATABASE_HOST" -p "$DATABASE_PORT" -U "$DATABASE_USER" -d "$DATABASE_NAME" >/dev/null 2>&1; do
  sleep 2
done

pending_offsite=""
next_local_at=0

while true; do
  now="$(date -u +%s)"
  if [ "$now" -ge "$next_local_at" ]; then
    if create_local_backup; then
      pending_offsite="$last_local_backup"
      next_local_at=$((now + BACKUP_INTERVAL_SECONDS))
    else
      run_local_retention || true
      echo "local backup retry scheduled in ${BACKUP_RETRY_SECONDS}s" >&2
      sleep "$BACKUP_RETRY_SECONDS"
      continue
    fi
    run_local_retention || true
  fi

  if offsite_requested && [ -n "$pending_offsite" ]; then
    if upload_offsite_backup "$pending_offsite"; then
      pending_offsite=""
    else
      run_local_retention || true
      now="$(date -u +%s)"
      wait_seconds="$BACKUP_RETRY_SECONDS"
      until_local=$((next_local_at - now))
      if [ "$until_local" -gt 0 ] && [ "$until_local" -lt "$wait_seconds" ]; then
        wait_seconds="$until_local"
      fi
      if [ "$wait_seconds" -lt 1 ]; then
        wait_seconds=1
      fi
      echo "off-host upload retry scheduled in ${wait_seconds}s" >&2
      sleep "$wait_seconds"
      continue
    fi
  fi

  now="$(date -u +%s)"
  wait_seconds=$((next_local_at - now))
  if [ "$wait_seconds" -lt 1 ]; then
    wait_seconds=1
  fi
  if offsite_requested && [ -n "$pending_offsite" ] && [ "$BACKUP_RETRY_SECONDS" -lt "$wait_seconds" ]; then
    wait_seconds="$BACKUP_RETRY_SECONDS"
  fi
  sleep "$wait_seconds"
done
