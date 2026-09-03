//go:build integration

package admin

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/database"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

func TestAdminRetentionOwnsOnlyAdministrativeHistory(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := sqlx.ConnectContext(ctx, "pgx", os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = database.ApplyMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}
	suffix := uuid.NewString()
	auditID := "admin-retention-audit-" + suffix
	sessionID := "admin-retention-session-" + suffix
	for _, statement := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO admin_audit_logs(id,actor_id,actor_name,action,object_type,created_at)
			VALUES ($1,'retention','Retention','expired','test',NOW()-INTERVAL '366 days')`, []any{auditID}},
		{`INSERT INTO admin_sessions(token_hash,admin_id,name,auth_method,admin_role,csrf_token,expires_at)
			VALUES ($1,'retention','Retention','telegram','owner','csrf',NOW()-INTERVAL '1 day')`, []any{sessionID}},
		{`UPDATE operational_maintenance SET last_run_at='-infinity' WHERE task_name='admin_retention'`, nil},
	} {
		if _, err = db.ExecContext(ctx, statement.query, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM admin_audit_logs WHERE id=$1`, auditID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM admin_sessions WHERE token_hash=$1`, sessionID)
	})
	run, err := NewStore(db).RunRetention(ctx)
	if err != nil || !run {
		t.Fatalf("admin retention run=%t err=%v", run, err)
	}
	var remaining int
	if err = db.GetContext(ctx, &remaining, `
		SELECT (SELECT COUNT(*) FROM admin_audit_logs WHERE id=$1)
		     + (SELECT COUNT(*) FROM admin_sessions WHERE token_hash=$2)`, auditID, sessionID); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatalf("admin retention left %d expired records", remaining)
	}
}
