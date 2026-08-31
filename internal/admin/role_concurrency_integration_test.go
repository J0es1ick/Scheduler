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
	"github.com/J0es1ick/Scheduler/internal/repository"
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

func TestConcurrentPromotionAndSelfDeletion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := sqlx.ConnectContext(ctx, "pgx", os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err = database.ApplyMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}
	for range 20 {
		id := "promotion-" + uuid.NewString()
		if _, err = repository.NewUserRepository(db).CreateUser(ctx, id, id, false); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.ExecContext(context.Background(), `DELETE FROM users WHERE id=$1`, id) })
		start := make(chan struct{})
		promoted, deleted := make(chan error, 1), make(chan error, 1)
		go func() { <-start; promoted <- NewStore(db).UpdateUserAdminRole(ctx, id, "editor") }()
		go func() { <-start; deleted <- repository.NewUserRepository(db).DeleteUser(ctx, id) }()
		close(start)
		promotionErr, deletionErr := <-promoted, <-deleted
		if promotionErr == nil && deletionErr == nil {
			t.Fatal("promoted administrator was deleted")
		}
		var count int
		if err = db.GetContext(ctx, &count, `SELECT COUNT(*) FROM users WHERE id=$1 AND is_admin AND admin_role='editor'`, id); err != nil {
			t.Fatal(err)
		}
		if promotionErr == nil && count != 1 {
			t.Fatal("successful promotion did not preserve administrator")
		}
		if deletionErr != nil && promotionErr != nil {
			t.Fatalf("both actions failed: promotion=%v deletion=%v", promotionErr, deletionErr)
		}
	}
}
