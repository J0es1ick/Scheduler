FROM postgres:16-alpine

COPY backup-loop.sh /usr/local/bin/scheduler-backup-loop
COPY verify-latest.sh /usr/local/bin/scheduler-verify-backup
COPY healthcheck.sh /usr/local/bin/scheduler-backup-health

RUN chmod 0555 /usr/local/bin/scheduler-backup-loop /usr/local/bin/scheduler-verify-backup /usr/local/bin/scheduler-backup-health \
    && mkdir -p /backups \
    && chown postgres:postgres /backups

USER postgres
ENTRYPOINT ["/usr/local/bin/scheduler-backup-loop"]
