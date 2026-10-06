//go:build integration

package handlers

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/state"
	"github.com/google/uuid"
)

func TestRuntimeTeacherConsentRecoveryAndBroadcastPermissions(t *testing.T) {
	j := newRuntimeJourney(t)
	j.message(t, j.h.HandleStart, "/start", "")
	j.pressAction(t, "select_role", j.h.HandleSelectRole, func(args string) bool { return strings.HasPrefix(args, "teacher|") })
	current := j.h.StateManager.Get(42)
	j.callback(t, j.h.HandleUniversitySelect, "isuct|"+current.FlowNonce)
	j.message(t, j.h.HandleTextInput, "Иванов", "")
	j.pressAction(t, "confirm_teacher", j.h.HandleConfirmTeacher, func(args string) bool { return strings.HasPrefix(args, "save|") })
	j.pressAction(t, "daily_action", j.h.HandleDailyAction, func(args string) bool { return strings.HasPrefix(args, "on|") })
	j.pressAction(t, "daily_action", j.h.HandleDailyAction, func(args string) bool { return strings.HasPrefix(args, "07:00|") })
	if !strings.Contains(j.telegram.text, "об обновлениях сервиса") {
		t.Fatal("missing consent question")
	}
	j.h.StateManager = state.NewManager()
	j.message(t, j.h.HandleStart, "/start", "")
	if !strings.Contains(j.telegram.text, "об обновлениях сервиса") {
		t.Fatal("unfinished consent lost after restart")
	}
	j.pressAction(t, "updates_choice", j.h.HandleUpdatesChoice, func(args string) bool { return strings.HasPrefix(args, "off|") })
	j.message(t, j.h.HandleStart, "/start", "")
	if strings.Contains(j.telegram.text, "об обновлениях сервиса") {
		t.Fatal("answered prompt repeated")
	}
	j.message(t, j.h.HandleUpdates, "/updates", "on")
	u, err := j.h.UserService.GetUser(j.ctx, "42")
	if err != nil || u.Role != domain.RoleTeacher || !u.DailyEnabled || u.DailyTime != "07:00" || u.ServiceUpdatesConsent == nil || !*u.ServiceUpdatesConsent {
		t.Fatalf("profile %+v %v", u, err)
	}
	broadcasts := repository.NewBroadcastRepository(j.adminDB)
	id, err := broadcasts.Create(j.ctx, "43")
	if err != nil {
		t.Fatal(err)
	}
	doc := json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"News"}]}]}`)
	if err = broadcasts.Save(j.ctx, id, repository.BroadcastInput{Document: doc, AudienceMode: "all", Version: 1}); err != nil {
		t.Fatal(err)
	}
	if err = broadcasts.AddAttachment(j.ctx, id, 2, domain.BroadcastAttachment{Filename: "news.txt", MediaType: "document", ContentType: "text/plain", Data: []byte("news")}); err != nil {
		t.Fatal(err)
	}
	if err = broadcasts.Send(j.ctx, id, uuid.NewString(), 3); err != nil {
		t.Fatal(err)
	}
	q := repository.NewNotificationRepository(j.botDB)
	items, err := q.ClaimBotOutbox(j.ctx, 10)
	if err != nil || len(items) != 1 {
		t.Fatalf("claims %+v %v", items, err)
	}
	files, err := q.BroadcastAttachments(j.ctx, id)
	if err != nil || len(files) != 1 {
		t.Fatalf("files %+v %v", files, err)
	}
	if _, err = q.BroadcastAttachmentData(j.ctx, files[0].ID); err != nil {
		t.Fatal(err)
	}
	item := items[0]
	if err = q.SaveBroadcastPart(j.ctx, item.ID, item.ClaimToken, 0, 111, "", ""); err != nil {
		t.Fatal(err)
	}
	if err = q.SaveBroadcastPart(j.ctx, item.ID, item.ClaimToken, 1, 112, files[0].ID, "telegram-file"); err != nil {
		t.Fatal(err)
	}
	if err = q.MarkBotOutboxDelivered(j.ctx, item.ID, item.ClaimToken); err != nil {
		t.Fatal(err)
	}
	if err = q.CompleteBroadcasts(j.ctx); err != nil {
		t.Fatal(err)
	}
	b, err := broadcasts.Get(j.ctx, id)
	if err != nil || b.Status != "completed" || b.Counts["delivered"] != 1 {
		t.Fatalf("result %+v %v", b, err)
	}
	j.message(t, j.h.HandleUpdates, "/updates", "off")
}
