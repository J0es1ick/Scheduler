#!/bin/sh
set -eu

: "${POSTGRES_SUPERUSER:=postgres}"
: "${DATABASE_HOST:=postgres}"
: "${DATABASE_PORT:=5432}"
: "${DATABASE_NAME:=scheduler}"
: "${DATABASE_SSLMODE:=prefer}"
: "${DATABASE_MIGRATOR_USER:=scheduler_migrator}"
: "${DATABASE_BOT_USER:=scheduler_bot}"
: "${DATABASE_ADMIN_USER:=scheduler_admin}"
: "${DATABASE_PARSER_USER:=scheduler_parser}"
: "${DATABASE_PRIVACY_USER:=scheduler_privacy}"
: "${DATABASE_SITE_USER:=scheduler_site}"
: "${DATABASE_BACKUP_USER:=scheduler_backup}"
: "${DATABASE_RESTORE_USER:=scheduler_restore}"

DATABASE_SITE_READER_ROLE="scheduler_public_reader"

postgres_superuser_password="${POSTGRES_SUPERUSER_PASSWORD:?POSTGRES_SUPERUSER_PASSWORD is required}"
database_migrator_password="${DATABASE_MIGRATOR_PASSWORD:?DATABASE_MIGRATOR_PASSWORD is required}"
database_bot_password="${DATABASE_BOT_PASSWORD:?DATABASE_BOT_PASSWORD is required}"
database_admin_password="${DATABASE_ADMIN_PASSWORD:?DATABASE_ADMIN_PASSWORD is required}"
database_parser_password="${DATABASE_PARSER_PASSWORD:?DATABASE_PARSER_PASSWORD is required}"
database_privacy_password="${DATABASE_PRIVACY_PASSWORD:?DATABASE_PRIVACY_PASSWORD is required}"
database_site_password="${DATABASE_SITE_PASSWORD:?DATABASE_SITE_PASSWORD is required}"
database_backup_password="${DATABASE_BACKUP_PASSWORD:?DATABASE_BACKUP_PASSWORD is required}"
database_restore_password="${DATABASE_RESTORE_PASSWORD:?DATABASE_RESTORE_PASSWORD is required}"

export PGPASSWORD="$postgres_superuser_password"
export PGSSLMODE="$DATABASE_SSLMODE"

seen_roles=""
seen_password_fingerprints=""
validate_role() {
  role="$1"
  case "$role" in
    ''|[0-9]*|*[!A-Za-z0-9_]*)
      echo "invalid PostgreSQL role name: $role" >&2
      exit 1
      ;;
    *) ;;
  esac
  if [ "${#role}" -gt 63 ]; then
    echo "PostgreSQL role name is longer than 63 characters: $role" >&2
    exit 1
  fi
  case " $seen_roles " in
    *" $role "*)
      echo "PostgreSQL bootstrap roles must be distinct: $role" >&2
      exit 1
      ;;
  esac
  seen_roles="$seen_roles $role"
}

validate_password() {
  name="$1"
  value="$2"
  if [ "${#value}" -lt 24 ]; then
    echo "$name must contain at least 24 characters" >&2
    exit 1
  fi
  normalized="$(printf '%s' "$value" | tr '[:lower:]' '[:upper:]')"
  case "$normalized" in
    *CHANGE_ME*|*PASTE_*)
      echo "$name must not contain a placeholder" >&2
      exit 1
      ;;
  esac
  fingerprint="$(printf '%s' "$value" | sha256sum | cut -d ' ' -f 1)"
  case " $seen_password_fingerprints " in
    *" $fingerprint "*)
      echo "$name must be distinct from every other database password" >&2
      exit 1
      ;;
  esac
  seen_password_fingerprints="$seen_password_fingerprints $fingerprint"
}

validate_role "$POSTGRES_SUPERUSER"
validate_role "$DATABASE_MIGRATOR_USER"
validate_role "$DATABASE_BOT_USER"
validate_role "$DATABASE_ADMIN_USER"
validate_role "$DATABASE_PARSER_USER"
validate_role "$DATABASE_PRIVACY_USER"
validate_role "$DATABASE_SITE_USER"
validate_role "$DATABASE_BACKUP_USER"
validate_role "$DATABASE_RESTORE_USER"
validate_role "$DATABASE_SITE_READER_ROLE"

validate_password POSTGRES_SUPERUSER_PASSWORD "$postgres_superuser_password"
validate_password DATABASE_MIGRATOR_PASSWORD "$database_migrator_password"
validate_password DATABASE_BOT_PASSWORD "$database_bot_password"
validate_password DATABASE_ADMIN_PASSWORD "$database_admin_password"
validate_password DATABASE_PARSER_PASSWORD "$database_parser_password"
validate_password DATABASE_PRIVACY_PASSWORD "$database_privacy_password"
validate_password DATABASE_SITE_PASSWORD "$database_site_password"
validate_password DATABASE_BACKUP_PASSWORD "$database_backup_password"
validate_password DATABASE_RESTORE_PASSWORD "$database_restore_password"

psql_super() {
  database="$1"
  shift
  psql --host "$DATABASE_HOST" --port "$DATABASE_PORT" --username "$POSTGRES_SUPERUSER" \
    --dbname "$database" --set ON_ERROR_STOP=1 "$@"
}

connected_role="$(psql_super "$DATABASE_NAME" --tuples-only --no-align --field-separator='|' \
  --command="SELECT current_user, rolsuper FROM pg_roles WHERE rolname=current_user")"
if [ "$connected_role" != "$POSTGRES_SUPERUSER|t" ]; then
  echo "POSTGRES_SUPERUSER must be the actual connected PostgreSQL superuser" >&2
  exit 1
fi
unsafe_attributes="$(psql_super "$DATABASE_NAME" --tuples-only --no-align \
  --set migrator="$DATABASE_MIGRATOR_USER" \
  --set bot="$DATABASE_BOT_USER" \
  --set admin="$DATABASE_ADMIN_USER" \
  --set parser="$DATABASE_PARSER_USER" \
  --set privacy="$DATABASE_PRIVACY_USER" \
  --set site="$DATABASE_SITE_USER" \
  --set site_reader="$DATABASE_SITE_READER_ROLE" \
  --set backup="$DATABASE_BACKUP_USER" \
  --set restore="$DATABASE_RESTORE_USER" <<'SQL'
SELECT COALESCE(string_agg(
  format('%s[super=%s,createdb=%s,createrole=%s,replication=%s,bypassrls=%s,login=%s]',
    rolname, rolsuper, rolcreatedb, rolcreaterole, rolreplication, rolbypassrls, rolcanlogin),
  ',' ORDER BY rolname), '')
FROM pg_roles
WHERE rolname IN (:'migrator', :'bot', :'admin', :'parser', :'privacy', :'site', :'site_reader', :'backup', :'restore')
  AND (
    rolsuper OR rolcreaterole OR rolreplication OR rolbypassrls
    OR (rolcreatedb AND rolname <> :'restore')
    OR NOT rolinherit
    OR (rolname = :'site_reader' AND rolcanlogin)
    OR (rolname <> :'site_reader' AND NOT rolcanlogin)
  );
SQL
)"
if [ -n "$unsafe_attributes" ]; then
  echo "application roles have unsafe attributes: $unsafe_attributes" >&2
  exit 1
fi
unsafe_memberships="$(psql_super "$DATABASE_NAME" --tuples-only --no-align \
  --set migrator="$DATABASE_MIGRATOR_USER" \
  --set bot="$DATABASE_BOT_USER" \
  --set admin="$DATABASE_ADMIN_USER" \
  --set parser="$DATABASE_PARSER_USER" \
  --set privacy="$DATABASE_PRIVACY_USER" \
  --set site="$DATABASE_SITE_USER" \
  --set site_reader="$DATABASE_SITE_READER_ROLE" \
  --set backup="$DATABASE_BACKUP_USER" \
  --set restore="$DATABASE_RESTORE_USER" <<'SQL'
SELECT COALESCE(string_agg(member_role.rolname || '->' || granted_role.rolname, ',' ORDER BY member_role.rolname, granted_role.rolname), '')
FROM pg_auth_members membership
JOIN pg_roles granted_role ON granted_role.oid=membership.roleid
JOIN pg_roles member_role ON member_role.oid=membership.member
WHERE (
    member_role.rolname IN (:'migrator', :'bot', :'admin', :'parser', :'privacy', :'site', :'site_reader', :'backup', :'restore')
    OR granted_role.rolname IN (:'migrator', :'bot', :'admin', :'parser', :'privacy', :'site', :'site_reader', :'backup', :'restore')
  )
  AND NOT (member_role.rolname=:'site' AND granted_role.rolname=:'site_reader');
SQL
)"
if [ -n "$unsafe_memberships" ]; then
  echo "application roles have unexpected role memberships: $unsafe_memberships" >&2
  exit 1
fi

psql_super "$DATABASE_NAME" --single-transaction \
  --set database="$DATABASE_NAME" \
  --set migrator="$DATABASE_MIGRATOR_USER" \
  --set migrator_password="$database_migrator_password" \
  --set bot="$DATABASE_BOT_USER" \
  --set bot_password="$database_bot_password" \
  --set admin="$DATABASE_ADMIN_USER" \
  --set admin_password="$database_admin_password" \
  --set parser="$DATABASE_PARSER_USER" \
  --set parser_password="$database_parser_password" \
  --set privacy="$DATABASE_PRIVACY_USER" \
  --set privacy_password="$database_privacy_password" \
  --set site="$DATABASE_SITE_USER" \
  --set site_password="$database_site_password" \
  --set site_reader="$DATABASE_SITE_READER_ROLE" \
  --set backup="$DATABASE_BACKUP_USER" \
  --set backup_password="$database_backup_password" \
  --set restore="$DATABASE_RESTORE_USER" \
  --set restore_password="$database_restore_password" <<'SQL'
SELECT format('CREATE ROLE %I LOGIN', :'migrator')
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname=:'migrator') \gexec
SELECT format('CREATE ROLE %I LOGIN', :'bot')
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname=:'bot') \gexec
SELECT format('CREATE ROLE %I LOGIN', :'admin')
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname=:'admin') \gexec
SELECT format('CREATE ROLE %I LOGIN', :'parser')
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname=:'parser') \gexec
SELECT format('CREATE ROLE %I LOGIN', :'privacy')
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname=:'privacy') \gexec
SELECT format('CREATE ROLE %I LOGIN', :'site')
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname=:'site') \gexec
SELECT format('CREATE ROLE %I NOLOGIN', :'site_reader')
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname=:'site_reader') \gexec
SELECT format('CREATE ROLE %I LOGIN', :'backup')
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname=:'backup') \gexec
SELECT format('CREATE ROLE %I LOGIN', :'restore')
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname=:'restore') \gexec

SELECT format('ALTER ROLE %I LOGIN PASSWORD %L NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS INHERIT VALID UNTIL %L', :'migrator', :'migrator_password', 'infinity') \gexec
SELECT format('ALTER ROLE %I LOGIN PASSWORD %L NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS INHERIT VALID UNTIL %L', :'bot', :'bot_password', 'infinity') \gexec
SELECT format('ALTER ROLE %I LOGIN PASSWORD %L NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS INHERIT VALID UNTIL %L', :'admin', :'admin_password', 'infinity') \gexec
SELECT format('ALTER ROLE %I LOGIN PASSWORD %L NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS INHERIT VALID UNTIL %L', :'parser', :'parser_password', 'infinity') \gexec
SELECT format('ALTER ROLE %I LOGIN PASSWORD %L NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS INHERIT VALID UNTIL %L', :'privacy', :'privacy_password', 'infinity') \gexec
SELECT format('ALTER ROLE %I LOGIN PASSWORD %L NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS INHERIT VALID UNTIL %L', :'site', :'site_password', 'infinity') \gexec
SELECT format('ALTER ROLE %I NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS INHERIT', :'site_reader') \gexec
SELECT format('ALTER ROLE %I LOGIN PASSWORD %L NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS INHERIT VALID UNTIL %L', :'backup', :'backup_password', 'infinity') \gexec
SELECT format('ALTER ROLE %I LOGIN PASSWORD %L NOSUPERUSER CREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS INHERIT VALID UNTIL %L', :'restore', :'restore_password', 'infinity') \gexec
SELECT format('GRANT %I TO %I', :'site_reader', :'site') \gexec

SELECT format('ALTER DATABASE %I OWNER TO %I', :'database', :'migrator') \gexec
SELECT format('GRANT CONNECT ON DATABASE %I TO %I, %I, %I, %I, %I, %I, %I, %I',
  :'database', :'migrator', :'bot', :'admin', :'parser', :'privacy', :'site', :'backup', :'restore') \gexec

SELECT format('ALTER SCHEMA public OWNER TO %I', :'migrator') \gexec
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
SELECT format('REVOKE TEMPORARY ON DATABASE %I FROM PUBLIC', :'database') \gexec
SELECT format('REVOKE TEMPORARY ON DATABASE %I FROM %I, %I, %I, %I, %I, %I', :'database', :'bot', :'admin', :'parser', :'privacy', :'site', :'backup') \gexec
SELECT format('GRANT TEMPORARY ON DATABASE %I TO %I, %I', :'database', :'migrator', :'restore') \gexec
SELECT format('GRANT USAGE ON SCHEMA public TO %I, %I, %I, %I, %I, %I, %I', :'bot', :'admin', :'parser', :'privacy', :'site', :'site_reader', :'backup') \gexec

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

SELECT format(
  'ALTER %s %I.%I(%s) OWNER TO %I',
  CASE p.prokind WHEN 'p' THEN 'PROCEDURE' ELSE 'FUNCTION' END,
  n.nspname,
  p.proname,
  pg_get_function_identity_arguments(p.oid),
  :'migrator'
)
FROM pg_proc p
JOIN pg_namespace n ON n.oid=p.pronamespace
WHERE n.nspname='public' AND p.prokind IN ('f', 'p')
  AND pg_get_userbyid(p.proowner) <> :'migrator'
  AND NOT EXISTS (
    SELECT 1 FROM pg_depend d
    WHERE d.classid='pg_proc'::regclass AND d.objid=p.oid AND d.deptype='e'
  ) \gexec

SELECT format('ALTER TYPE %I.%I OWNER TO %I', n.nspname, t.typname, :'migrator')
FROM pg_type t
JOIN pg_namespace n ON n.oid=t.typnamespace
WHERE n.nspname='public' AND t.typtype IN ('d', 'e')
  AND pg_get_userbyid(t.typowner) <> :'migrator' \gexec

SELECT format('REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA public FROM %I', :'site') \gexec
SELECT format('REVOKE ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public FROM %I', :'site') \gexec
SELECT format('REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA public FROM %I', :'site_reader') \gexec
SELECT format('REVOKE ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public FROM %I', :'site_reader') \gexec
SELECT format('GRANT SELECT ON %I.%I TO %I', n.nspname, c.relname, :'site_reader')
FROM pg_class c
JOIN pg_namespace n ON n.oid=c.relnamespace
WHERE n.nspname='public' AND c.relkind IN ('v', 'm')
  AND c.relname IN ('public_site_statistics', 'public_site_universities', 'public_site_sources') \gexec
SELECT format('GRANT SELECT ON ALL TABLES IN SCHEMA public TO %I', :'backup') \gexec
SELECT format('GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO %I', :'backup') \gexec

SELECT format('ALTER DEFAULT PRIVILEGES FOR ROLE %I IN SCHEMA public REVOKE ALL ON TABLES FROM %I, %I, %I, %I', :'migrator', :'bot', :'admin', :'parser', :'privacy') \gexec
SELECT format('ALTER DEFAULT PRIVILEGES FOR ROLE %I IN SCHEMA public REVOKE ALL ON SEQUENCES FROM %I, %I, %I, %I', :'migrator', :'bot', :'admin', :'parser', :'privacy') \gexec
SELECT format('ALTER DEFAULT PRIVILEGES FOR ROLE %I IN SCHEMA public REVOKE ALL ON TABLES FROM %I, %I', :'migrator', :'site', :'site_reader') \gexec
SELECT format('ALTER DEFAULT PRIVILEGES FOR ROLE %I IN SCHEMA public REVOKE ALL ON SEQUENCES FROM %I, %I', :'migrator', :'site', :'site_reader') \gexec
SELECT format('ALTER DEFAULT PRIVILEGES FOR ROLE %I IN SCHEMA public GRANT SELECT ON TABLES TO %I', :'migrator', :'backup') \gexec
SELECT format('ALTER DEFAULT PRIVILEGES FOR ROLE %I IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO %I', :'migrator', :'backup') \gexec
SQL

echo "PostgreSQL application roles and privileges are ready."
