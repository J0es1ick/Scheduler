package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
)

func (r *LessonRepository) GetTimeSlots(ctx context.Context, universityID string, from, to time.Time) ([]domain.LessonTimeSlot, error) {
	slots := []domain.LessonTimeSlot{}
	err := r.db.SelectContext(ctx, &slots, `SELECT l.time_start,l.time_end
 FROM effective_lessons l JOIN groups g ON g.id=l.group_id AND g.is_active
 JOIN semesters s ON s.id=l.semester_id
 WHERE l.university_id=$1 AND COALESCE(l.valid_from,s.start_date) <= $3::date AND COALESCE(l.valid_to,s.end_date) >= $2::date
 GROUP BY l.time_start,l.time_end ORDER BY count(*) DESC,l.time_start,l.time_end LIMIT 64`, universityID, from.Format(time.DateOnly), to.Format(time.DateOnly))
	if err != nil {
		return nil, fmt.Errorf("load university time slots: %w", err)
	}
	return slots, nil
}
