package bot

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/dto"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/state"
	tele "gopkg.in/telebot.v3"
)

type restrictionReaderFunc func(context.Context, string) (domain.UserRestrictions, error)

func (f restrictionReaderFunc) Restrictions(ctx context.Context, id string) (domain.UserRestrictions, error) {
	return f(ctx, id)
}

func TestRestrictionsChangeWithoutRestartAcrossUpdateTypes(t *testing.T) {
	b, err := tele.NewBot(tele.Settings{Offline: true, Token: "synthetic"})
	if err != nil {
		t.Fatal(err)
	}
	manager := state.NewManager()
	var restrictions domain.UserRestrictions
	reader := restrictionReaderFunc(func(_ context.Context, id string) (domain.UserRestrictions, error) {
		if id != "42" {
			t.Fatalf("checked chat instead of sender: %s", id)
		}
		return restrictions, nil
	})
	called := 0
	handler := EnforceUserRestrictions(context.Background(), reader, manager)(func(tele.Context) error { called++; return nil })
	message := func(text string) tele.Update {
		return tele.Update{Message: &tele.Message{Text: text, Sender: &tele.User{ID: 42}, Chat: &tele.Chat{ID: 42, Type: tele.ChatPrivate}}}
	}
	callback := func(name string) tele.Update {
		return tele.Update{Callback: &tele.Callback{Sender: &tele.User{ID: 42}, Unique: name, Message: &tele.Message{Chat: &tele.Chat{ID: 42, Type: tele.ChatPrivate}}}}
	}
	for _, test := range []struct {
		name    string
		update  tele.Update
		support bool
		step    string
	}{
		{"start", message("/start"), false, ""},
		{"today", message("/today"), false, ""},
		{"inline", tele.Update{Query: &tele.Query{Sender: &tele.User{ID: 42}, Text: "4/147"}}, false, ""},
		{"group", tele.Update{Message: &tele.Message{Text: "/today", Sender: &tele.User{ID: 42}, Chat: &tele.Chat{ID: -100, Type: tele.ChatSuperGroup}}}, false, ""},
		{"report", message("/report@SchedulerBot"), true, ""},
		{"hotline", message("/hotline"), true, ""},
		{"hotline button", message("Горячая линия"), true, ""},
		{"old report link", message("/start report_old"), true, ""},
		{"open hotline", callback("open_hotline"), true, ""},
		{"choose type", callback("select_hotline_type"), true, ""},
		{"cancel form", callback("cancel_hotline"), true, ""},
		{"old report button", callback("schedule_feedback"), true, ""},
		{"schedule callback", callback("schedule_day"), false, "awaiting_hotline_submission"},
		{"already open form", message("This is a pending feedback message"), true, "awaiting_hotline_submission"},
		{"short feedback", message("oops"), true, "awaiting_hotline_submission"},
		{"interrupt form", message("/today"), false, "awaiting_hotline_submission"},
		{"schedule button interrupts form", message("Сегодня"), false, "awaiting_hotline_submission"},
	} {
		t.Run(test.name, func(t *testing.T) {
			manager.Set(42, &dto.UserState{Step: test.step})
			for _, flags := range []domain.UserRestrictions{{BotBlocked: true}, {SupportBlocked: true}, {BotBlocked: true, SupportBlocked: true}, {}} {
				restrictions = flags
				before := called
				if err := handler(b.NewContext(test.update)); err != nil {
					t.Fatal(err)
				}
				wantBlocked := flags.BotBlocked || (flags.SupportBlocked && test.support)
				if (called == before) != wantBlocked {
					t.Fatalf("flags=%+v invoked=%v", flags, called != before)
				}
			}
		})
	}
	errUnavailable := errors.New("database unavailable")
	reader = restrictionReaderFunc(func(context.Context, string) (domain.UserRestrictions, error) {
		return domain.UserRestrictions{}, errUnavailable
	})
	err = EnforceUserRestrictions(context.Background(), reader, manager)(func(tele.Context) error { t.Fatal("failed open"); return nil })(b.NewContext(message("/start")))
	if !errors.Is(err, errUnavailable) {
		t.Fatalf("error=%v", err)
	}
}

type restrictionSendContext struct {
	tele.Context
	sent *atomic.Int32
}

func (c restrictionSendContext) Send(interface{}, ...interface{}) error { c.sent.Add(1); return nil }

func TestBanSuppressesResponseFromAlreadyRunningHandler(t *testing.T) {
	b, _ := tele.NewBot(tele.Settings{Offline: true, Token: "synthetic"})
	var blocked atomic.Bool
	var sent atomic.Int32
	reader := restrictionReaderFunc(func(context.Context, string) (domain.UserRestrictions, error) {
		return domain.UserRestrictions{BotBlocked: blocked.Load()}, nil
	})
	entered, release := make(chan struct{}), make(chan struct{})
	handler := LimitOutboundByRecipient(context.Background(), nil, reader)(func(c tele.Context) error { close(entered); <-release; return c.Send("rendered schedule") })
	c := restrictionSendContext{Context: b.NewContext(tele.Update{Message: &tele.Message{Text: "/today", Sender: &tele.User{ID: 42}, Chat: &tele.Chat{ID: 42, Type: tele.ChatPrivate}}}), sent: &sent}
	done := make(chan error, 1)
	go func() { done <- handler(c) }()
	<-entered
	blocked.Store(true)
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if sent.Load() != 0 {
		t.Fatal("response sent after ban")
	}
	blocked.Store(false)
	if err := LimitOutboundByRecipient(context.Background(), nil, reader)(func(c tele.Context) error { return c.Send("schedule") })(c); err != nil {
		t.Fatal(err)
	}
	if sent.Load() != 1 {
		t.Fatal("response not restored after unban")
	}
}

func TestBanIsRecheckedAfterWaitingInSenderQueue(t *testing.T) {
	b, err := tele.NewBot(tele.Settings{Offline: true, Token: "synthetic"})
	if err != nil {
		t.Fatal(err)
	}
	var blocked atomic.Bool
	var calls atomic.Int32
	reader := restrictionReaderFunc(func(context.Context, string) (domain.UserRestrictions, error) {
		return domain.UserRestrictions{BotBlocked: blocked.Load()}, nil
	})
	entered, release := make(chan struct{}), make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	guard := EnforceUserRestrictions(context.Background(), reader, nil)
	handler := guard(SerializeBySender(2)(guard(func(tele.Context) error {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
		return nil
	})))
	c := b.NewContext(tele.Update{Message: &tele.Message{Text: "/today", Sender: &tele.User{ID: 42}, Chat: &tele.Chat{ID: 42, Type: tele.ChatPrivate}}})
	done := make(chan error, 2)
	go func() { done <- handler(c) }()
	<-entered
	waiting := handlerWaiting.Load()
	go func() { done <- handler(c) }()
	deadline := time.After(time.Second)
	for handlerWaiting.Load() <= waiting {
		select {
		case <-deadline:
			t.Fatal("second update did not enter the sender queue")
		case <-time.After(time.Millisecond):
		}
	}
	blocked.Store(true)
	close(release)
	for range 2 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 1 {
		t.Fatal("queued update ran after ban")
	}
}
