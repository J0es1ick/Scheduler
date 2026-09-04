package handlers

import (
	"context"

	tele "gopkg.in/telebot.v3"
)

func (h *Handler) sendTelegram(
	ctx context.Context,
	current tele.Context,
	recipient tele.Recipient,
	what interface{},
	options ...interface{},
) (*tele.Message, error) {
	if h.TelegramLimiter != nil {
		if err := h.TelegramLimiter.Wait(ctx, recipient.Recipient()); err != nil {
			return nil, err
		}
	}
	message, err := current.Bot().Send(recipient, what, options...)
	if h.TelegramLimiter != nil {
		h.TelegramLimiter.Observe(err)
	}
	return message, err
}

func (h *Handler) deleteTelegram(ctx context.Context, current tele.Context, message tele.Editable) error {
	if h.TelegramLimiter != nil {
		if err := h.TelegramLimiter.Wait(ctx, current.Recipient().Recipient()); err != nil {
			return err
		}
	}
	err := current.Bot().Delete(message)
	if h.TelegramLimiter != nil {
		h.TelegramLimiter.Observe(err)
	}
	return err
}

func (h *Handler) editTelegramMarkup(ctx context.Context, current tele.Context, message tele.Editable, markup *tele.ReplyMarkup) error {
	if h.TelegramLimiter != nil {
		if err := h.TelegramLimiter.Wait(ctx, current.Recipient().Recipient()); err != nil {
			return err
		}
	}
	_, err := current.Bot().EditReplyMarkup(message, markup)
	if h.TelegramLimiter != nil {
		h.TelegramLimiter.Observe(err)
	}
	return err
}
