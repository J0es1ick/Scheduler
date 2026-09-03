//go:build integration

package repository_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/database"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

func TestPrivacyDeletionQueueIsIdempotentAndLeaseFenced(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TEST_DATABASE_URL is required for integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	db, err := sqlx.ConnectContext(ctx, "pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err = database.ApplyMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}
	userID := "privacy-user-" + uuid.NewString()
	userRepo := repository.NewUserRepository(db)
	if _, err = userRepo.CreateUser(ctx, userID, "privacy", false); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM privacy_deletion_requests WHERE user_id=$1`, userID)
		_, _ = db.Exec(`DELETE FROM users WHERE id=$1`, userID)
	})
	firstID, err := userRepo.EnqueuePrivacyDeletion(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := userRepo.EnqueuePrivacyDeletion(ctx, userID)
	if err != nil || secondID != firstID {
		t.Fatalf("idempotent enqueue: first=%s second=%s err=%v", firstID, secondID, err)
	}
	queue := repository.NewPrivacyDeletionRepository(db)
	firstClaims, err := queue.ClaimPending(ctx, 1)
	if err != nil || len(firstClaims) != 1 {
		t.Fatalf("first claim: %+v err=%v", firstClaims, err)
	}
	if _, err = db.ExecContext(ctx, `UPDATE privacy_deletion_requests SET lease_expires_at=TIMESTAMPTZ '-infinity' WHERE id=$1`, firstID); err != nil {
		t.Fatal(err)
	}
	secondClaims, err := queue.ClaimPending(ctx, 1)
	if err != nil || len(secondClaims) != 1 || secondClaims[0].ClaimToken == firstClaims[0].ClaimToken {
		t.Fatalf("reclaim: %+v err=%v", secondClaims, err)
	}
	if err = queue.Complete(ctx, firstID, firstClaims[0].ClaimToken); !errors.Is(err, repository.ErrPrivacyDeletionClaimLost) {
		t.Fatalf("stale completion accepted: %v", err)
	}
	if err = queue.Complete(ctx, firstID, secondClaims[0].ClaimToken); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.GetContext(ctx, &count, `SELECT COUNT(*) FROM privacy_deletion_requests WHERE user_id=$1`, userID); err != nil || count != 0 {
		t.Fatalf("completed request retained: count=%d err=%v", count, err)
	}
}

func TestPrivacyDeletionRecoversAfterUserDeleteCommit(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TEST_DATABASE_URL is required for integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	db, err := sqlx.ConnectContext(ctx, "pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err = database.ApplyMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}
	userID := "privacy-crash-user-" + uuid.NewString()
	userRepo := repository.NewUserRepository(db)
	if _, err = userRepo.CreateUser(ctx, userID, "privacy-crash", false); err != nil {
		t.Fatal(err)
	}
	requestID, err := userRepo.EnqueuePrivacyDeletion(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM privacy_deletion_requests WHERE id=$1`, requestID)
		_, _ = db.Exec(`DELETE FROM users WHERE id=$1`, userID)
	})
	queue := repository.NewPrivacyDeletionRepository(db)
	claims, err := queue.ClaimPending(ctx, 1)
	if err != nil || len(claims) != 1 {
		t.Fatalf("claim before simulated crash: %+v err=%v", claims, err)
	}
	if err = userRepo.DeleteUser(ctx, userID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `UPDATE privacy_deletion_requests SET lease_expires_at=TIMESTAMPTZ '-infinity' WHERE id=$1`, requestID); err != nil {
		t.Fatal(err)
	}
	claims, err = queue.ClaimPending(ctx, 1)
	if err != nil || len(claims) != 1 {
		t.Fatalf("claim after simulated crash: %+v err=%v", claims, err)
	}
	if err = userRepo.DeleteUser(ctx, userID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("repeated deletion = %v, want sql.ErrNoRows", err)
	}
	if err = queue.Complete(ctx, requestID, claims[0].ClaimToken); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.GetContext(ctx, &count, `SELECT COUNT(*) FROM privacy_deletion_requests WHERE id=$1`, requestID); err != nil || count != 0 {
		t.Fatalf("request remained after recovery: count=%d err=%v", count, err)
	}
}
