ALTER TABLE personal_schedule_overrides
    ADD COLUMN basis JSONB NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(basis)='object'),
    ADD COLUMN occurrences JSONB NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(occurrences)='array'),
    ADD COLUMN needs_review BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE personal_schedule_overrides DROP CONSTRAINT personal_schedule_overrides_scope_check;
ALTER TABLE personal_schedule_overrides ADD CONSTRAINT personal_schedule_overrides_scope_check CHECK (scope IN ('day','semester','selected'));
UPDATE personal_schedule_overrides o SET basis=jsonb_build_object(
    'ID',l.id,'GroupID',l.group_id,'SemesterID',l.semester_id,'DayOfWeek',COALESCE(l.day_of_week,EXTRACT(ISODOW FROM l.special_date)::int,0),
    'TimeStart',l.time_start,'TimeEnd',l.time_end,
    'Subgroup',l.subgroup,'WeekType',l.week_type,'SpecialDate',to_char(l.special_date,'YYYY-MM-DD"T"00:00:00"Z"'),
    'ValidFrom',to_char(l.valid_from,'YYYY-MM-DD"T"00:00:00"Z"'),'ValidTo',to_char(l.valid_to,'YYYY-MM-DD"T"00:00:00"Z"'),'recurrence',l.recurrence
) FROM effective_lessons l WHERE COALESCE(l.base_lesson_id,l.id)=o.lesson_id AND l.semester_id=o.semester_id;

CREATE FUNCTION scheduler_personal_publication_changed(university TEXT, changed_groups TEXT[]) RETURNS VOID
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $$
BEGIN
    UPDATE personal_schedule_overrides SET needs_review=TRUE,version=version+1,updated_at=NOW()
    WHERE university_id=university AND (
        basis->>'GroupID'=ANY(changed_groups)
        OR (role='student' AND target_id=ANY(changed_groups))
        OR (role='teacher' AND basis='{}'::jsonb)
    );
END;
$$;
DO $$ BEGIN
    EXECUTE format('ALTER FUNCTION scheduler_personal_publication_changed(TEXT,TEXT[]) SET search_path TO pg_catalog, %I, pg_temp',current_schema());
END; $$;
REVOKE ALL ON FUNCTION scheduler_personal_publication_changed(TEXT,TEXT[]) FROM PUBLIC;
