ALTER TABLE lessons ADD COLUMN IF NOT EXISTS valid_from DATE;
ALTER TABLE lessons ADD COLUMN IF NOT EXISTS valid_to DATE;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'lessons_validity_check'
          AND conrelid = 'lessons'::regclass
    ) THEN
        ALTER TABLE lessons
            ADD CONSTRAINT lessons_validity_check CHECK (
                (valid_from IS NULL AND valid_to IS NULL) OR
                (valid_from IS NOT NULL AND valid_to IS NOT NULL AND valid_from <= valid_to)
            );
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_lessons_validity ON lessons(valid_from, valid_to);
