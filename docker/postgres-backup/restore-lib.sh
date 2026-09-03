#!/bin/sh

. /usr/local/lib/scheduler-backup-generation-lib.sh

validate_migration_manifest_files() {
  expected="$1"
  restored="$2"
  verify_checksums="$3"
  awk -F '|' -v verify_checksums="$verify_checksums" '
    NR == FNR {
      expected_count++
      expected_name[expected_count] = $1
      expected_checksum[expected_count] = $2
      next
    }
    {
      restored_count++
      if (restored_count > expected_count) {
        printf "unknown restored migration: %s\n", $1 > "/dev/stderr"
        failed = 1
        next
      }
      if ($1 != expected_name[restored_count]) {
        printf "migration sequence mismatch at position %d: expected %s, restored %s\n", restored_count, expected_name[restored_count], $1 > "/dev/stderr"
        failed = 1
      } else if (verify_checksums == "true" && $2 != expected_checksum[restored_count]) {
        printf "migration checksum mismatch: %s\n", $1 > "/dev/stderr"
        failed = 1
      }
    }
    END {
      if (restored_count == 0) {
        print "restored migration manifest is empty" > "/dev/stderr"
        failed = 1
      }
      exit failed ? 1 : 0
    }
  ' "$expected" "$restored"
}

write_expected_migration_manifest() {
  migrations_path="$1"
  output="$2"
  temporary="${output}.unsorted"
  : > "$temporary"
  for migration in "$migrations_path"/*.up.sql; do
    [ -f "$migration" ] || {
      rm -f "$temporary"
      echo "no migrations found in $migrations_path" >&2
      return 1
    }
    name="$(basename "$migration")"
    digest="$(sha256sum "$migration" | awk '{print $1}')"
    printf '%s|%s\n' "$name" "$digest" >> "$temporary"
  done
  sort "$temporary" > "$output"
  rm -f "$temporary"
}

restore_archive_into_database() {
  restore_database="$1"
  shift
  pg_restore \
    --exit-on-error \
    --no-owner \
    --no-privileges \
    --host "$DATABASE_HOST" \
    --port "$DATABASE_PORT" \
    --username "$DATABASE_USER" \
    --dbname "$restore_database" \
    "$@"
}

restore_backup_into_database() {
  restore_kind="$1"
  restore_archive="$2"
  restore_database="$3"
  restore_decrypt_marker="${TMPDIR:-/tmp}/scheduler-restore-decrypt-$$-${restore_database}"
  rm -f "$restore_decrypt_marker"
  case "$restore_kind" in
    plain)
      restore_archive_into_database "$restore_database" "$restore_archive"
      return $?
      ;;
    age)
      if (age --decrypt --identity "$BACKUP_AGE_IDENTITY_FILE" "$restore_archive" || {
        : > "$restore_decrypt_marker"
        exit 1
      }) | restore_archive_into_database "$restore_database"; then
        restore_pipeline_status=0
      else
        restore_pipeline_status=$?
      fi
      if [ -e "$restore_decrypt_marker" ]; then
        rm -f "$restore_decrypt_marker"
        echo "age backup decryption failed" >&2
        return 1
      fi
      return "$restore_pipeline_status"
      ;;
    legacy)
      if (openssl enc -d -aes-256-cbc -pbkdf2 \
        -pass "file:${BACKUP_ENCRYPTION_PASSPHRASE_FILE}" \
        -in "$restore_archive" || {
          : > "$restore_decrypt_marker"
          exit 1
        }) | restore_archive_into_database "$restore_database"; then
        restore_pipeline_status=0
      else
        restore_pipeline_status=$?
      fi
      if [ -e "$restore_decrypt_marker" ]; then
        rm -f "$restore_decrypt_marker"
        echo "legacy backup decryption failed" >&2
        return 1
      fi
      return "$restore_pipeline_status"
      ;;
    *)
      echo "unsupported backup kind: $restore_kind" >&2
      return 1
      ;;
  esac
}

validate_restored_scheduler_database() {
  restored_database="$1"
  restored_migrations_path="$2"
  validation_suffix="$$-$(date -u +%s)"
  validation_expected="${TMPDIR:-/tmp}/scheduler-expected-${validation_suffix}"
  validation_actual="${TMPDIR:-/tmp}/scheduler-actual-${validation_suffix}"
  validation_cleanup() {
    rm -f "$validation_expected" "${validation_expected}.unsorted" "$validation_actual"
  }

  validation_migrations="$(psql --host "$DATABASE_HOST" --port "$DATABASE_PORT" --username "$DATABASE_USER" \
    --dbname "$restored_database" --tuples-only --no-align --command='SELECT COUNT(*) FROM schema_migrations')" || {
      validation_cleanup
      return 1
    }
  validation_users="$(psql --host "$DATABASE_HOST" --port "$DATABASE_PORT" --username "$DATABASE_USER" \
    --dbname "$restored_database" --tuples-only --no-align --command="SELECT to_regclass('public.users') IS NOT NULL")" || {
      validation_cleanup
      return 1
    }
  if [ "${validation_migrations:-0}" -lt 1 ] || [ "$validation_users" != t ]; then
    validation_cleanup
    echo "restored database failed validation" >&2
    return 1
  fi

  write_expected_migration_manifest "$restored_migrations_path" "$validation_expected" || {
    validation_cleanup
    return 1
  }
  validation_has_checksums="$(psql --host "$DATABASE_HOST" --port "$DATABASE_PORT" --username "$DATABASE_USER" \
    --dbname "$restored_database" --tuples-only --no-align --command="SELECT EXISTS (
      SELECT 1 FROM information_schema.columns
      WHERE table_schema=current_schema() AND table_name='schema_migrations' AND column_name='checksum'
    )")" || {
      validation_cleanup
      return 1
    }
  if [ "$validation_has_checksums" = t ]; then
    psql --host "$DATABASE_HOST" --port "$DATABASE_PORT" --username "$DATABASE_USER" \
      --dbname "$restored_database" --tuples-only --no-align --field-separator='|' \
      --command='SELECT name, checksum FROM schema_migrations ORDER BY name' > "$validation_actual" || {
        validation_cleanup
        return 1
      }
    validation_verify_checksums=true
  else
    psql --host "$DATABASE_HOST" --port "$DATABASE_PORT" --username "$DATABASE_USER" \
      --dbname "$restored_database" --tuples-only --no-align \
      --command='SELECT name FROM schema_migrations ORDER BY name' > "$validation_actual" || {
        validation_cleanup
        return 1
      }
    validation_verify_checksums=false
  fi
  if ! validate_migration_manifest_files "$validation_expected" "$validation_actual" "$validation_verify_checksums"; then
    validation_cleanup
    echo "restored schema contains unknown, reordered or modified migrations" >&2
    return 1
  fi
  validation_cleanup
  validated_migration_count="$validation_migrations"
  return 0
}
