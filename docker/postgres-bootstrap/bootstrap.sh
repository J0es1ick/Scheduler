#!/bin/sh
set -eu

: "${POSTGRES_SUPERUSER:=postgres}"
: "${DATABASE_NAME:=scheduler}"
: "${DATABASE_MIGRATOR_USER:=scheduler_migrator}"
: "${DATABASE_BOT_USER:=scheduler_bot}"
: "${DATABASE_ADMIN_USER:=scheduler_admin}"
: "${DATABASE_SITE_USER:=scheduler_site}"
: "${DATABASE_BACKUP_USER:=scheduler_backup}"
: "${DATABASE_RESTORE_USER:=scheduler_restore}"

export PGPASSWORD="${POSTGRES_SUPERUSER_PASSWORD:?POSTGRES_SUPERUSER_PASSWORD is required}"

psql_super() {
  database="$1"
  shift
  psql --host postgres --port 5432 --username "$POSTGRES_SUPERUSER" \
    --dbname "$database" --set ON_ERROR_STOP=1 "$@"
}

psql_super postgres \
  --set database="$DATABASE_NAME" \
  --set migrator="$DATABASE_MIGRATOR_USER" \
  --set migrator_password="${DATABASE_MIGRATOR_PASSWORD:?DATABASE_MIGRATOR_PASSWORD is required}" \
  --set bot="$DATABASE_BOT_USER" \
  --set bot_password="${DATABASE_BOT_PASSWORD:?DATABASE_BOT_PASSWORD is required}" \
  --set admin="$DATABASE_ADMIN_USER" \
  --set admin_password="${DATABASE_ADMIN_PASSWORD:?DATABASE_ADMIN_PASSWORD is required}" \
  --set site="$DATABASE_SITE_USER" \
  --set site_password="${DATABASE_SITE_PASSWORD:?DATABASE_SITE_PASSWORD is required}" \
  --set backup="$DATABASE_BACKUP_USER" \
  --set backup_password="${DATABASE_BACKUP_PASSWORD:?DATABASE_BACKUP_PASSWORD is required}" \
  --set restore="$DATABASE_RESTORE_USER" \
  --set restore_password="${DATABASE_RESTORE_PASSWORD:?DATABASE_RESTORE_PASSWORD is required}" <<'SQL'
SELECT format('CREATE ROLE %I LOGIN', :'migrator')
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname=:'migrator') \gexec
SELECT format('CREATE ROLE %I LOGIN', :'bot')
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname=:'bot') \gexec
SELECT format('CREATE ROLE %I LOGIN', :'admin')
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname=:'admin') \gexec
SELECT format('CREATE ROLE %I LOGIN', :'site')
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname=:'site') \gexec
SELECT format('CREATE ROLE %I LOGIN', :'backup')
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname=:'backup') \gexec
SELECT format('CREATE ROLE %I LOGIN', :'restore')
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname=:'restore') \gexec

SELECT format('ALTER ROLE %I LOGIN PASSWORD %L NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION', :'migrator', :'migrator_password') \gexec
SELECT format('ALTER ROLE %I LOGIN PASSWORD %L NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION', :'bot', :'bot_password') \gexec
SELECT format('ALTER ROLE %I LOGIN PASSWORD %L NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION', :'admin', :'admin_password') \gexec
SELECT format('ALTER ROLE %I LOGIN PASSWORD %L NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION', :'site', :'site_password') \gexec
SELECT format('ALTER ROLE %I LOGIN PASSWORD %L NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION', :'backup', :'backup_password') \gexec
SELECT format('ALTER ROLE %I LOGIN PASSWORD %L NOSUPERUSER CREATEDB NOCREATEROLE NOREPLICATION', :'restore', :'restore_password') \gexec

SELECT format('ALTER DATABASE %I OWNER TO %I', :'database', :'migrator') \gexec
SELECT format('GRANT CONNECT ON DATABASE %I TO %I, %I, %I, %I, %I, %I',
  :'database', :'migrator', :'bot', :'admin', :'site', :'backup', :'restore') \gexec
SQL

psql_super "$DATABASE_NAME" \
  --set migrator="$DATABASE_MIGRATOR_USER" \
  --set bot="$DATABASE_BOT_USER" \
  --set admin="$DATABASE_ADMIN_USER" \
  --set site="$DATABASE_SITE_USER" \
  --set backup="$DATABASE_BACKUP_USER" <<'SQL'
SELECT format('ALTER SCHEMA public OWNER TO %I', :'migrator') \gexec
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
SELECT format('GRANT USAGE ON SCHEMA public TO %I, %I, %I, %I', :'bot', :'admin', :'site', :'backup') \gexec

SELECT format(
  'ALTER %s %I.%I OWNER TO %I',
  CASE c.relkind
    WHEN 'r' THEN 'TABLE'
    WHEN 'p' THEN 'TABLE'
    WHEN 'S' THEN 'SEQUENCE'
    WHEN 'v' THEN 'VIEW'
    WHEN 'm' THEN 'MATERIALIZED VIEW'
  END,
  n.nspname,
  c.relname,
  :'migrator'
)
FROM pg_class c
JOIN pg_namespace n ON n.oid=c.relnamespace
WHERE n.nspname='public' AND c.relkind IN ('r', 'p', 'S', 'v', 'm')
  AND pg_get_userbyid(c.relowner) <> :'migrator' \gexec

SELECT format('ALTER TYPE %I.%I OWNER TO %I', n.nspname, t.typname, :'migrator')
FROM pg_type t
JOIN pg_namespace n ON n.oid=t.typnamespace
WHERE n.nspname='public' AND t.typtype IN ('d', 'e')
  AND pg_get_userbyid(t.typowner) <> :'migrator' \gexec

SELECT format('GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO %I, %I', :'bot', :'admin') \gexec
SELECT format('GRANT USAGE, SELECT, UPDATE ON ALL SEQUENCES IN SCHEMA public TO %I, %I', :'bot', :'admin') \gexec
SELECT format('GRANT SELECT ON ALL TABLES IN SCHEMA public TO %I, %I', :'site', :'backup') \gexec
SELECT format('GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO %I, %I', :'site', :'backup') \gexec

SELECT format('ALTER DEFAULT PRIVILEGES FOR ROLE %I IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO %I, %I', :'migrator', :'bot', :'admin') \gexec
SELECT format('ALTER DEFAULT PRIVILEGES FOR ROLE %I IN SCHEMA public GRANT USAGE, SELECT, UPDATE ON SEQUENCES TO %I, %I', :'migrator', :'bot', :'admin') \gexec
SELECT format('ALTER DEFAULT PRIVILEGES FOR ROLE %I IN SCHEMA public GRANT SELECT ON TABLES TO %I, %I', :'migrator', :'site', :'backup') \gexec
SELECT format('ALTER DEFAULT PRIVILEGES FOR ROLE %I IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO %I, %I', :'migrator', :'site', :'backup') \gexec
SQL

echo "PostgreSQL application roles and privileges are ready."
