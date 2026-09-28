//go:build integration

package repository_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/jmoiron/sqlx"
)

func TestPrivacyDeletionConcurrentEnqueueAndCompletion(t *testing.T) {
	db, ctx := openOperationalIntegrationDB(t)
	queue := repository.NewPrivacyDeletionRepository(db)
	if _, err := repository.NewUserRepository(db).CreateUser(ctx, "concurrent", "Synthetic user", false); err != nil {
		t.Fatal(err)
	}
	id, err := queue.Enqueue(ctx, "concurrent")
	if err != nil {
		t.Fatal(err)
	}
	claims, err := queue.ClaimPending(ctx, 1)
	if err != nil || len(claims) != 1 {
		t.Fatalf("claim=%v err=%v", claims, err)
	}
	enqueueTx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer enqueueTx.Rollback()
	if _, err = enqueueTx.ExecContext(ctx, `SELECT id FROM users WHERE id='concurrent' FOR UPDATE`); err != nil {
		t.Fatal(err)
	}
	var enqueuePID int
	if err = enqueueTx.GetContext(ctx, &enqueuePID, `SELECT pg_backend_pid()`); err != nil {
		t.Fatal(err)
	}
	completionCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	completed := make(chan error, 1)
	go func() { completed <- queue.Complete(completionCtx, id, claims[0].ClaimToken) }()
	waitPrivacyBlockedBy(t, ctx, db, enqueuePID)
	var repeatedID string
	enqueueErr := enqueueTx.GetContext(ctx, &repeatedID, `SELECT enqueue_privacy_deletion('concurrent')`)
	if enqueueErr == nil {
		enqueueErr = enqueueTx.Commit()
	} else {
		_ = enqueueTx.Rollback()
	}
	completionErr := <-completed
	if enqueueErr != nil || completionErr != nil || repeatedID != id {
		t.Fatalf("concurrent enqueue=%v completion=%v repeated=%q want=%q", enqueueErr, completionErr, repeatedID, id)
	}
	var remaining int
	if err = db.GetContext(ctx, &remaining, `SELECT (SELECT count(*) FROM users WHERE id='concurrent') + (SELECT count(*) FROM privacy_deletion_requests)`); err != nil || remaining != 0 {
		t.Fatalf("incomplete deletion: remaining=%d err=%v", remaining, err)
	}
}

func TestPrivacyDeletionRechecksClaimAfterWaitingForProfile(t *testing.T) {
	for _, reclaim := range []bool{false, true} {
		t.Run(map[bool]string{false: "expired lease", true: "replacement claim"}[reclaim], func(t *testing.T) {
			db, ctx := openOperationalIntegrationDB(t)
			queue := repository.NewPrivacyDeletionRepository(db)
			users := repository.NewUserRepository(db)
			if _, err := users.CreateUser(ctx, "waiting", "Synthetic user", false); err != nil {
				t.Fatal(err)
			}
			id, err := queue.Enqueue(ctx, "waiting")
			if err != nil {
				t.Fatal(err)
			}
			claims, err := queue.ClaimPending(ctx, 1)
			if err != nil || len(claims) != 1 {
				t.Fatalf("claim=%v err=%v", claims, err)
			}
			profileTx, err := db.BeginTxx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer profileTx.Rollback()
			var blockingPID int
			if err = profileTx.GetContext(ctx, &blockingPID, `SELECT pg_backend_pid() FROM users WHERE id='waiting' FOR UPDATE`); err != nil {
				t.Fatal(err)
			}
			completionCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			completed := make(chan error, 1)
			go func() { completed <- queue.Complete(completionCtx, id, claims[0].ClaimToken) }()
			waitPrivacyBlockedBy(t, ctx, db, blockingPID)
			if _, err = db.ExecContext(ctx, `UPDATE privacy_deletion_requests SET lease_expires_at=TIMESTAMPTZ '-infinity' WHERE id=$1`, id); err != nil {
				t.Fatal(err)
			}
			var replacementToken string
			if reclaim {
				replacement, claimErr := queue.ClaimPending(ctx, 1)
				if claimErr != nil || len(replacement) != 1 {
					t.Fatalf("replacement=%v err=%v", replacement, claimErr)
				}
				replacementToken = replacement[0].ClaimToken
			}
			if err = profileTx.Commit(); err != nil {
				t.Fatal(err)
			}
			if err = <-completed; !errors.Is(err, repository.ErrPrivacyDeletionClaimLost) {
				t.Fatalf("stale completion accepted after lock wait: %v", err)
			}
			if user, loadErr := users.GetUserByID(ctx, "waiting"); loadErr != nil || user == nil {
				t.Fatalf("profile deleted by expired claim: user=%v err=%v", user, loadErr)
			}
			if reclaim {
				if err = queue.Complete(ctx, id, replacementToken); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func waitPrivacyBlockedBy(t *testing.T, ctx context.Context, db *sqlx.DB, blockingPID int) {
	t.Helper()
	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var blocked bool
		if err := db.GetContext(waitCtx, &blocked, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1=ANY(pg_blocking_pids(pid)))`, blockingPID); err != nil {
			t.Fatalf("wait for concurrent deletion lock: %v", err)
		}
		if blocked {
			return
		}
		select {
		case <-waitCtx.Done():
			t.Fatal("completion did not wait for the profile lock")
		case <-ticker.C:
		}
	}
}

func TestPrivacyDeletionRejectsPromotedAdministrator(t *testing.T) {
	db, ctx := openOperationalIntegrationDB(t)
	queue := repository.NewPrivacyDeletionRepository(db)
	if _, err := repository.NewUserRepository(db).CreateUser(ctx, "promoted", "Synthetic user", false); err != nil {
		t.Fatal(err)
	}
	id, err := queue.Enqueue(ctx, "promoted")
	if err != nil {
		t.Fatal(err)
	}
	claims, err := queue.ClaimPending(ctx, 1)
	if err != nil || len(claims) != 1 {
		t.Fatalf("claim=%v err=%v", claims, err)
	}
	if _, err = db.ExecContext(ctx, `UPDATE users SET is_admin=TRUE, admin_role='owner' WHERE id='promoted'`); err != nil {
		t.Fatal(err)
	}
	if err = queue.Complete(ctx, id, claims[0].ClaimToken); err == nil || !strings.Contains(err.Error(), "remove administrator role") {
		t.Fatalf("administrator guard failed: %v", err)
	}
	remaining, err := queue.ExistingRequestIDs(ctx, []string{id, "missing"})
	if err != nil || len(remaining) != 1 || remaining[0] != id {
		t.Fatalf("rejected deletion removed its request: %v err=%v", remaining, err)
	}
	if _, err = db.ExecContext(ctx, `UPDATE users SET is_admin=FALSE, admin_role='none' WHERE id='promoted'`); err != nil {
		t.Fatal(err)
	}
	if err = queue.Complete(ctx, id, claims[0].ClaimToken); err != nil {
		t.Fatal(err)
	}
	if remaining, err = queue.ExistingRequestIDs(ctx, []string{id}); err != nil || len(remaining) != 0 {
		t.Fatalf("completed request still exists: %v err=%v", remaining, err)
	}
}
