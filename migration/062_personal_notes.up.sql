CREATE TABLE personal_notes (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('student','teacher')),
    target_id TEXT NOT NULL,
    lesson_id TEXT NOT NULL,
    lesson_date DATE NOT NULL,
    basis JSONB NOT NULL CHECK (jsonb_typeof(basis)='object'),
    body TEXT NOT NULL CHECK (length(body) BETWEEN 1 AND 4000),
    due_date DATE,
    completed BOOLEAN NOT NULL DEFAULT FALSE,
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX personal_notes_owner_idx ON personal_notes(user_id,completed,due_date,lesson_date);
