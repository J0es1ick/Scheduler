//go:build integration

package repository_test

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/J0es1ick/Scheduler/internal/worker"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jmoiron/sqlx"
)

type lostPrivacyCommitAcknowledgement struct {
	*repository.PrivacyDeletionRepository
	reconciling chan struct{}
	release     chan struct{}
	once        sync.Once
}

func (q *lostPrivacyCommitAcknowledgement) Complete(ctx context.Context, id, token string) error {
	if err := q.PrivacyDeletionRepository.Complete(ctx, id, token); err != nil {
		return err
	}
	return errors.New("commit acknowledgement lost after database committed")
}

func (q *lostPrivacyCommitAcknowledgement) ExistingRequestIDs(ctx context.Context, ids []string) ([]string, error) {
	q.once.Do(func() { close(q.reconciling) })
	select {
	case <-q.release:
		return q.PrivacyDeletionRepository.ExistingRequestIDs(ctx, ids)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestPrivacyDeletionWorkerRecoversAfterCommittedDeletion(t *testing.T) {
	db, ctx := openOperationalIntegrationDB(t)
	var schema string
	if err := db.GetContext(ctx, &schema, `SELECT current_schema()`); err != nil {
		t.Fatal(err)
	}
	role := "privacy_connection_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	password := uuid.NewString()
	quotedRole := pgx.Identifier{role}.Sanitize()
	if _, err := db.ExecContext(ctx, `CREATE ROLE `+quotedRole+` LOGIN PASSWORD '`+password+`'`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DROP OWNED BY ` + quotedRole)
		_, _ = db.Exec(`DROP ROLE ` + quotedRole)
	})
	for _, statement := range []string{
		`GRANT USAGE ON SCHEMA ` + pgx.Identifier{schema}.Sanitize() + ` TO ` + quotedRole,
		`GRANT SELECT, UPDATE, DELETE ON privacy_deletion_requests TO ` + quotedRole,
		`GRANT EXECUTE ON FUNCTION scheduler_lock_privacy_deletion_request(TEXT,TEXT), execute_privacy_deletion(TEXT,TEXT) TO ` + quotedRole,
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	connectionURL, err := url.Parse(os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	connectionURL.User = url.UserPassword(role, password)
	params := connectionURL.Query()
	params.Set("search_path", schema)
	connectionURL.RawQuery = params.Encode()
	limited, err := sqlx.ConnectContext(ctx, "pgx", connectionURL.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = limited.Close() })
	if _, err = limited.ExecContext(ctx, `UPDATE users SET username='forbidden'`); err == nil {
		t.Fatal("privacy connection can modify profiles directly")
	}
	users := repository.NewUserRepository(db)
	if _, err = users.CreateUser(ctx, "lost-ack", "Synthetic user", false); err != nil {
		t.Fatal(err)
	}
	if _, err = repository.NewPrivacyDeletionRepository(db).Enqueue(ctx, "lost-ack"); err != nil {
		t.Fatal(err)
	}
	queue := &lostPrivacyCommitAcknowledgement{
		PrivacyDeletionRepository: repository.NewPrivacyDeletionRepository(limited),
		reconciling:               make(chan struct{}), release: make(chan struct{}),
	}
	monitor := worker.NewMonitor()
	monitor.Register(worker.PrivacyDeletionWorkerName, time.Minute)
	workerCtx, cancel := context.WithCancel(ctx)
	done := worker.NewPrivacyDeletionWorker(queue, 10*time.Millisecond, 1).Start(workerCtx, monitor)
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("privacy worker did not stop")
		}
	}()
	select {
	case <-queue.reconciling:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not reconcile the lost acknowledgement")
	}
	if monitor.Checks()[worker.PrivacyDeletionWorkerName] {
		t.Fatal("failed completion was not reported")
	}
	if user, loadErr := users.GetUserByID(ctx, "lost-ack"); loadErr != nil || user != nil {
		t.Fatalf("deletion did not commit: user=%v err=%v", user, loadErr)
	}
	if _, err = users.CreateUser(ctx, "lost-ack", "Recreated user", false); err != nil {
		t.Fatal(err)
	}
	close(queue.release)
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for !monitor.Checks()[worker.PrivacyDeletionWorkerName] {
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatal("health did not recover after committed deletion")
		}
	}
	if user, loadErr := users.GetUserByID(ctx, "lost-ack"); loadErr != nil || user == nil || user.Username != "Recreated user" {
		t.Fatalf("reconciliation changed recreated profile: user=%v err=%v", user, loadErr)
	}
}
