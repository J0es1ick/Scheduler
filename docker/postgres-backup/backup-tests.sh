#!/bin/sh
set -eu

library=/usr/local/lib/scheduler-backup-lib.sh
restore_library=/usr/local/lib/scheduler-restore-lib.sh

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
  printf '%s\n' scheduler-offsite > "$work/offsite/.scheduler-offsite"
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

retention_work=/tmp/backup-test-retention
rm -rf "$retention_work"
mkdir -p "$retention_work/offsite"
printf '%s\n' scheduler-offsite > "$retention_work/offsite/.scheduler-offsite"
printf '%s\n' unrelated > "$retention_work/offsite/keep.txt"
age-keygen --output "$retention_work/identity" >/dev/null 2>&1
retention_recipient="$(age-keygen -y "$retention_work/identity")"
for generation in 01 02 03 04 05; do
  archive="$retention_work/offsite/scheduler-202601${generation}T000000Z.dump.age"
  dd if=/dev/zero of="$archive" bs=1024 count=1 >/dev/null 2>&1
  (cd "$retention_work/offsite" && sha256sum "$(basename "$archive")" > "$(basename "${archive}.sha256")")
done
printf '%s\n' orphan > "$retention_work/offsite/scheduler-20251201T000000Z.dump.age"
printf '%s\n' fresh-orphan > "$retention_work/offsite/scheduler-20260106T000000Z.dump.age"
touch -t 202001010000 "$retention_work/offsite"/scheduler-2026010[1-3]T000000Z.dump.age*
touch -t 202001010000 "$retention_work/offsite/scheduler-20251201T000000Z.dump.age"
(
  export BACKUP_REQUIRE_OFFSITE=true
  export BACKUP_OFFSITE_DIRECTORY="$retention_work/offsite"
  export BACKUP_OFFSITE_RETENTION_MODE=filesystem
  export BACKUP_OFFSITE_RETENTION_DAYS=30
  export BACKUP_OFFSITE_MIN_GENERATIONS=2
  export BACKUP_OFFSITE_MAX_BYTES=2500
  export BACKUP_AGE_RECIPIENT="$retention_recipient"
  . "$library"
  validate_offsite || fail "filesystem retention configuration was rejected"
  run_offsite_retention || fail "filesystem retention failed"
  collect_offsite_stats || fail "offsite statistics failed"
  [ "$offsite_generation_count" -eq 2 ] || fail "retention did not preserve exactly two newest complete generations"
  [ "$offsite_total_bytes" -le "$BACKUP_OFFSITE_MAX_BYTES" ] || fail "retention did not enforce storage quota"
  [ -e "$BACKUP_OFFSITE_DIRECTORY/scheduler-20260104T000000Z.dump.age" ] || fail "retention deleted a protected generation"
  [ -e "$BACKUP_OFFSITE_DIRECTORY/scheduler-20260105T000000Z.dump.age" ] || fail "retention deleted the newest generation"
  [ ! -e "$BACKUP_OFFSITE_DIRECTORY/scheduler-20251201T000000Z.dump.age" ] || fail "retention kept an expired incomplete generation"
  [ -e "$BACKUP_OFFSITE_DIRECTORY/scheduler-20260106T000000Z.dump.age" ] || fail "retention deleted a fresh incomplete generation"
  [ -e "$BACKUP_OFFSITE_DIRECTORY/.scheduler-offsite" ] || fail "retention deleted the mount sentinel"
  [ -e "$BACKUP_OFFSITE_DIRECTORY/keep.txt" ] || fail "retention deleted an unrelated file"
)
rm -rf "$retention_work"

external_work=/tmp/backup-test-external-retention
rm -rf "$external_work"
mkdir -p "$external_work/offsite"
printf '%s\n' scheduler-offsite > "$external_work/offsite/.scheduler-offsite"
printf '%s\n' archive > "$external_work/offsite/scheduler-20250101T000000Z.dump.age"
age-keygen --output "$external_work/identity" >/dev/null 2>&1
external_recipient="$(age-keygen -y "$external_work/identity")"
(cd "$external_work/offsite" && sha256sum scheduler-20250101T000000Z.dump.age > scheduler-20250101T000000Z.dump.age.sha256)
touch -t 202001010000 "$external_work/offsite"/scheduler-20250101T000000Z.dump.age*
(
  export BACKUP_REQUIRE_OFFSITE=true
  export BACKUP_OFFSITE_DIRECTORY="$external_work/offsite"
  export BACKUP_OFFSITE_RETENTION_MODE=external
  export BACKUP_OFFSITE_RETENTION_DAYS=1
  export BACKUP_OFFSITE_MIN_GENERATIONS=1
  export BACKUP_OFFSITE_MAX_BYTES=1
  export BACKUP_AGE_RECIPIENT="$external_recipient"
  . "$library"
  run_offsite_retention || fail "external lifecycle mode failed"
  [ -e "$BACKUP_OFFSITE_DIRECTORY/scheduler-20250101T000000Z.dump.age" ] || fail "external lifecycle mode deleted provider-managed data"
)
rm -rf "$external_work"

manifest_work=/tmp/backup-test-manifest
rm -rf "$manifest_work"
mkdir -p "$manifest_work/backups"
. "$restore_library"
printf '%s\n' '001_first.up.sql|aaa' '002_second.up.sql|bbb' '003_third.up.sql|ccc' > "$manifest_work/expected"
printf '%s\n' '001_first.up.sql|aaa' '002_second.up.sql|bbb' > "$manifest_work/older"
validate_migration_manifest_files "$manifest_work/expected" "$manifest_work/older" true ||
  fail "an older migration prefix was rejected"
printf '%s\n' '001_first.up.sql|aaa' '003_third.up.sql|ccc' > "$manifest_work/reordered"
if validate_migration_manifest_files "$manifest_work/expected" "$manifest_work/reordered" true; then
  fail "a non-prefix migration manifest was accepted"
fi
printf '%s\n' '001_first.up.sql|changed' > "$manifest_work/changed"
if validate_migration_manifest_files "$manifest_work/expected" "$manifest_work/changed" true; then
  fail "a changed migration checksum was accepted"
fi
printf '%s\n' '001_first.up.sql' '002_second.up.sql' > "$manifest_work/legacy"
validate_migration_manifest_files "$manifest_work/expected" "$manifest_work/legacy" false ||
  fail "a legacy migration prefix was rejected"
printf '%s\n' valid > "$manifest_work/backups/scheduler-20260101T000000Z.dump"
(
  cd "$manifest_work/backups"
  sha256sum scheduler-20260101T000000Z.dump > scheduler-20260101T000000Z.dump.sha256
)
printf '%s\n' orphan > "$manifest_work/backups/scheduler-20260103T000000Z.dump"
selected="$(select_newest_valid_backup "$manifest_work/backups" 'scheduler-*.dump')"
[ "$(basename "$selected")" = scheduler-20260101T000000Z.dump ] ||
  fail "an orphan archive masked the newest complete backup"
printf '%s\n' before > "$manifest_work/backups/scheduler-20260102T000000Z.dump"
(
  cd "$manifest_work/backups"
  sha256sum scheduler-20260102T000000Z.dump > scheduler-20260102T000000Z.dump.sha256
)
printf '%s\n' after > "$manifest_work/backups/scheduler-20260102T000000Z.dump"
selected="$(select_newest_valid_backup "$manifest_work/backups" 'scheduler-*.dump')"
[ "$(basename "$selected")" = scheduler-20260101T000000Z.dump ] ||
  fail "a corrupted newest generation masked an older valid backup"

printf '%s\n' base > "$manifest_work/backups/scheduler-20260104T000000Z.dump"
printf '%s\n' collision-one > "$manifest_work/backups/scheduler-20260104T000000Z-1.dump"
printf '%s\n' collision-two > "$manifest_work/backups/scheduler-20260104T000000Z_000002.dump"
for ordered_archive in "$manifest_work/backups"/scheduler-20260104T000000Z*.dump; do
  (cd "$manifest_work/backups" && sha256sum "$(basename "$ordered_archive")" > "$(basename "${ordered_archive}.sha256")")
done
ordered_names="$(valid_backup_candidates "$manifest_work/backups" 'scheduler-20260104T000000Z*.dump' newest | xargs -n1 basename)"
expected_order="$(printf '%s\n' scheduler-20260104T000000Z_000002.dump scheduler-20260104T000000Z-1.dump scheduler-20260104T000000Z.dump)"
[ "$ordered_names" = "$expected_order" ] || fail "same-second generations are not ordered by their sequence"

receipt="$manifest_work/verified.receipt"
receipt_archive="$manifest_work/backups/scheduler-20260104T000000Z-1.dump"
write_backup_receipt "$receipt" "$receipt_archive" || fail "verification receipt could not be written"
receipt_selected="$(select_receipt_backup "$manifest_work/backups" 'scheduler-*.dump' "$receipt")"
[ "$receipt_selected" = "$receipt_archive" ] || fail "verification receipt did not pin the selected generation"
printf '%s\n' changed >> "$receipt_archive"
if select_receipt_backup "$manifest_work/backups" 'scheduler-*.dump' "$receipt" >/dev/null; then
  fail "verification receipt accepted a changed generation"
fi

fallback_candidates="$manifest_work/fallback-candidates"
printf '%s|%s\n' plain newest plain oldest > "$fallback_candidates"
fallback_calls=
fallback_probe() {
  fallback_calls="${fallback_calls}$2 "
  [ "$2" = oldest ]
}
select_first_usable_backup "$fallback_candidates" fallback_probe || fail "restore fallback did not find the older usable generation"
[ "$selected_backup_archive" = oldest ] || fail "restore fallback selected the wrong generation"
[ "$fallback_calls" = "newest oldest " ] || fail "restore fallback did not test generations newest first"
rm -rf "$manifest_work"

integrity_work=/tmp/backup-test-integrity-retention
rm -rf "$integrity_work"
mkdir -p "$integrity_work/offsite"
printf '%s\n' scheduler-offsite > "$integrity_work/offsite/.scheduler-offsite"
age-keygen --output "$integrity_work/identity" >/dev/null 2>&1
integrity_recipient="$(age-keygen -y "$integrity_work/identity")"
for integrity_generation in 01 02 03; do
  integrity_archive="$integrity_work/offsite/scheduler-202602${integrity_generation}T000000Z_000000.dump.age"
  printf 'generation-%s\n' "$integrity_generation" > "$integrity_archive"
  (cd "$integrity_work/offsite" && sha256sum "$(basename "$integrity_archive")" > "$(basename "${integrity_archive}.sha256")")
done
printf '%s\n' corrupted >> "$integrity_work/offsite/scheduler-20260203T000000Z_000000.dump.age"
touch -t 202001010000 "$integrity_work/offsite"/scheduler-*.dump.age*
(
  export BACKUP_REQUIRE_OFFSITE=true
  export BACKUP_OFFSITE_DIRECTORY="$integrity_work/offsite"
  export BACKUP_OFFSITE_RETENTION_MODE=filesystem
  export BACKUP_OFFSITE_RETENTION_DAYS=1
  export BACKUP_OFFSITE_MIN_GENERATIONS=2
  export BACKUP_OFFSITE_MAX_BYTES=0
  export BACKUP_AGE_RECIPIENT="$integrity_recipient"
  . "$library"
  run_offsite_retention || fail "integrity-aware retention failed"
  collect_offsite_stats || fail "integrity-aware statistics failed"
  [ "$offsite_generation_count" -eq 2 ] || fail "corrupted generation counted toward the protected minimum"
  [ ! -e "$BACKUP_OFFSITE_DIRECTORY/scheduler-20260203T000000Z_000000.dump.age" ] || fail "expired corrupted generation was retained as complete"
  [ -e "$BACKUP_OFFSITE_DIRECTORY/scheduler-20260201T000000Z_000000.dump.age" ] || fail "oldest valid protected generation was deleted"
  [ -e "$BACKUP_OFFSITE_DIRECTORY/scheduler-20260202T000000Z_000000.dump.age" ] || fail "newest valid protected generation was deleted"
)
rm -rf "$integrity_work"

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
  printf '%s\n' scheduler-offsite > "$invalid_work/offsite/.scheduler-offsite"
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
mkdir -p "$health_work/offsite"
printf '%s\n' scheduler-offsite > "$health_work/offsite/.scheduler-offsite"
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

grep -q -- '--no-privileges' "$restore_library" ||
  fail "restore verification attempts to replay environment-specific ACLs"
grep -q '| restore_archive_into_database' "$restore_library" ||
  fail "encrypted restore verification is not streamed into pg_restore"
if grep -q 'offsite-restore-verify.*dump' /usr/local/bin/scheduler-verify-backup; then
  fail "encrypted restore verification still materializes the dump in tmpfs"
fi
grep -q '| restore_archive_into_database' "$restore_library" ||
  fail "disaster recovery does not stream encrypted data into pg_restore"
grep -q 'scheduler-restore-lib.sh' /usr/local/bin/scheduler-verify-backup ||
  fail "restore verification does not use shared manifest validation"
grep -q 'BACKUP_VERIFY_RECEIPT_FILE' /usr/local/bin/scheduler-verify-backup ||
  fail "restore verification cannot pin a verified generation"
grep -q 'BACKUP_RESTORE_RECEIPT_FILE' /usr/local/bin/scheduler-restore-backup ||
  fail "disaster recovery does not consume the verification receipt"
grep -q 'export PGSSLMODE=' /usr/local/bin/scheduler-backup-loop ||
  fail "backup loop does not apply DATABASE_SSLMODE to libpq"
grep -q 'export PGSSLMODE=' /usr/local/bin/scheduler-verify-backup ||
  fail "restore verification does not apply DATABASE_SSLMODE to libpq"

assert_bootstrap_password_rejected() {
  name="$1"
  expected="$2"
  shift 2
  output="/tmp/bootstrap-password-${name}.log"
  if env \
    DATABASE_HOST=127.0.0.1 DATABASE_PORT=1 PGCONNECT_TIMEOUT=1 \
    POSTGRES_SUPERUSER_PASSWORD=bootstrap-superuser-password-01 \
    DATABASE_MIGRATOR_PASSWORD=bootstrap-migrator-password-02 \
    DATABASE_BOT_PASSWORD=bootstrap-bot-password-03 \
    DATABASE_ADMIN_PASSWORD=bootstrap-admin-password-04 \
    DATABASE_PARSER_PASSWORD=bootstrap-parser-password-08 \
    DATABASE_PRIVACY_PASSWORD=bootstrap-privacy-password-09 \
    DATABASE_SITE_PASSWORD=bootstrap-site-password-05 \
    DATABASE_BACKUP_PASSWORD=bootstrap-backup-password-06 \
    DATABASE_RESTORE_PASSWORD=bootstrap-restore-password-07 \
    "$@" /usr/local/bin/scheduler-postgres-bootstrap >"$output" 2>&1; then
    fail "$name bootstrap password case unexpectedly succeeded"
  fi
  grep -q "$expected" "$output" || fail "$name bootstrap password case failed for the wrong reason"
  rm -f "$output"
}

assert_bootstrap_password_rejected short-root "at least 24 characters" \
  POSTGRES_SUPERUSER_PASSWORD=short
assert_bootstrap_password_rejected placeholder-root "must not contain a placeholder" \
  POSTGRES_SUPERUSER_PASSWORD=CHANGE_ME_TO_A_REAL_SUPERUSER_PASSWORD
assert_bootstrap_password_rejected duplicate-root "must be distinct" \
  DATABASE_BOT_PASSWORD=bootstrap-superuser-password-01

echo "backup failure-injection tests passed"
