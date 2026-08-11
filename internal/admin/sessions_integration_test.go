//go:build integration

package admin

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/database"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

func TestPostgresAdminSessionLifecycle(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TEST_DATABASE_URL is required for integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := sqlx.ConnectContext(ctx, "pgx", databaseURL)
	if err != nil {
		t.Fatalf("connect integration database: %v", err)
	}
	defer db.Close()
	if err = database.ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	store := NewStore(db)
	tokenHash := "integration-session-" + uuid.NewString()
	identity := AdminIdentity{
		ID: "integration-admin", Name: "Integration Admin", AuthMethod: "telegram",
		Role: "owner", CSRFToken: "csrf-" + uuid.NewString(),
	}
	t.Cleanup(func() { _ = store.DeleteAdminSession(context.Background(), tokenHash) })

	expires := time.Now().Add(time.Hour).UTC().Truncate(time.Microsecond)
	if err = store.SaveAdminSession(ctx, tokenHash, identity, expires, 100); err != nil {
		t.Fatalf("save session: %v", err)
	}
	loaded, loadedExpires, err := store.AdminSession(ctx, tokenHash)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	if loaded.ID != identity.ID || loaded.Role != identity.Role || loaded.CSRFToken != identity.CSRFToken {
		t.Fatalf("unexpected identity: %+v", loaded)
	}
	if !loadedExpires.Equal(expires) {
		t.Fatalf("expires = %s, want %s", loadedExpires, expires)
	}
	if err = store.DeleteAdminSession(ctx, tokenHash); err != nil {
		t.Fatalf("delete session: %v", err)
	}
	if _, _, err = store.AdminSession(ctx, tokenHash); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("deleted session remains available: %v", err)
	}
}
