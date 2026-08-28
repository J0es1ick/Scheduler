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
  age-keygen --output "$work/age-identity" >/dev/null 2>&1
  age_recipient="$(age-keygen -y "$work/age-identity")"

  (
    export BACKUP_DIRECTORY="$work/backups"
    export BACKUP_STATE_DIRECTORY="$work/state"
    export BACKUP_OFFSITE_DIRECTORY="$work/offsite"
    export BACKUP_AGE_RECIPIENT="$age_recipient"
    export BACKUP_RETENTION_DAYS=14
    export FAIL_STEP="$fail_step"
    pg_dump_no_acl=false

    pg_dump() {
      output=""
      while [ "$#" -gt 0 ]; do
        case "$1" in
          --file)
            shift
            output="$1"
            ;;
          --no-acl)
            pg_dump_no_acl=true
            ;;
        esac
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
    age() {
      if [ "$FAIL_STEP" = "age" ]; then
        for argument in "$@"; do
          if [ "$argument" = "--output" ]; then
            return 73
          fi
        done
      fi
      /usr/bin/age "$@"
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
        [ "$pg_dump_no_acl" = true ] || fail "local backup contains environment-specific ACLs"
        upload_offsite_backup "$last_local_backup" || fail "successful offsite upload failed"
        [ -s "$local_success_marker" ] || fail "local success marker missing"
        [ -s "$offsite_success_marker" ] || fail "offsite success marker missing"
        encrypted="$(find "$BACKUP_OFFSITE_DIRECTORY" -maxdepth 1 -name 'scheduler-*.dump.age' -print | head -n 1)"
        [ -n "$encrypted" ] || fail "authenticated encrypted offsite dump missing"
        recovered="$work/recovered.dump"
        /usr/bin/age --decrypt --identity "$work/age-identity" \
          --output "$recovered" "$encrypted" ||
          fail "authenticated encrypted backup could not be decrypted"
        cmp -s "$last_local_backup" "$recovered" || fail "encrypted backup round-trip changed the dump"

        tampered="$work/tampered.dump.age"
        /bin/busybox cp "$encrypted" "$tampered"
        printf '\000' | dd of="$tampered" bs=1 seek=20 count=1 conv=notrunc 2>/dev/null
        /bin/busybox sha256sum "$tampered" > "${tampered}.sha256"
        /bin/busybox sha256sum -c "${tampered}.sha256" >/dev/null ||
          fail "tampered backup checksum was not recomputed"
        if /usr/bin/age --decrypt --identity "$work/age-identity" \
          --output "$work/tampered.dump" "$tampered" >/dev/null 2>&1; then
          fail "authenticated encryption accepted a tampered backup with a recomputed checksum"
        fi
        ;;
    esac
  )
  rm -rf "$work"
}

run_case local-mv mv
run_case local-checksum checksum
run_case encryption age
run_case offsite-copy offsite
run_case success none

(
  export BACKUP_REQUIRE_OFFSITE=true
  export BACKUP_OFFSITE_DIRECTORY=
  export BACKUP_AGE_RECIPIENT=
  . "$library"
  if validate_offsite; then
    fail "required offsite backup accepted missing configuration"
  fi
)

(
  export DEPLOYMENT_ENV=production
  export BACKUP_REQUIRE_OFFSITE=false
  export BACKUP_OFFSITE_DIRECTORY=
  export BACKUP_AGE_RECIPIENT=
  . "$library"
  if validate_offsite; then
    fail "production backup accepted BACKUP_REQUIRE_OFFSITE=false"
  fi
)

(
  invalid_work=/tmp/backup-test-invalid-identity
  rm -rf "$invalid_work"
  mkdir -p "$invalid_work/offsite"
  export BACKUP_REQUIRE_OFFSITE=true
  export BACKUP_OFFSITE_DIRECTORY="$invalid_work/offsite"
  export BACKUP_AGE_RECIPIENT=not-an-age-recipient
  . "$library"
  if validate_offsite; then
    fail "required offsite backup accepted an invalid age recipient"
  fi
  rm -rf "$invalid_work"
)

health_work=/tmp/backup-test-health
rm -rf "$health_work"
mkdir -p "$health_work"
printf '%s\n' "$(date -u +%s)" > "$health_work/last-backup-success"
if (
  cd "$health_work"
    BACKUP_REQUIRE_OFFSITE=true \
    BACKUP_OFFSITE_DIRECTORY= \
    BACKUP_AGE_RECIPIENT= \
    /usr/local/bin/scheduler-backup-health
); then
  fail "healthcheck accepted missing required offsite configuration"
fi
rm -rf "$health_work"

if DEPLOYMENT_ENV=production \
  BACKUP_REQUIRE_OFFSITE=false \
  /usr/local/bin/scheduler-backup-health; then
  fail "production healthcheck accepted BACKUP_REQUIRE_OFFSITE=false"
fi

grep -q -- '--no-privileges' /usr/local/bin/scheduler-verify-backup ||
  fail "restore verification attempts to replay environment-specific ACLs"
grep -q 'export PGSSLMODE=' /usr/local/bin/scheduler-backup-loop ||
  fail "backup loop does not apply DATABASE_SSLMODE to libpq"
grep -q 'export PGSSLMODE=' /usr/local/bin/scheduler-verify-backup ||
  fail "restore verification does not apply DATABASE_SSLMODE to libpq"

echo "backup failure-injection tests passed"
