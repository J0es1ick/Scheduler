package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/telegramlimit"
	tele "gopkg.in/telebot.v3"
)

type broadcastProgressStub struct {
	notificationRepositoryStub
	ids    []int
	cached string
	reads  int
}

func (r *broadcastProgressStub) BroadcastAttachments(context.Context, string) ([]domain.BroadcastAttachment, error) {
	return []domain.BroadcastAttachment{{ID: "file", Filename: "notes.txt", MediaType: "document", TelegramFileID: r.cached}}, nil
}
func (r *broadcastProgressStub) BroadcastAttachmentData(context.Context, string) ([]byte, error) {
	r.reads++
	return []byte("notes"), nil
}
func (r *broadcastProgressStub) SaveBroadcastPart(_ context.Context, _, _ string, part, id int, attachment, fileID string) error {
	if part != len(r.ids) {
		return errors.New("wrong progress")
	}
	r.ids = append(r.ids, id)
	if attachment != "" {
		r.cached = fileID
	}
	return nil
}
func TestBroadcastResumesAfterConfirmedText(t *testing.T) {
	repo := &broadcastProgressStub{}
	textCount, fileCount := 0, 0
	fail := true
	w := &NotificationWorker{repository: repo, limiter: telegramlimit.New(0, 0), waitSend: func(context.Context, string) error { return nil }, bot: notificationSenderFunc(func(_ tele.Recipient, body any, options ...any) (*tele.Message, error) {
		if _, ok := body.(string); ok {
			textCount++
			return &tele.Message{ID: 1}, nil
		}
		if fail {
			fail = false
			return nil, errors.New("temporary send failure")
		}
		fileCount++
		return &tele.Message{ID: 2, Document: &tele.Document{File: tele.File{FileID: "telegram-file"}}}, nil
	})}
	item := domain.BotOutboxDelivery{ID: "delivery", UserID: "42", BroadcastID: "broadcast", Body: "Text", BroadcastMessageIDs: json.RawMessage(`[]`)}
	if err := w.sendBroadcast(context.Background(), item); err == nil {
		t.Fatal("expected temporary failure")
	}
	if len(repo.ids) != 1 {
		t.Fatal("text progress missing")
	}
	item.BroadcastMessageIDs, _ = json.Marshal(repo.ids)
	if err := w.sendBroadcast(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	if textCount != 1 || fileCount != 1 || repo.cached != "telegram-file" {
		t.Fatalf("text %d files %d cached %q", textCount, fileCount, repo.cached)
	}
	reads := repo.reads
	repo.ids = nil
	item.BroadcastMessageIDs = json.RawMessage(`[]`)
	if err := w.sendBroadcast(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	if repo.reads != reads {
		t.Fatal("cached Telegram file was uploaded again")
	}
}
func TestBroadcastConsentCheckedBeforeAttachment(t *testing.T) {
	repo := &broadcastProgressStub{}
	sent := 0
	repo.active = func(context.Context, string, string) (bool, error) { return sent == 0, nil }
	w := &NotificationWorker{repository: repo, limiter: telegramlimit.New(0, 0), waitSend: func(context.Context, string) error { return nil }, bot: notificationSenderFunc(func(tele.Recipient, any, ...any) (*tele.Message, error) { sent++; return &tele.Message{ID: 1}, nil })}
	err := w.sendBroadcast(context.Background(), domain.BotOutboxDelivery{UserID: "42", BroadcastMessageIDs: json.RawMessage(`[]`)})
	if !errors.Is(err, errDailyCancelled) || sent != 1 || repo.reads != 0 {
		t.Fatalf("cancelled: %v sent=%d reads=%d", err, sent, repo.reads)
	}
}
