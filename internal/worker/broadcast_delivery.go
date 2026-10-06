package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/keyboards"
	tele "gopkg.in/telebot.v3"
)

type broadcastDeliveryRepository interface {
	BroadcastAttachments(context.Context, string) ([]domain.BroadcastAttachment, error)
	BroadcastAttachmentData(context.Context, string) ([]byte, error)
	SaveBroadcastPart(context.Context, string, string, int, int, string, string) error
}

func (w *NotificationWorker) sendBroadcast(ctx context.Context, item domain.BotOutboxDelivery) error {
	repo, ok := w.repository.(broadcastDeliveryRepository)
	if !ok {
		return fmt.Errorf("broadcast delivery unavailable")
	}
	attachments, err := repo.BroadcastAttachments(ctx, item.BroadcastID)
	if err != nil {
		return err
	}
	var ids []int
	if err = json.Unmarshal(item.BroadcastMessageIDs, &ids); err != nil {
		return err
	}
	if len(ids) > len(attachments)+1 {
		return fmt.Errorf("invalid broadcast progress")
	}
	userID, err := strconv.ParseInt(item.UserID, 10, 64)
	if err != nil {
		return err
	}
	for part := len(ids); part <= len(attachments); part++ {
		if err = w.waitForTelegram(ctx, item.UserID); err != nil {
			return err
		}
		decision, err := w.repository.BotOutboxDecision(ctx, item.ID, item.ClaimToken)
		if err != nil {
			return err
		}
		if decision != repository.NotificationQueueReady {
			return errDailyCancelled
		}
		var body any = item.Body
		options := []any{tele.ModeHTML, keyboards.ServiceUpdatesUnsubscribe()}
		var attachmentID string
		if part > 0 {
			a := attachments[part-1]
			attachmentID = a.ID
			file := tele.File{FileID: a.TelegramFileID}
			if a.TelegramFileID == "" {
				data, err := repo.BroadcastAttachmentData(ctx, a.ID)
				if err != nil {
					return err
				}
				file = tele.FromReader(bytes.NewReader(data))
			}
			if a.MediaType == "photo" {
				body = &tele.Photo{File: file}
			} else {
				body = &tele.Document{File: file, FileName: a.Filename, MIME: a.ContentType}
			}
			options = nil
		}
		message, err := w.bot.Send(&tele.User{ID: userID}, body, options...)
		w.limiter.Observe(err)
		if err != nil {
			return err
		}
		if message == nil || message.ID == 0 {
			return fmt.Errorf("Telegram did not acknowledge broadcast message")
		}
		fileID := ""
		if message.Photo != nil {
			fileID = message.Photo.FileID
		}
		if message.Document != nil {
			fileID = message.Document.FileID
		}
		if err = repo.SaveBroadcastPart(ctx, item.ID, item.ClaimToken, part, message.ID, attachmentID, fileID); err != nil {
			return err
		}
	}
	return nil
}
