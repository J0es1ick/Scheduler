//go:build integration

package admin

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/database"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

func TestConcurrentOwnerDemotionKeepsAnOwner(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TEST_DATABASE_URL is required for integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := sqlx.ConnectContext(ctx, "pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Errorf("close integration database: %v", closeErr)
		}
	})
	if err = database.ApplyMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}
	suffix := uuid.NewString()
	var initialOwners int
	if err = db.GetContext(ctx, &initialOwners, `SELECT COUNT(*)::int FROM users WHERE admin_role='owner'`); err != nil {
		t.Fatal(err)
	}
	ownerIDs := []string{"owner-a-" + suffix, "owner-b-" + suffix}
	for _, ownerID := range ownerIDs {
		if _, err = db.ExecContext(ctx, `
			INSERT INTO users (id, username, is_admin, admin_role)
			VALUES ($1,$1,TRUE,'owner')`, ownerID); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		if _, cleanupErr := db.ExecContext(context.Background(), `DELETE FROM users WHERE id=ANY($1)`, ownerIDs); cleanupErr != nil {
			t.Errorf("cleanup concurrent owners: %v", cleanupErr)
		}
	})

	store := NewStore(db)
	start := make(chan struct{})
	errorsChannel := make(chan error, len(ownerIDs))
	var wait sync.WaitGroup
	for _, ownerID := range ownerIDs {
		wait.Add(1)
		go func(id string) {
			defer wait.Done()
			<-start
			errorsChannel <- store.UpdateUserAdminRole(ctx, id, "none")
		}(ownerID)
	}
	close(start)
	wait.Wait()
	close(errorsChannel)
	successes := 0
	conflicts := 0
	for updateErr := range errorsChannel {
		switch {
		case updateErr == nil:
			successes++
		case errors.Is(updateErr, ErrConflict):
			conflicts++
		default:
			t.Fatalf("unexpected role update error: %v", updateErr)
		}
	}
	expectedSuccesses := 2
	expectedConflicts := 0
	if initialOwners == 0 {
		expectedSuccesses = 1
		expectedConflicts = 1
	}
	if successes != expectedSuccesses || conflicts != expectedConflicts {
		t.Fatalf("successes=%d conflicts=%d", successes, conflicts)
	}
	var owners int
	if err = db.GetContext(ctx, &owners, `SELECT COUNT(*)::int FROM users WHERE admin_role='owner'`); err != nil {
		t.Fatal(err)
	}
	if owners < 1 {
		t.Fatal("all owners were removed")
	}
}
