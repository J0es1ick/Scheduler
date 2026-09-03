//go:build integration

package repository_test

import (
	"context"
	"testing"

	"github.com/J0es1ick/Scheduler/internal/admin"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

func TestOldestPendingMetricCountsOnlyDeliverableMessages(t *testing.T) {
	db, ctx := openOperationalIntegrationDB(t)
	suffix := uuid.NewString()
	universityID := "queue-metric-university-" + suffix
	groupID := "queue-metric-group-" + suffix
	userID := "queue-metric-user-" + suffix
	eventID := "queue-metric-event-" + suffix
	outboxID := "queue-metric-outbox-" + suffix
	adminOutboxID := "queue-metric-admin-outbox-" + suffix
	if _, err := repository.NewUniversityRepository(db).CreateUniversity(
		ctx, universityID, "Queue metric", "", "", true,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.NewGroupRepository(db).CreateGroup(
		ctx, groupID, universityID, "QUEUE", true,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.NewUserRepository(db).CreateUser(ctx, userID, "", false); err != nil {
		t.Fatal(err)
	}
	if err := repository.NewSubscriptionRepository(db).UpsertSubscription(
		ctx, "queue-metric-subscription-"+suffix, userID, groupID, "group",
	); err != nil {
		t.Fatal(err)
	}
	if err := repository.NewNotificationRepository(db).EnqueueScheduleChange(
		ctx, eventID, groupID, "parser", "changed",
	); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO bot_outbox(id, user_id, group_id, kind, body)
		VALUES ($1, $2, $3, 'lesson_reminder', 'reminder')`, outboxID, userID, groupID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO bot_outbox(id, user_id, kind, body)
		VALUES ($1, $2, 'admin_alert', 'alert')`, adminOutboxID, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		UPDATE notification_deliveries
		SET created_at=NOW()-INTERVAL '1 hour', next_attempt_at=NOW()+INTERVAL '1 hour'
		WHERE event_id=$1`, eventID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		UPDATE bot_outbox
		SET created_at=NOW()-INTERVAL '1 hour', next_attempt_at=NOW()+INTERVAL '1 hour'
		WHERE id IN ($1, $2)`, outboxID, adminOutboxID); err != nil {
		t.Fatal(err)
	}
	assertOldestPending(t, ctx, db, 0)

	if _, err := db.ExecContext(ctx, `
		UPDATE users SET quiet_hours_enabled=TRUE,
			quiet_hours_start=((NOW() AT TIME ZONE 'Europe/Moscow')-INTERVAL '1 hour')::time,
			quiet_hours_end=((NOW() AT TIME ZONE 'Europe/Moscow')+INTERVAL '1 hour')::time
		WHERE id=$1`, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		`UPDATE notification_deliveries SET next_attempt_at=NOW()-INTERVAL '1 minute' WHERE event_id=$1`, eventID,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		`UPDATE bot_outbox SET next_attempt_at=NOW()-INTERVAL '1 minute' WHERE id IN ($1, $2)`, outboxID, adminOutboxID,
	); err != nil {
		t.Fatal(err)
	}
	assertOldestPending(t, ctx, db, 0)

	if _, err := db.ExecContext(ctx, `
		UPDATE users
		SET quiet_hours_enabled=FALSE, reminder_enabled=TRUE, default_group_id=$2
		WHERE id=$1`, userID, groupID,
	); err != nil {
		t.Fatal(err)
	}
	assertOldestPending(t, ctx, db, 3500)
	if _, err := db.ExecContext(ctx, `UPDATE users SET default_group_id=NULL WHERE id=$1`, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM subscriptions WHERE user_id=$1`, userID); err != nil {
		t.Fatal(err)
	}
	assertOldestPending(t, ctx, db, 0)
	if err := repository.NewSubscriptionRepository(db).UpsertSubscription(
		ctx, "queue-metric-subscription-restored-"+suffix, userID, groupID, "group",
	); err != nil {
		t.Fatal(err)
	}
	assertOldestPending(t, ctx, db, 3500)
	if _, err := db.ExecContext(ctx,
		`UPDATE users SET notifications_enabled=FALSE WHERE id=$1`, userID,
	); err != nil {
		t.Fatal(err)
	}
	assertOldestPending(t, ctx, db, 0)
	if _, err := db.ExecContext(ctx, `
		UPDATE users SET is_admin=TRUE, admin_role='read_only' WHERE id=$1`, userID,
	); err != nil {
		t.Fatal(err)
	}
	assertOldestPending(t, ctx, db, 3500)
}

func assertOldestPending(t *testing.T, ctx context.Context, db *sqlx.DB, minimum int64) {
	t.Helper()
	serviceMetrics, err := repository.NewMetricsRepository(db).Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	operations, err := admin.NewStore(db).OperationalHealth(ctx)
	if err != nil {
		t.Fatal(err)
	}
	values := []int64{serviceMetrics.OldestPendingSeconds, operations.OldestPendingSeconds}
	for _, value := range values {
		if minimum == 0 && value != 0 {
			t.Fatalf("oldest pending=%d, want 0", value)
		}
		if minimum > 0 && value < minimum {
			t.Fatalf("oldest pending=%d, want at least %d", value, minimum)
		}
	}
}
