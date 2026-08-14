FROM postgres:16-alpine

RUN apk add --no-cache openssl

COPY backup-loop.sh /usr/local/bin/scheduler-backup-loop
COPY backup-lib.sh /usr/local/lib/scheduler-backup-lib.sh
COPY backup-tests.sh /usr/local/bin/scheduler-backup-tests
COPY verify-latest.sh /usr/local/bin/scheduler-verify-backup
COPY healthcheck.sh /usr/local/bin/scheduler-backup-health

RUN chmod 0444 /usr/local/lib/scheduler-backup-lib.sh \
    && chmod 0555 /usr/local/bin/scheduler-backup-loop /usr/local/bin/scheduler-backup-tests /usr/local/bin/scheduler-verify-backup /usr/local/bin/scheduler-backup-health \
    && mkdir -p /backups \
    && chown postgres:postgres /backups

USER postgres
ENTRYPOINT ["/usr/local/bin/scheduler-backup-loop"]
