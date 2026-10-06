//go:build integration

package repository_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/google/uuid"
)

func TestBroadcastConsentDeliveryAndPrivacy(t *testing.T) {
	db, ctx := openOperationalIntegrationDB(t)
	_, err := db.ExecContext(ctx, `INSERT INTO users(id,username,service_updates_consent) VALUES('42','alice',TRUE),('43','bob',FALSE),('44','pending',NULL),('45','alice',TRUE)`)
	if err != nil {
		t.Fatal(err)
	}
	broadcasts := repository.NewBroadcastRepository(db)
	audience, err := broadcasts.Audience(ctx, "selected", "@ALICE, bob pending missing @alice")
	if err != nil || len(audience.Recipients) != 5 || audience.Eligible != 2 {
		t.Fatalf("audience: %+v %v", audience, err)
	}
	if audience.Recipients[0].Reason != "ambiguous" {
		t.Fatal("ambiguous username not surfaced")
	}
	id, err := broadcasts.Create(ctx, "42")
	if err != nil {
		t.Fatal(err)
	}
	document := json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Новое расписание 🎉"}]}]}`)
	if err = broadcasts.Save(ctx, id, repository.BroadcastInput{Document: document, AudienceMode: "all", Version: 1}); err != nil {
		t.Fatal(err)
	}
	a := domain.BroadcastAttachment{Filename: "notes.txt", MediaType: "document", ContentType: "text/plain", Data: []byte("notes")}
	if err = broadcasts.AddAttachment(ctx, id, 2, a); err != nil {
		t.Fatal(err)
	}
	key := uuid.NewString()
	var wg sync.WaitGroup
	for range 3 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := broadcasts.Send(ctx, id, key, 3); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	queue := repository.NewNotificationRepository(db)
	items, err := queue.ClaimBotOutbox(ctx, 20)
	if err != nil || len(items) != 2 {
		t.Fatalf("queue %+v %v", items, err)
	}
	seen := map[string]bool{}
	for _, item := range items {
		if seen[item.UserID] {
			t.Fatal("duplicate delivery")
		}
		seen[item.UserID] = true
	}
	item := items[0]
	if err = queue.SaveBroadcastPart(ctx, item.ID, item.ClaimToken, 0, 123, "", ""); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepository(db)
	if _, err = users.SetServiceUpdates(ctx, item.UserID, false, ""); err != nil {
		t.Fatal(err)
	}
	decision, err := queue.BotOutboxDecision(ctx, item.ID, item.ClaimToken)
	if err != nil || decision != repository.NotificationQueueCancel {
		t.Fatalf("revoked: %s %v", decision, err)
	}
	if err = queue.MarkBotOutboxCancelled(ctx, item.ID, item.ClaimToken); err != nil {
		t.Fatal(err)
	}
	b, err := broadcasts.Get(ctx, id)
	if err != nil || b.Counts["partial"] != 1 {
		t.Fatalf("partial delivery: %+v %v", b, err)
	}
	if err = broadcasts.Stop(ctx, id); err != nil {
		t.Fatal(err)
	}
	decision, err = queue.BotOutboxDecision(ctx, items[1].ID, items[1].ClaimToken)
	if err != nil || decision != repository.NotificationQueueCancel {
		t.Fatalf("stopped: %s %v", decision, err)
	}
	export, err := users.ExportUserData(ctx, "42")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ref := range export.References {
		found = found || ref.Category == "broadcast"
	}
	if !found {
		t.Fatal("missing broadcast export")
	}
	if _, err = db.ExecContext(ctx, `DELETE FROM users WHERE id='42'`); err != nil {
		t.Fatal(err)
	}
	b, err = broadcasts.Get(ctx, id)
	if err != nil || b.AuthorID != "" {
		t.Fatalf("author privacy: %+v %v", b, err)
	}
	for _, recipient := range b.Recipients {
		if recipient.UserID == "42" {
			t.Fatal("recipient not deleted")
		}
	}
}

func TestServiceUpdatesInvitationsAreDurable(t *testing.T) {
	db, ctx := openOperationalIntegrationDB(t)
	_, err := db.ExecContext(ctx, `INSERT INTO users(id,service_updates_backfill) VALUES('42',TRUE),('43',FALSE),('44',TRUE);UPDATE users SET service_updates_consent=FALSE WHERE id='44'`)
	if err != nil {
		t.Fatal(err)
	}
	q := repository.NewNotificationRepository(db)
	for range 3 {
		if err = q.EnqueueServiceUpdatesPrompts(ctx); err != nil {
			t.Fatal(err)
		}
	}
	items, err := q.ClaimBotOutbox(ctx, 20)
	if err != nil || len(items) != 1 || items[0].UserID != "42" {
		t.Fatalf("prompts: %+v %v", items, err)
	}
	if err = q.MarkBotOutboxDelivered(ctx, items[0].ID, items[0].ClaimToken); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `DELETE FROM bot_outbox WHERE kind='service_updates_prompt'`); err != nil {
		t.Fatal(err)
	}
	if err = q.EnqueueServiceUpdatesPrompts(ctx); err != nil {
		t.Fatal(err)
	}
	items, err = q.ClaimBotOutbox(ctx, 20)
	if err != nil || len(items) != 0 {
		t.Fatalf("invitation repeated: %+v %v", items, err)
	}
	users := repository.NewUserRepository(db)
	u, err := users.GetUserByID(ctx, "42")
	if err != nil {
		t.Fatal(err)
	}
	changed, err := users.SetServiceUpdates(ctx, "42", true, u.ServiceUpdatesPromptKey)
	if err != nil || !changed {
		t.Fatalf("first answer: %t %v", changed, err)
	}
	changed, err = users.SetServiceUpdates(ctx, "42", false, u.ServiceUpdatesPromptKey)
	if err != nil || changed {
		t.Fatalf("stale answer: %t %v", changed, err)
	}
}

func TestOutboxListenerDoesNotOccupyPoolAndWakesOnCommit(t *testing.T) {
	db, parent := openOperationalIntegrationDB(t)
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	q := repository.NewNotificationRepository(db)
	wake := make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() { done <- q.ListenOutbox(ctx, wake) }()
	select {
	case <-wake:
	case err := <-done:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if _, err := db.ExecContext(ctx, `SELECT pg_notify('scheduler_outbox_ready','')`); err != nil {
		t.Fatal(err)
	}
	select {
	case <-wake:
	case err := <-done:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("listener did not stop")
	}
}

func TestBroadcastAttachmentLimitsOrderAndRetention(t *testing.T) {
	db, ctx := openOperationalIntegrationDB(t)
	if _, err := db.ExecContext(ctx, `INSERT INTO users(id,service_updates_consent) VALUES('42',TRUE)`); err != nil {
		t.Fatal(err)
	}
	r := repository.NewBroadcastRepository(db)
	id, err := r.Create(ctx, "42")
	if err != nil {
		t.Fatal(err)
	}
	a := domain.BroadcastAttachment{Filename: "test.txt", MediaType: "document", ContentType: "text/plain", Data: []byte("test")}
	for i := 1; i <= 10; i++ {
		if err = r.AddAttachment(ctx, id, i, a); err != nil {
			t.Fatal(err)
		}
	}
	if err = r.AddAttachment(ctx, id, 11, a); err == nil {
		t.Fatal("accepted eleventh attachment")
	}
	if err = r.RemoveAttachment(ctx, id, "missing", 1); err == nil {
		t.Fatal("stale version accepted")
	}
	b, err := r.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, len(b.Attachments))
	for i, a := range b.Attachments {
		ids[len(ids)-1-i] = a.ID
	}
	doc := json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Hello"}]}]}`)
	if err = r.Save(ctx, id, repository.BroadcastInput{Document: doc, AudienceMode: "selected", UserIDs: []string{"42"}, Version: 11, AttachmentIDs: ids}); err != nil {
		t.Fatal(err)
	}
	b, err = r.Get(ctx, id)
	if err != nil || b.Attachments[0].ID != ids[0] {
		t.Fatalf("order not saved: %+v %v", b, err)
	}
	if err = r.Send(ctx, id, uuid.NewString(), 12); err != nil {
		t.Fatal(err)
	}
	q := repository.NewNotificationRepository(db)
	items, err := q.ClaimBotOutbox(ctx, 20)
	if err != nil || len(items) != 1 {
		t.Fatalf("queue %+v %v", items, err)
	}
	if err = q.MarkBotOutboxDelivered(ctx, items[0].ID, items[0].ClaimToken); err != nil {
		t.Fatal(err)
	}
	if err = q.CompleteBroadcasts(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `UPDATE broadcasts SET completed_at=NOW()-INTERVAL '91 days' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err = q.PruneCompleted(ctx, 90*24*time.Hour); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.GetContext(ctx, &count, `SELECT COUNT(*) FROM broadcast_attachments WHERE broadcast_id=$1`, id); err != nil || count != 0 {
		t.Fatalf("attachments retained: %d %v", count, err)
	}
}
