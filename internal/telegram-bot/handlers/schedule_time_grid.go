package handlers

import (
	"context"
	"log/slog"
	"time"

	"github.com/J0es1ick/Scheduler/internal/scheduleview"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/dto"
)

func (h *Handler) visualScheduleRequest(ctx context.Context, target *scheduleTarget, days []dto.DaySchedule, from time.Time, count int) scheduleview.Request {
	request := scheduleRenderRequest(target, days, from, count)
	if h.TimeSlotService != nil {
		slots, err := h.TimeSlotService.GetTimeSlots(ctx, target.UniversityID, from, from.AddDate(0, 0, count-1))
		if err != nil {
			slog.Warn("load time grid for visual schedule failed", "university_id", target.UniversityID, "err", err)
		} else {
			request.TimeSlots = slots
		}
	}
	return request
}
