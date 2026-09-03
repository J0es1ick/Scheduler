//go:build integration

package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/google/uuid"
)

func TestClaimedScheduleCancellationKeepsBatchOwnership(t *testing.T) {
	db, ctx := openOperationalIntegrationDB(t)
	createNotificationLeaseFixture(t, db, ctx, "notification_deliveries", 1)
	createNotificationLeaseFixture(t, db, ctx, "notification_deliveries", 1)
	repo := repository.NewNotificationRepository(db)
	items, err := repo.ClaimPending(ctx, 2)
	if err != nil || len(items) != 2 || items[0].ClaimToken == "" || items[0].ClaimToken != items[1].ClaimToken {
		t.Fatalf("claim=%v err=%v", items, err)
	}
	cancelled := items[0]
	remaining := items[1]
	if err = repository.NewUserRepository(db).SetNotificationsEnabled(ctx, cancelled.UserID, false); err != nil {
		t.Fatal(err)
	}
	var state struct {
		Status          string     `db:"status"`
		Token           string     `db:"claim_token"`
		CancelRequested *time.Time `db:"cancel_requested_at"`
	}
	if err = db.GetContext(ctx, &state, `
		SELECT status, claim_token, cancel_requested_at
		FROM notification_deliveries WHERE id=$1`, cancelled.ID); err != nil {
		t.Fatal(err)
	}
	if state.Status != "pending" || state.Token != cancelled.ClaimToken || state.CancelRequested == nil {
		t.Fatalf("cancelled claimed state=%+v", state)
	}
	decision, err := repo.DeliveryDecision(ctx, cancelled.ID, cancelled.ClaimToken)
	if err != nil || decision != repository.NotificationQueueCancel {
		t.Fatalf("decision=%q err=%v", decision, err)
	}
	if err = repo.MarkCancelled(ctx, cancelled.ID, cancelled.ClaimToken); err != nil {
		t.Fatal(err)
	}
	if err = repo.RenewDeliveryClaims(ctx, remaining.ClaimToken, []string{remaining.ID}); err != nil {
		t.Fatalf("remaining claim was lost: %v", err)
	}
	if err = repo.MarkDelivered(ctx, remaining.ID, remaining.ClaimToken); err != nil {
		t.Fatal(err)
	}
}

func TestClaimedScheduleUnsubscribeIsWorkerFinalized(t *testing.T) {
	db, ctx := openOperationalIntegrationDB(t)
	createNotificationLeaseFixture(t, db, ctx, "notification_deliveries", 1)
	repo := repository.NewNotificationRepository(db)
	items, err := repo.ClaimPending(ctx, 1)
	if err != nil || len(items) != 1 {
		t.Fatalf("claim=%v err=%v", items, err)
	}
	item := items[0]
	if _, err = repository.NewSubscriptionRepository(db).UnsubscribeAndSelectDefault(
		ctx, item.UserID, item.GroupID,
	); err != nil {
		t.Fatal(err)
	}
	decision, err := repo.DeliveryDecision(ctx, item.ID, item.ClaimToken)
	if err != nil || decision != repository.NotificationQueueCancel {
		t.Fatalf("decision=%q err=%v", decision, err)
	}
	if err = repo.MarkCancelled(ctx, item.ID, item.ClaimToken); err != nil {
		t.Fatal(err)
	}
}

func TestClaimedReminderCancellationKeepsBatchOwnership(t *testing.T) {
	db, ctx := openOperationalIntegrationDB(t)
	createNotificationLeaseFixture(t, db, ctx, "bot_outbox", 1)
	createNotificationLeaseFixture(t, db, ctx, "bot_outbox", 1)
	if _, err := db.ExecContext(ctx, `
		UPDATE users u
		SET reminder_enabled=TRUE, default_group_id=g.id
		FROM groups g
		WHERE g.id LIKE 'notification-lease-group-%'
		  AND g.university_id LIKE 'notification-lease-university-%'
		  AND u.id=REPLACE(g.id, 'notification-lease-group-', 'notification-lease-user-')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		UPDATE bot_outbox o
		SET kind='lesson_reminder', group_id=u.default_group_id
		FROM users u
		WHERE u.id=o.user_id`); err != nil {
		t.Fatal(err)
	}
	repo := repository.NewNotificationRepository(db)
	items, err := repo.ClaimBotOutbox(ctx, 2)
	if err != nil || len(items) != 2 || items[0].ClaimToken == "" || items[0].ClaimToken != items[1].ClaimToken {
		t.Fatalf("claim=%v err=%v", items, err)
	}
	cancelled := items[0]
	remaining := items[1]
	if err = repository.NewUserRepository(db).SetLessonReminder(ctx, cancelled.UserID, false, 15); err != nil {
		t.Fatal(err)
	}
	decision, err := repo.BotOutboxDecision(ctx, cancelled.ID, cancelled.ClaimToken)
	if err != nil || decision != repository.NotificationQueueCancel {
		t.Fatalf("decision=%q err=%v", decision, err)
	}
	if err = repo.MarkBotOutboxCancelled(ctx, cancelled.ID, cancelled.ClaimToken); err != nil {
		t.Fatal(err)
	}
	if err = repo.RenewBotOutboxClaims(ctx, remaining.ClaimToken, []string{remaining.ID}); err != nil {
		t.Fatalf("remaining claim was lost: %v", err)
	}
	if err = repo.MarkBotOutboxDelivered(ctx, remaining.ID, remaining.ClaimToken); err != nil {
		t.Fatal(err)
	}
}

func TestQueuedReminderIsCancelledWhenScheduleScopeBecomesInactive(t *testing.T) {
	for _, scope := range []string{"group", "university"} {
		t.Run(scope, func(t *testing.T) {
			db, ctx := openOperationalIntegrationDB(t)
			suffix := uuid.NewString()
			universityID := "notification-scope-university-" + suffix
			groupID := "notification-scope-group-" + suffix
			userID := "notification-scope-user-" + suffix
			outboxID := "notification-scope-outbox-" + suffix
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
			if _, err := repository.NewUniversityRepository(db).CreateUniversity(ctx, universityID, "Notification scope test", "", "", true); err != nil {
				t.Fatal(err)
			}
			if _, err := repository.NewGroupRepository(db).CreateGroup(ctx, groupID, universityID, "SCOPE", true); err != nil {
				t.Fatal(err)
			}
			if _, err := repository.NewUserRepository(db).CreateUser(ctx, userID, "", false); err != nil {
				t.Fatal(err)
			}
			if err := repository.NewSubscriptionRepository(db).UpsertSubscription(ctx, "notification-scope-subscription-"+suffix, userID, groupID, "group"); err != nil {
				t.Fatal(err)
			}
			if err := repository.NewUserRepository(db).SetDefaultGroup(ctx, userID, groupID); err != nil {
				t.Fatal(err)
			}
			if err := repository.NewUserRepository(db).SetLessonReminder(ctx, userID, true, 15); err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(ctx, `
				INSERT INTO bot_outbox(id, user_id, group_id, kind, body)
				VALUES ($1, $2, $3, 'lesson_reminder', 'test reminder')`, outboxID, userID, groupID); err != nil {
				t.Fatal(err)
			}
			if scope == "group" {
				if _, err := db.ExecContext(ctx, `UPDATE groups SET is_active=FALSE WHERE id=$1`, groupID); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := db.ExecContext(ctx, `UPDATE universities SET is_active=FALSE WHERE id=$1`, universityID); err != nil {
					t.Fatal(err)
				}
			}
			var eligibility struct {
				Decision string `db:"policy_decision"`
				Reason   string `db:"policy_reason"`
			}
			if err := db.GetContext(ctx, &eligibility, `
				SELECT policy_decision, policy_reason
				FROM notification_queue_eligibility
				WHERE queue_type='outbox' AND id=$1`, outboxID); err != nil {
				t.Fatal(err)
			}
			expectedReason := "reminder_" + scope + "_inactive"
			if eligibility.Decision != "cancel" || eligibility.Reason != expectedReason {
				t.Fatalf("eligibility=%+v want cancel/%s", eligibility, expectedReason)
			}
			if _, err := db.ExecContext(ctx, `SELECT scheduler_reconcile_notification_queue()`); err != nil {
				t.Fatal(err)
			}
			var status string
			if err := db.GetContext(ctx, &status, `SELECT status FROM bot_outbox WHERE id=$1`, outboxID); err != nil {
				t.Fatal(err)
			}
			if status != "cancelled" {
				t.Fatalf("status=%q", status)
			}
		})
	}
}

func TestCancelledQueuedDeliveryCannotBeRevived(t *testing.T) {
	db, ctx := openOperationalIntegrationDB(t)
	createNotificationLeaseFixture(t, db, ctx, "notification_deliveries", 1)
	var userID string
	if err := db.GetContext(ctx, &userID, `SELECT user_id FROM notification_deliveries LIMIT 1`); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepository(db)
	if err := users.SetNotificationsEnabled(ctx, userID, false); err != nil {
		t.Fatal(err)
	}
	if err := users.SetNotificationsEnabled(ctx, userID, true); err != nil {
		t.Fatal(err)
	}
	items, err := repository.NewNotificationRepository(db).ClaimPending(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("cancelled delivery was revived: %v", items)
	}
	var status string
	if err = db.GetContext(ctx, &status, `SELECT status FROM notification_deliveries LIMIT 1`); err != nil {
		t.Fatal(err)
	}
	if status != "cancelled" {
		t.Fatalf("status=%q", status)
	}
}

func TestExpiredCancellationRequestIsTerminalized(t *testing.T) {
	db, ctx := openOperationalIntegrationDB(t)
	createNotificationLeaseFixture(t, db, ctx, "notification_deliveries", 1)
	repo := repository.NewNotificationRepository(db)
	items, err := repo.ClaimPending(ctx, 1)
	if err != nil || len(items) != 1 {
		t.Fatalf("claim=%v err=%v", items, err)
	}
	if err = repository.NewUserRepository(db).SetNotificationsEnabled(ctx, items[0].UserID, false); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `
		UPDATE notification_deliveries
		SET lease_expires_at=clock_timestamp()-INTERVAL '1 second'
		WHERE id=$1`, items[0].ID); err != nil {
		t.Fatal(err)
	}
	claimed, err := repo.ClaimPending(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 0 {
		t.Fatalf("cancel-requested delivery was reclaimed: %v", claimed)
	}
	var terminal bool
	if err = db.GetContext(ctx, &terminal, `
		SELECT status='cancelled' AND claim_token='' AND lease_expires_at IS NULL
			AND cancel_requested_at IS NULL
		FROM notification_deliveries WHERE id=$1`, items[0].ID); err != nil || !terminal {
		t.Fatalf("terminal=%t err=%v", terminal, err)
	}
}

func TestClaimFinalizationResolvesCancellationRace(t *testing.T) {
	for _, outcome := range []string{"delivered", "retry"} {
		t.Run(outcome, func(t *testing.T) {
			db, ctx := openOperationalIntegrationDB(t)
			createNotificationLeaseFixture(t, db, ctx, "notification_deliveries", 1)
			repo := repository.NewNotificationRepository(db)
			items, err := repo.ClaimPending(ctx, 1)
			if err != nil || len(items) != 1 {
				t.Fatalf("claim=%v err=%v", items, err)
			}
			item := items[0]
			if err = repository.NewUserRepository(db).SetNotificationsEnabled(ctx, item.UserID, false); err != nil {
				t.Fatal(err)
			}
			switch outcome {
			case "delivered":
				err = repo.MarkDelivered(ctx, item.ID, item.ClaimToken)
			case "retry":
				err = repo.MarkFailed(ctx, item.ID, item.ClaimToken, item.Attempts, time.Minute, context.DeadlineExceeded)
			}
			if err != nil {
				t.Fatal(err)
			}
			var status string
			if err = db.GetContext(ctx, &status, `
				SELECT status FROM notification_deliveries WHERE id=$1`, item.ID); err != nil {
				t.Fatal(err)
			}
			expected := "cancelled"
			if outcome == "delivered" {
				expected = "delivered"
			}
			if status != expected {
				t.Fatalf("status=%q want=%q", status, expected)
			}
		})
	}
}

func TestNotificationQueueOwnershipConstraints(t *testing.T) {
	db, ctx := openOperationalIntegrationDB(t)
	createNotificationLeaseFixture(t, db, ctx, "bot_outbox", 1)
	var id string
	if err := db.GetContext(ctx, &id, `SELECT id FROM bot_outbox LIMIT 1`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		UPDATE bot_outbox SET claim_token='invalid', lease_expires_at=NULL WHERE id=$1`, id); err == nil {
		t.Fatal("claim token without lease was accepted")
	}
	if _, err := db.ExecContext(ctx, `
		UPDATE bot_outbox SET status='delivered', claim_token='invalid',
			lease_expires_at=clock_timestamp()+INTERVAL '1 minute' WHERE id=$1`, id); err == nil {
		t.Fatal("terminal delivery retained ownership")
	}
}

func TestScheduleQuietHoursUseSharedEligibility(t *testing.T) {
	db, ctx := openOperationalIntegrationDB(t)
	createNotificationLeaseFixture(t, db, ctx, "notification_deliveries", 1)
	if _, err := db.ExecContext(ctx, `
		UPDATE users
		SET quiet_hours_enabled=TRUE,
			quiet_hours_start=((clock_timestamp() AT TIME ZONE 'Europe/Moscow')-INTERVAL '1 hour')::time,
			quiet_hours_end=((clock_timestamp() AT TIME ZONE 'Europe/Moscow')+INTERVAL '1 hour')::time`); err != nil {
		t.Fatal(err)
	}
	var state struct {
		Decision  string `db:"policy_decision"`
		Claimable bool   `db:"claimable"`
	}
	if err := db.GetContext(ctx, &state, `
		SELECT policy_decision, claimable
		FROM notification_queue_eligibility WHERE queue_type='schedule'`); err != nil {
		t.Fatal(err)
	}
	if state.Decision != "defer" || state.Claimable {
		t.Fatalf("state=%+v", state)
	}
	claimed, err := repository.NewNotificationRepository(db).ClaimPending(ctx, 1)
	if err != nil || len(claimed) != 0 {
		t.Fatalf("quiet delivery claim=%v err=%v", claimed, err)
	}
}
