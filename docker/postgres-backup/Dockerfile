FROM postgres:16-alpine

RUN apk add --no-cache age openssl

COPY docker/postgres-backup/backup-loop.sh /usr/local/bin/scheduler-backup-loop
COPY docker/postgres-backup/generation-lib.sh /usr/local/lib/scheduler-backup-generation-lib.sh
COPY docker/postgres-backup/backup-lib.sh /usr/local/lib/scheduler-backup-lib.sh
COPY docker/postgres-backup/restore-lib.sh /usr/local/lib/scheduler-restore-lib.sh
COPY docker/postgres-backup/backup-tests.sh /usr/local/bin/scheduler-backup-tests
COPY docker/postgres-backup/verify-latest.sh /usr/local/bin/scheduler-verify-backup
COPY docker/postgres-backup/restore-latest.sh /usr/local/bin/scheduler-restore-backup
COPY docker/postgres-backup/healthcheck.sh /usr/local/bin/scheduler-backup-health
COPY docker/postgres-backup/preflight.sh /usr/local/bin/scheduler-backup-preflight
COPY docker/postgres-bootstrap/bootstrap.sh /usr/local/bin/scheduler-postgres-bootstrap
COPY migration /app/migrations

RUN chmod 0444 /usr/local/lib/scheduler-backup-generation-lib.sh /usr/local/lib/scheduler-backup-lib.sh /usr/local/lib/scheduler-restore-lib.sh \
    && chmod 0555 /usr/local/bin/scheduler-backup-loop /usr/local/bin/scheduler-backup-tests /usr/local/bin/scheduler-verify-backup /usr/local/bin/scheduler-restore-backup /usr/local/bin/scheduler-backup-health /usr/local/bin/scheduler-backup-preflight /usr/local/bin/scheduler-postgres-bootstrap \
    && find /app/migrations -type d -exec chmod 0555 {} + \
    && find /app/migrations -type f -exec chmod 0444 {} + \
    && mkdir -p /backups \
    && chown postgres:postgres /backups

USER postgres
ENTRYPOINT ["/usr/local/bin/scheduler-backup-loop"]
