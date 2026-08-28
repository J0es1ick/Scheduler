//go:build integration

package repository_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type notificationLeaseItem struct {
	id       string
	token    string
	attempts int
}

type notificationLeaseQueue struct {
	table     string
	claim     func(context.Context, int) ([]notificationLeaseItem, error)
	renew     func(context.Context, string, []string) error
	active    func(context.Context, string, string) (bool, error)
	delivered func(context.Context, string, string) error
	cancelled func(context.Context, string, string) error
	failed    func(context.Context, string, string, int, time.Duration, error) error
	permanent func(context.Context, string, string, error) error
}

func notificationLeaseQueues(repo *repository.NotificationRepository) []notificationLeaseQueue {
	return []notificationLeaseQueue{
		{
			table: "notification_deliveries",
			claim: func(ctx context.Context, limit int) ([]notificationLeaseItem, error) {
				items, err := repo.ClaimPending(ctx, limit)
				result := make([]notificationLeaseItem, len(items))
				for index, item := range items {
					result[index] = notificationLeaseItem{item.ID, item.ClaimToken, item.Attempts}
				}
				return result, err
			},
			renew: repo.RenewDeliveryClaims, active: repo.IsDeliveryActive,
			delivered: repo.MarkDelivered, cancelled: repo.MarkCancelled,
			failed: repo.MarkFailed, permanent: repo.MarkPermanentFailure,
		},
		{
			table: "bot_outbox",
			claim: func(ctx context.Context, limit int) ([]notificationLeaseItem, error) {
				items, err := repo.ClaimBotOutbox(ctx, limit)
				result := make([]notificationLeaseItem, len(items))
				for index, item := range items {
					result[index] = notificationLeaseItem{item.ID, item.ClaimToken, item.Attempts}
				}
				return result, err
			},
			renew: repo.RenewBotOutboxClaims, active: repo.IsBotOutboxActive,
			delivered: repo.MarkBotOutboxDelivered, cancelled: repo.MarkBotOutboxCancelled,
			failed: repo.MarkBotOutboxFailed, permanent: repo.MarkBotOutboxPermanentFailure,
		},
	}
}

func createNotificationLeaseFixture(t *testing.T, db *sqlx.DB, ctx context.Context, table string, count int) {
	t.Helper()
	suffix := uuid.NewString()
	universityID := "notification-lease-university-" + suffix
	groupID := "notification-lease-group-" + suffix
	userID := "notification-lease-user-" + suffix
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := db.ExecContext(cleanupCtx, `DELETE FROM users WHERE id=$1`, userID); err != nil {
			t.Errorf("clean up notification user: %v", err)
		}
		if _, err := db.ExecContext(cleanupCtx, `DELETE FROM universities WHERE id=$1`, universityID); err != nil {
			t.Errorf("clean up notification university: %v", err)
		}
	})
	if _, err := repository.NewUniversityRepository(db).CreateUniversity(ctx, universityID, "Notification lease test", "", "", true); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.NewGroupRepository(db).CreateGroup(ctx, groupID, universityID, "LEASE", true); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.NewUserRepository(db).CreateUser(ctx, userID, "", false); err != nil {
		t.Fatal(err)
	}
	if err := repository.NewSubscriptionRepository(db).UpsertSubscription(ctx, "notification-lease-sub-"+suffix, userID, groupID, "group"); err != nil {
		t.Fatal(err)
	}
	for index := range count {
		id := fmt.Sprintf("notification-lease-event-%s-%d", suffix, index)
		var err error
		if table == "notification_deliveries" {
			err = repository.NewNotificationRepository(db).EnqueueScheduleChange(ctx, id, groupID, "parser", "test update")
		} else {
			_, err = db.ExecContext(ctx, `INSERT INTO bot_outbox(id,user_id,kind,body) VALUES ($1,$2,'support_resolution','test response')`, id, userID)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestNotificationLeaseRejectsStaleWorkerWrites(t *testing.T) {
	for _, table := range []string{"notification_deliveries", "bot_outbox"} {
		t.Run(table, func(t *testing.T) {
			db, ctx := openOperationalIntegrationDB(t)
			repo := repository.NewNotificationRepository(db)
			queues := notificationLeaseQueues(repo)
			queue := queues[0]
			if table == "bot_outbox" {
				queue = queues[1]
			}
			createNotificationLeaseFixture(t, db, ctx, table, 1)
			first, err := queue.claim(ctx, 1)
			if err != nil || len(first) != 1 || first[0].token == "" {
				t.Fatalf("first claim=%v err=%v", first, err)
			}
			old := first[0]
			if claimed, err := queue.claim(ctx, 1); err != nil || len(claimed) != 0 {
				t.Fatalf("live claim was reclaimed: %v err=%v", claimed, err)
			}
			if _, err = db.ExecContext(ctx, `UPDATE `+table+` SET lease_expires_at=clock_timestamp()+INTERVAL '5 seconds' WHERE id=$1`, old.id); err != nil {
				t.Fatal(err)
			}
			if err = queue.renew(ctx, old.token, []string{old.id}); err != nil {
				t.Fatal(err)
			}
			var renewed bool
			if err = db.GetContext(ctx, &renewed, `SELECT lease_expires_at>clock_timestamp()+INTERVAL '100 seconds' FROM `+table+` WHERE id=$1`, old.id); err != nil || !renewed {
				t.Fatalf("lease was not extended: renewed=%t err=%v", renewed, err)
			}
			if _, err = db.ExecContext(ctx, `UPDATE `+table+` SET lease_expires_at=TIMESTAMPTZ '-infinity' WHERE id=$1`, old.id); err != nil {
				t.Fatal(err)
			}
			assertLost := func() {
				t.Helper()
				if active, err := queue.active(ctx, old.id, old.token); err != nil || active {
					t.Fatalf("stale claim active=%t err=%v", active, err)
				}
				for name, action := range map[string]func() error{
					"renew":   func() error { return queue.renew(ctx, old.token, []string{old.id}) },
					"deliver": func() error { return queue.delivered(ctx, old.id, old.token) },
					"cancel":  func() error { return queue.cancelled(ctx, old.id, old.token) },
					"retry": func() error {
						return queue.failed(ctx, old.id, old.token, old.attempts, time.Minute, errors.New("stale"))
					},
					"fail": func() error { return queue.permanent(ctx, old.id, old.token, errors.New("stale")) },
				} {
					if err := action(); !errors.Is(err, repository.ErrNotificationClaimLost) {
						t.Fatalf("stale %s was accepted: %v", name, err)
					}
				}
			}
			assertLost()
			second, err := queue.claim(ctx, 1)
			if err != nil || len(second) != 1 || second[0].id != old.id || second[0].token == old.token || second[0].attempts != 2 {
				t.Fatalf("reclaim=%v err=%v", second, err)
			}
			assertLost()
			current := second[0]
			if active, err := queue.active(ctx, current.id, current.token); err != nil || !active {
				t.Fatalf("current claim inactive: %t %v", active, err)
			}
			if err = queue.delivered(ctx, current.id, current.token); err != nil {
				t.Fatal(err)
			}
			var released bool
			if err = db.GetContext(ctx, &released, `SELECT status='delivered' AND claim_token='' AND lease_expires_at IS NULL FROM `+table+` WHERE id=$1`, current.id); err != nil || !released {
				t.Fatalf("completed claim retained: %t %v", released, err)
			}
		})
	}
}

func TestNotificationLeaseConcurrentClaimsAreDisjoint(t *testing.T) {
	for _, table := range []string{"notification_deliveries", "bot_outbox"} {
		t.Run(table, func(t *testing.T) {
			db, ctx := openOperationalIntegrationDB(t)
			queues := notificationLeaseQueues(repository.NewNotificationRepository(db))
			queue := queues[0]
			if table == "bot_outbox" {
				queue = queues[1]
			}
			createNotificationLeaseFixture(t, db, ctx, table, 8)
			start := make(chan struct{})
			results := make(chan []notificationLeaseItem, 4)
			failures := make(chan error, 4)
			var group sync.WaitGroup
			for range 4 {
				group.Go(func() {
					<-start
					items, err := queue.claim(ctx, 2)
					results <- items
					failures <- err
				})
			}
			close(start)
			group.Wait()
			close(results)
			close(failures)
			for err := range failures {
				if err != nil {
					t.Fatal(err)
				}
			}
			seen := make(map[string]string)
			for items := range results {
				for _, item := range items {
					if previous, exists := seen[item.id]; exists {
						t.Fatalf("delivery %s claimed by %s and %s", item.id, previous, item.token)
					}
					seen[item.id] = item.token
				}
			}
			remaining, err := queue.claim(ctx, 8)
			if err != nil {
				t.Fatal(err)
			}
			for _, item := range remaining {
				if previous, exists := seen[item.id]; exists {
					t.Fatalf("live delivery %s reclaimed from %s by %s", item.id, previous, item.token)
				}
				seen[item.id] = item.token
			}
			if len(seen) != 8 {
				t.Fatalf("claimed %d deliveries, want 8", len(seen))
			}
		})
	}
}

func TestNotificationLeaseFinalizationReleasesOwnership(t *testing.T) {
	for _, table := range []string{"notification_deliveries", "bot_outbox"} {
		for _, outcome := range []string{"retry", "cancel", "permanent"} {
			t.Run(table+"/"+outcome, func(t *testing.T) {
				db, ctx := openOperationalIntegrationDB(t)
				queues := notificationLeaseQueues(repository.NewNotificationRepository(db))
				queue := queues[0]
				if table == "bot_outbox" {
					queue = queues[1]
				}
				createNotificationLeaseFixture(t, db, ctx, table, 1)
				claimed, err := queue.claim(ctx, 1)
				if err != nil || len(claimed) != 1 {
					t.Fatalf("claim=%v err=%v", claimed, err)
				}
				item := claimed[0]
				status := "pending"
				switch outcome {
				case "retry":
					err = queue.failed(ctx, item.id, item.token, item.attempts, time.Minute, errors.New("retry"))
				case "cancel":
					status = "cancelled"
					err = queue.cancelled(ctx, item.id, item.token)
				case "permanent":
					status = "failed"
					err = queue.permanent(ctx, item.id, item.token, errors.New("permanent"))
				}
				if err != nil {
					t.Fatal(err)
				}
				var released bool
				if err = db.GetContext(ctx, &released, `SELECT status=$2 AND claim_token='' AND lease_expires_at IS NULL FROM `+table+` WHERE id=$1`, item.id, status); err != nil || !released {
					t.Fatalf("finalized claim retained: %t %v", released, err)
				}
				if err = queue.delivered(ctx, item.id, item.token); !errors.Is(err, repository.ErrNotificationClaimLost) {
					t.Fatalf("released claim was finalized again: %v", err)
				}
				if pending, err := queue.claim(ctx, 1); err != nil || len(pending) != 0 {
					t.Fatalf("finalized delivery immediately reclaimed: %v %v", pending, err)
				}
			})
		}
	}
}
