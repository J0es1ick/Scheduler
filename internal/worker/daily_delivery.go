package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/repository"
	tele "gopkg.in/telebot.v3"
)

type dailyScheduleProvider func(context.Context, string, time.Time) ([]domain.ScheduleMessage, error)

type scheduleProgressRepository interface {
	SaveScheduleMessages(context.Context, string, string, []domain.ScheduleMessage) error
	SaveDeliveredParts(context.Context, string, string, int) error
}

func (w *NotificationWorker) sendDailySchedule(ctx context.Context, item domain.BotOutboxDelivery) error {
	progress, ok := w.repository.(scheduleProgressRepository)
	if !ok || w.dailySchedule == nil {
		return fmt.Errorf("daily delivery provider unavailable")
	}
	var schedule domain.DailyContext
	if err := json.Unmarshal(item.ScheduleContext, &schedule); err != nil {
		return err
	}
	location, err := time.LoadLocation(schedule.Timezone)
	if err != nil {
		return err
	}
	date, err := time.ParseInLocation(time.DateOnly, schedule.Date, location)
	if err != nil {
		return err
	}
	var messages []domain.ScheduleMessage
	if err = json.Unmarshal(item.ScheduleMessages, &messages); err != nil {
		return err
	}
	if item.DeliveredParts == 0 {
		messages, err = w.dailySchedule(ctx, item.UserID, date)
		if err != nil {
			return err
		}
		if err = progress.SaveScheduleMessages(ctx, item.ID, item.ClaimToken, messages); err != nil {
			return err
		}
	}
	if len(messages) == 0 || item.DeliveredParts > len(messages) {
		return fmt.Errorf("invalid daily schedule messages")
	}
	userID, err := strconv.ParseInt(item.UserID, 10, 64)
	if err != nil {
		return err
	}
	for i := item.DeliveredParts; i < len(messages); i++ {
		if err = w.waitForTelegram(ctx, item.UserID); err != nil {
			return err
		}
		decision, decisionErr := w.repository.BotOutboxDecision(ctx, item.ID, item.ClaimToken)
		if decisionErr != nil {
			return decisionErr
		}
		if decision != repository.NotificationQueueReady {
			return errDailyCancelled
		}
		var markup *tele.ReplyMarkup
		if len(messages[i].Markup) > 0 {
			if err = json.Unmarshal(messages[i].Markup, &markup); err != nil {
				return err
			}
		}
		var body interface{} = messages[i].Text
		if len(messages[i].PNG) > 0 {
			body = &tele.Photo{File: tele.FromReader(bytes.NewReader(messages[i].PNG)), Caption: messages[i].Text}
		}
		_, err = w.bot.Send(&tele.User{ID: userID}, body, markup, tele.ModeHTML)
		w.limiter.Observe(err)
		if err != nil {
			return err
		}
		if err = progress.SaveDeliveredParts(ctx, item.ID, item.ClaimToken, i+1); err != nil {
			return err
		}
	}
	return nil
}

var errDailyCancelled = fmt.Errorf("daily schedule no longer active")
