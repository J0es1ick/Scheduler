//go:build integration

package site_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/site"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

func TestPublicSiteRoleCanReadOnlyPublicViews(t *testing.T) {
	databaseURL := os.Getenv("TEST_SITE_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TEST_SITE_DATABASE_URL is required for integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	db, err := sqlx.ConnectContext(ctx, "pgx", databaseURL)
	if err != nil {
		t.Fatalf("connect with public site role: %v", err)
	}
	defer db.Close()

	if err = site.NewStore(db).Ready(ctx); err != nil {
		t.Fatalf("public site readiness: %v", err)
	}
	if _, err = site.NewStore(db).PublicInfo(ctx, "https://example.test/project", "https://t.me/example"); err != nil {
		t.Fatalf("load public information: %v", err)
	}

	for _, relation := range []string{
		"public.users",
		"public.subscriptions",
		"public.support_requests",
		"public.admin_sessions",
	} {
		var allowed bool
		if err = db.GetContext(ctx, &allowed, `SELECT has_table_privilege(current_user, $1, 'SELECT')`, relation); err != nil {
			t.Fatalf("inspect privilege for %s: %v", relation, err)
		}
		if allowed {
			t.Fatalf("public site role can read private relation %s", relation)
		}
	}

	for _, view := range []string{
		"public.public_site_statistics",
		"public.public_site_universities",
		"public.public_site_sources",
	} {
		var allowed bool
		if err = db.GetContext(ctx, &allowed, `SELECT has_table_privilege(current_user, $1, 'SELECT')`, view); err != nil {
			t.Fatalf("inspect privilege for %s: %v", view, err)
		}
		if !allowed {
			t.Fatalf("public site role cannot read public view %s", view)
		}
	}
}
