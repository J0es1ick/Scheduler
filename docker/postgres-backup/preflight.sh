#!/bin/sh
set -eu

. /usr/local/lib/scheduler-backup-lib.sh

validate_backup_settings
validate_offsite

echo "backup storage preflight completed"
