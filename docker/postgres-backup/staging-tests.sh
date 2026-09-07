#!/bin/sh
set -eu
. /usr/local/lib/scheduler-backup-generation-lib.sh

work="$(mktemp -d "${BACKUP_STAGING_DIRECTORY:-/restore-staging}/test.XXXXXX")"
trap 'cleanup_staged_backup; rm -f "$work/scheduler-test.dump" "$work/scheduler-test.dump.sha256" "$work/receipt"; rmdir "$work"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
original="$work/scheduler-test.dump"
printf 'generation A\n' > "$original"
sha256sum "$original" > "${original}.sha256"
archive="$original"
stage_backup_archive "$original"
[ "$archive" = "$original" ]
expected_digest="$staged_backup_digest"
printf 'generation B\n' > "$original"
sha256sum "$original" > "${original}.sha256"
[ "$(cat "$staged_backup_archive")" = 'generation A' ]
write_backup_receipt "$work/receipt" "$staged_backup_archive" "$expected_digest"
read_backup_receipt "$work/receipt"
[ "$receipt_archive_digest" = "$expected_digest" ]
[ "$receipt_archive_digest" != "$(backup_archive_digest "$original")" ]
cleanup_staged_backup
if stage_backup_archive "$original" "$work/receipt"; then
  echo 'Substituted generation accepted against old receipt' >&2
  exit 1
fi
printf 'corrupt\n' > "$original"
if stage_backup_archive "$original"; then
  echo 'Corrupt archive accepted' >&2
  exit 1
fi
sha256sum "$original" > "${original}.sha256"
cp() { return 1; }
if stage_backup_archive "$original"; then exit 1; fi
[ -z "${staged_backup_directory:-}" ]
echo 'Backup staging regression tests passed'
