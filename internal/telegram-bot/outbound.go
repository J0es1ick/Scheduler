package bot

import (
	"context"
	"strconv"
	"time"

	"github.com/J0es1ick/Scheduler/internal/telegramlimit"
	tele "gopkg.in/telebot.v3"
)

const telegramInteractiveWait = 15 * time.Second

type limitedContext struct {
	tele.Context
	parent  context.Context
	limiter *telegramlimit.Limiter
}

func LimitOutboundByRecipient(parent context.Context, limiter *telegramlimit.Limiter) tele.MiddlewareFunc {
	return func(next tele.HandlerFunc) tele.HandlerFunc {
		return func(current tele.Context) error {
			if limiter == nil {
				return next(current)
			}
			return next(&limitedContext{Context: current, parent: parent, limiter: limiter})
		}
	}
}

func (current *limitedContext) Send(what interface{}, options ...interface{}) error {
	return current.call(func() error { return current.Context.Send(what, options...) })
}

func (current *limitedContext) SendAlbum(album tele.Album, options ...interface{}) error {
	return current.call(func() error { return current.Context.SendAlbum(album, options...) })
}

func (current *limitedContext) Reply(what interface{}, options ...interface{}) error {
	return current.call(func() error { return current.Context.Reply(what, options...) })
}

func (current *limitedContext) Forward(message tele.Editable, options ...interface{}) error {
	return current.call(func() error { return current.Context.Forward(message, options...) })
}

func (current *limitedContext) ForwardTo(recipient tele.Recipient, options ...interface{}) error {
	return current.call(func() error { return current.Context.ForwardTo(recipient, options...) })
}

func (current *limitedContext) Edit(what interface{}, options ...interface{}) error {
	return current.call(func() error { return current.Context.Edit(what, options...) })
}

func (current *limitedContext) EditCaption(caption string, options ...interface{}) error {
	return current.call(func() error { return current.Context.EditCaption(caption, options...) })
}

func (current *limitedContext) EditOrSend(what interface{}, options ...interface{}) error {
	return current.call(func() error { return current.Context.EditOrSend(what, options...) })
}

func (current *limitedContext) EditOrReply(what interface{}, options ...interface{}) error {
	return current.call(func() error { return current.Context.EditOrReply(what, options...) })
}

func (current *limitedContext) Delete() error {
	return current.call(current.Context.Delete)
}

func (current *limitedContext) DeleteAfter(delay time.Duration) *time.Timer {
	return time.AfterFunc(delay, func() {
		_ = current.Delete()
	})
}

func (current *limitedContext) Notify(action tele.ChatAction) error {
	return current.call(func() error { return current.Context.Notify(action) })
}

func (current *limitedContext) Ship(what ...interface{}) error {
	return current.call(func() error { return current.Context.Ship(what...) })
}

func (current *limitedContext) Accept(errorMessage ...string) error {
	return current.call(func() error { return current.Context.Accept(errorMessage...) })
}

func (current *limitedContext) Answer(response *tele.QueryResponse) error {
	return current.call(func() error { return current.Context.Answer(response) })
}

func (current *limitedContext) Respond(response ...*tele.CallbackResponse) error {
	return current.call(func() error { return current.Context.Respond(response...) })
}

func (current *limitedContext) RespondText(text string) error {
	return current.call(func() error { return current.Context.RespondText(text) })
}

func (current *limitedContext) RespondAlert(text string) error {
	return current.call(func() error { return current.Context.RespondAlert(text) })
}

func (current *limitedContext) call(send func() error) error {
	parent := current.parent
	if parent == nil {
		parent = context.Background()
	}
	waitContext, cancel := context.WithTimeout(parent, telegramInteractiveWait)
	defer cancel()
	if err := current.limiter.Wait(waitContext, outboundRecipient(current.Context)); err != nil {
		return err
	}
	err := send()
	current.limiter.Observe(err)
	return err
}

func outboundRecipient(current tele.Context) string {
	if current == nil {
		return ""
	}
	if chat := current.Chat(); chat != nil {
		return strconv.FormatInt(chat.ID, 10)
	}
	if sender := current.Sender(); sender != nil {
		return strconv.FormatInt(sender.ID, 10)
	}
	return ""
}
