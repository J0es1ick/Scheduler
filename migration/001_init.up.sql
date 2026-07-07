CREATE TYPE week_type AS ENUM ('every', 'odd', 'even', 'date');
CREATE TYPE lesson_type AS ENUM ('lecture', 'practice', 'lab', 'seminar');
CREATE TYPE subscription_object_type AS ENUM ('group', 'teacher', 'room');
CREATE TYPE parse_log_status AS ENUM ('running', 'success', 'failed');

CREATE TABLE universities (
    id           TEXT PRIMARY KEY,
    name         TEXT NOT NULL,
    full_name    TEXT,
    schedule_url TEXT,
    is_active    BOOLEAN NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE groups (
    id            TEXT PRIMARY KEY,
    university_id TEXT NOT NULL REFERENCES universities(id) ON DELETE CASCADE,
    name          TEXT NOT NULL,
    is_active     BOOLEAN NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMP NOT NULL DEFAULT NOW(),
    UNIQUE(university_id, name)
);

CREATE TABLE semesters (
    id            TEXT PRIMARY KEY,
    university_id TEXT NOT NULL REFERENCES universities(id) ON DELETE CASCADE,
    name          TEXT NOT NULL,
    start_date    DATE NOT NULL,
    end_date      DATE NOT NULL,
    CHECK (start_date <= end_date)
);

CREATE TABLE lessons (
    id            TEXT PRIMARY KEY,
    university_id TEXT NOT NULL REFERENCES universities(id) ON DELETE CASCADE,
    semester_id   TEXT NOT NULL REFERENCES semesters(id) ON DELETE CASCADE,
    day_of_week   INT,
    special_date  DATE,
    time_start    TEXT NOT NULL,
    time_end      TEXT NOT NULL,
    week_type     week_type NOT NULL,
    subject       TEXT NOT NULL,
    type          lesson_type NOT NULL,
    teacher       TEXT NOT NULL DEFAULT '',
    room          TEXT NOT NULL DEFAULT '',
    group_id      TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    subgroup      INT DEFAULT 0,
    updated_at    TIMESTAMP NOT NULL DEFAULT NOW(),
    CHECK (
        (week_type = 'date' AND special_date IS NOT NULL AND day_of_week IS NULL) OR
        (week_type != 'date' AND special_date IS NULL AND day_of_week BETWEEN 1 AND 6)
    )
);

CREATE TABLE users (
    id         TEXT PRIMARY KEY,
    username   TEXT,
    is_admin   BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE subscriptions (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    object_id   TEXT NOT NULL,
    object_type subscription_object_type NOT NULL,
    created_at  TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMP NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, object_id, object_type)
);

CREATE TABLE data_sources (
    id              TEXT PRIMARY KEY,
    university_id   TEXT NOT NULL REFERENCES universities(id) ON DELETE CASCADE,
    adapter_type    TEXT NOT NULL,
    config          TEXT,
    update_interval INT NOT NULL DEFAULT 600,
    last_run_at     TIMESTAMP,
    last_error      TEXT,
    created_at      TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE parse_logs (
    id              TEXT PRIMARY KEY,
    data_source_id  TEXT NOT NULL REFERENCES data_sources(id) ON DELETE CASCADE,
    started_at      TIMESTAMP NOT NULL,
    finished_at     TIMESTAMP,
    status          parse_log_status NOT NULL,
    records_fetched INT DEFAULT 0,
    error_message   TEXT,
    CHECK ( (status = 'running' AND finished_at IS NULL) OR
            (status IN ('success', 'failed') AND finished_at IS NOT NULL) )
);

CREATE INDEX idx_universities_is_active ON universities(is_active);

CREATE INDEX idx_groups_university_id ON groups(university_id);
CREATE INDEX idx_groups_name ON groups(name);

CREATE INDEX idx_semesters_university_id ON semesters(university_id);
CREATE INDEX idx_semesters_dates ON semesters(start_date, end_date);

CREATE INDEX idx_lessons_group_id ON lessons(group_id);
CREATE INDEX idx_lessons_semester_id ON lessons(semester_id);
CREATE INDEX idx_lessons_teacher ON lessons(teacher);
CREATE INDEX idx_lessons_room ON lessons(room);
CREATE INDEX idx_lessons_date_range ON lessons(special_date) WHERE special_date IS NOT NULL;
CREATE INDEX idx_lessons_week_day ON lessons(day_of_week, week_type) WHERE week_type != 'date';

CREATE INDEX idx_subscriptions_user_id ON subscriptions(user_id);
CREATE INDEX idx_subscriptions_object ON subscriptions(object_id, object_type);

CREATE INDEX idx_data_sources_university_id ON data_sources(university_id);
CREATE INDEX idx_data_sources_adapter_type ON data_sources(adapter_type);

CREATE INDEX idx_parse_logs_data_source_id ON parse_logs(data_source_id);
CREATE INDEX idx_parse_logs_started_at ON parse_logs(started_at);