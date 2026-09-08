//go:build integration

package database

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jmoiron/sqlx"
)

func TestDefinerCannotBeShadowedByRuntimeConnection(t *testing.T) {
	ctx := context.Background()
	db, cleanup := isolatedMigrationSchema(t, ctx, os.Getenv("TEST_DATABASE_URL"))
	defer cleanup()
	if err := applyMigrationsThrough(ctx, db, "044_execute_privacy_deletion.up.sql"); err != nil {
		t.Fatal(err)
	}
	if err := ApplyMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}
	var schema, databaseName string
	if err := db.QueryRowx(`SELECT current_schema(),current_database()`).Scan(&schema, &databaseName); err != nil {
		t.Fatal(err)
	}
	role := "release_bot_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	password := uuid.NewString()
	if _, err := db.Exec(`CREATE ROLE ` + pgx.Identifier{role}.Sanitize() + ` LOGIN PASSWORD '` + password + `'`); err != nil {
		t.Fatal(err)
	}
	defer func() {
		db.Exec(`DROP OWNED BY ` + pgx.Identifier{role}.Sanitize())
		db.Exec(`DROP ROLE ` + pgx.Identifier{role}.Sanitize())
	}()
	for _, sql := range []string{
		`GRANT USAGE ON SCHEMA ` + pgx.Identifier{schema}.Sanitize() + ` TO ` + pgx.Identifier{role}.Sanitize(),
		`GRANT TEMPORARY ON DATABASE ` + pgx.Identifier{databaseName}.Sanitize() + ` TO ` + pgx.Identifier{role}.Sanitize(),
		`GRANT EXECUTE ON FUNCTION enqueue_privacy_deletion(TEXT) TO ` + pgx.Identifier{role}.Sanitize(),
		`GRANT EXECUTE ON FUNCTION scheduler_lock_active_group(TEXT), scheduler_select_replacement_group(TEXT) TO ` + pgx.Identifier{role}.Sanitize(),
		`INSERT INTO users(id,username) VALUES ('123456789','Synthetic user')`,
	} {
		if _, err := db.Exec(sql); err != nil {
			t.Fatal(err)
		}
	}
	parsed, err := url.Parse(os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	parsed.User = url.UserPassword(role, password)
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	limited, err := sqlx.ConnectContext(ctx, "pgx", parsed.String())
	if err != nil {
		t.Fatal("connect disposable restricted role:", err)
	}
	defer limited.Close()
	limited.SetMaxOpenConns(1)
	if _, err = limited.Exec(`
		CREATE TEMP TABLE universities(id TEXT, is_active BOOLEAN);
		CREATE TEMP TABLE groups(id TEXT, university_id TEXT, is_active BOOLEAN);
		CREATE TEMP TABLE subscriptions(id TEXT, user_id TEXT, object_id TEXT, object_type TEXT, updated_at TIMESTAMPTZ, created_at TIMESTAMPTZ);
		INSERT INTO universities VALUES('shadow-university',TRUE);
		INSERT INTO groups VALUES('shadow-group','shadow-university',TRUE);
		INSERT INTO subscriptions VALUES('shadow-sub','shadow-user','shadow-group','group',NOW(),NOW());
	`); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`SELECT scheduler_lock_active_group('shadow-group')`,
		`SELECT scheduler_select_replacement_group('shadow-user')`,
	} {
		var groupID string
		if err = limited.Get(&groupID, query); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("runtime shadow group accepted: group=%q err=%v", groupID, err)
		}
	}
	if _, err = limited.Exec(`CREATE TEMP TABLE privacy_deletion_requests (id TEXT, user_id TEXT, status TEXT, created_at TIMESTAMPTZ, next_retry_at TIMESTAMPTZ);
 CREATE FUNCTION pg_temp.attack() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN EXECUTE 'ALTER ROLE "` + role + `" SUPERUSER'; RETURN NEW; END $$;
 CREATE TRIGGER attack BEFORE INSERT ON privacy_deletion_requests FOR EACH ROW EXECUTE FUNCTION pg_temp.attack()`); err != nil {
		t.Fatal(err)
	}
	var requestID string
	if err = limited.Get(&requestID, `SELECT enqueue_privacy_deletion('123456789')`); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.Get(&count, `SELECT COUNT(*) FROM privacy_deletion_requests WHERE id=$1`, requestID); err != nil || count != 1 {
		t.Fatalf("trusted table count=%d err=%v", count, err)
	}
	var privileged bool
	if err = db.Get(&privileged, `SELECT rolsuper FROM pg_roles WHERE rolname=$1`, role); err != nil || privileged {
		t.Fatalf("runtime privilege escalation: %t %v", privileged, err)
	}
	var unsafe int
	if err = db.Get(&unsafe, `SELECT COUNT(*) FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname=$1 AND p.prosecdef AND (p.proowner=(SELECT oid FROM pg_roles WHERE rolname=$2) OR NOT ('search_path=pg_catalog, ' || quote_ident($1) || ', pg_temp'=ANY(p.proconfig)))`, schema, role); err != nil || unsafe != 0 {
		t.Fatalf("unsafe definer count=%d err=%v", unsafe, err)
	}
}
