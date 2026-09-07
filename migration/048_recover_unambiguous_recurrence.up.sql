-- Before 045 the editor could not intentionally edit recurrence. Recover only
-- overrides whose entire calendar definition still agrees with the source.
-- Ambiguous rows are reported by scripts/release-data-audit.sql for review.
UPDATE lesson_overrides o
SET recurrence=l.recurrence, updated_at=NOW(), version=o.version+1
FROM lessons l
WHERE l.id=o.base_lesson_id
  AND o.recurrence='{}'::jsonb AND l.recurrence<>'{}'::jsonb
  AND ROW(o.semester_id,o.day_of_week,o.special_date,o.week_type,o.valid_from,o.valid_to)
      IS NOT DISTINCT FROM ROW(l.semester_id,l.day_of_week,l.special_date,l.week_type,l.valid_from,l.valid_to);
