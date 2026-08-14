#!/bin/sh
set -eu

library=/usr/local/lib/scheduler-backup-lib.sh

fail() {
  echo "backup test failed: $*" >&2
  exit 1
}

run_case() {
  name="$1"
  fail_step="$2"
  work="/tmp/backup-test-${name}"
  rm -rf "$work"
  mkdir -p "$work/backups" "$work/state" "$work/offsite"
  printf '%s\n' 'integration-passphrase' > "$work/passphrase"

  (
    export BACKUP_DIRECTORY="$work/backups"
    export BACKUP_STATE_DIRECTORY="$work/state"
    export BACKUP_OFFSITE_DIRECTORY="$work/offsite"
    export BACKUP_ENCRYPTION_PASSPHRASE_FILE="$work/passphrase"
    export BACKUP_RETENTION_DAYS=14
    export FAIL_STEP="$fail_step"

    pg_dump() {
      output=""
      while [ "$#" -gt 0 ]; do
        if [ "$1" = "--file" ]; then
          shift
          output="$1"
        fi
        shift
      done
      [ -n "$output" ] || return 1
      printf '%s\n' 'test dump' > "$output"
    }
    mv() {
      if [ "$FAIL_STEP" = "mv" ]; then
        return 71
      fi
      /bin/busybox mv "$@"
    }
    sha256sum() {
      if [ "$FAIL_STEP" = "checksum" ]; then
        return 72
      fi
      /bin/busybox sha256sum "$@"
    }
    openssl() {
      if [ "$FAIL_STEP" = "openssl" ]; then
        return 73
      fi
      input=""
      output=""
      while [ "$#" -gt 0 ]; do
        case "$1" in
          -in)
            shift
            input="$1"
            ;;
          -out)
            shift
            output="$1"
            ;;
        esac
        shift
      done
      /bin/busybox cp "$input" "$output"
    }
    cp() {
      if [ "$FAIL_STEP" = "offsite" ]; then
        return 74
      fi
      /bin/busybox cp "$@"
    }

    . "$library"

    case "$name" in
      local-mv|local-checksum)
        if create_local_backup; then
          fail "$name unexpectedly succeeded"
        fi
        [ ! -e "$local_success_marker" ] || fail "$name wrote success marker"
        ;;
      encryption|offsite-copy)
        create_local_backup || fail "$name local backup failed"
        before="$(find "$BACKUP_DIRECTORY" -maxdepth 1 -name 'scheduler-*.dump' | wc -l)"
        if upload_offsite_backup "$last_local_backup"; then
          fail "$name unexpectedly uploaded"
        fi
        run_local_retention || fail "$name retention failed"
        if upload_offsite_backup "$last_local_backup"; then
          fail "$name retry unexpectedly uploaded"
        fi
        after="$(find "$BACKUP_DIRECTORY" -maxdepth 1 -name 'scheduler-*.dump' | wc -l)"
        [ "$before" -eq 1 ] || fail "$name did not create exactly one local dump"
        [ "$after" -eq "$before" ] || fail "$name retry created another local dump"
        [ -s "$local_success_marker" ] || fail "$name lost local success state"
        [ ! -e "$offsite_success_marker" ] || fail "$name wrote offsite success marker"
        ;;
      success)
        create_local_backup || fail "successful local backup failed"
        upload_offsite_backup "$last_local_backup" || fail "successful offsite upload failed"
        [ -s "$local_success_marker" ] || fail "local success marker missing"
        [ -s "$offsite_success_marker" ] || fail "offsite success marker missing"
        find "$BACKUP_OFFSITE_DIRECTORY" -maxdepth 1 -name 'scheduler-*.dump.enc' | grep -q . ||
          fail "encrypted offsite dump missing"
        ;;
    esac
  )
  rm -rf "$work"
}

run_case local-mv mv
run_case local-checksum checksum
run_case encryption openssl
run_case offsite-copy offsite
run_case success none

echo "backup failure-injection tests passed"
