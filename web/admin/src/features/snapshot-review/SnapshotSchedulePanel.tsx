import { CalendarRange, Clock3, MapPin, UserRound } from "lucide-react";
import type { SnapshotLesson } from "../../types";
import { buildScheduleWeekSections } from "../schedule-shared/weekSections";
import {
  formatSnapshotDate,
  snapshotDays,
  snapshotLessonDay,
  snapshotLessonTypeLabels,
  snapshotWeekLabels,
} from "./model";

export function SnapshotSchedulePanel({
  title,
  subtitle,
  lessons,
  emptyText,
  tone,
  splitAlternatingWeeks,
}: {
  title: string;
  subtitle: string;
  lessons: SnapshotLesson[];
  emptyText: string;
  tone: "current" | "candidate";
  splitAlternatingWeeks: boolean;
}) {
  const weekSections = buildScheduleWeekSections(
    lessons,
    "all",
    splitAlternatingWeeks,
  );

  return (
    <section className={`snapshot-schedule-panel is-${tone}`}>
      <header>
        <div>
          <span>{tone === "current" ? "До публикации" : "После публикации"}</span>
          <h4>{title}</h4>
        </div>
        <strong>{lessons.length}</strong>
      </header>
      <p className="snapshot-schedule-subtitle">{subtitle}</p>
      {lessons.length ? (
        <div className="snapshot-week-sections">
          {weekSections.map((section) => (
            <section
              className={`snapshot-week-section is-${section.key}`}
              key={section.key}
            >
              {section.title && (
                <header className="snapshot-week-heading">
                  <div>
                    <span>Учебная неделя</span>
                    <h5>{section.title}</h5>
                  </div>
                  <p>{section.note}</p>
                  <strong>{section.lessons.length}</strong>
                </header>
              )}
              <SnapshotDays lessons={section.lessons} />
            </section>
          ))}
        </div>
      ) : (
        <div className="snapshot-schedule-empty">
          <CalendarRange size={22} />
          <strong>Занятий нет</strong>
          <p>{emptyText}</p>
        </div>
      )}
    </section>
  );
}

function SnapshotDays({ lessons }: { lessons: SnapshotLesson[] }) {
  if (!lessons.length) {
    return (
      <div className="snapshot-week-empty">
        В этой части недели занятий нет
      </div>
    );
  }

  return (
    <div className="snapshot-schedule-days">
      {snapshotDays.map((day, index) => {
        const dayLessons = lessons.filter(
          (lesson) => snapshotLessonDay(lesson) === index + 1,
        );
        if (!dayLessons.length) return null;
        return (
          <section className="snapshot-schedule-day" key={day}>
            <header>
              <span>{String(index + 1).padStart(2, "0")}</span>
              <h5>{day}</h5>
              <em>{dayLessons.length}</em>
            </header>
            <div>
              {dayLessons.map((lesson, lessonIndex) => (
                <SnapshotLessonCard
                  key={`${lesson.id}-${lessonIndex}`}
                  lesson={lesson}
                />
              ))}
            </div>
          </section>
        );
      })}
    </div>
  );
}

function SnapshotLessonCard({ lesson }: { lesson: SnapshotLesson }) {
  const validity =
    lesson.valid_from || lesson.valid_to
      ? `${formatSnapshotDate(lesson.valid_from)} — ${formatSnapshotDate(lesson.valid_to)}`
      : "Период действия не указан";

  return (
    <article className={`snapshot-lesson-card is-${lesson.diff}`}>
      <div className="snapshot-lesson-topline">
        <span>
          <Clock3 size={13} /> {lesson.time_start}–{lesson.time_end}
        </span>
        {lesson.diff !== "unchanged" && (
          <em>{lesson.diff === "added" ? "добавлено" : "удалено"}</em>
        )}
      </div>
      <small>{snapshotLessonTypeLabels[lesson.type] ?? lesson.type}</small>
      <h6>{lesson.subject || "Предмет не указан"}</h6>
      <div className="snapshot-lesson-details">
        <span>
          <UserRound size={13} /> {lesson.teacher || "Преподаватель не указан"}
        </span>
        <span>
          <MapPin size={13} /> {lesson.room || "Аудитория не указана"}
        </span>
      </div>
      <footer>
        <span>{snapshotWeekLabels[lesson.week_type]}</span>
        {lesson.subgroup > 0 && <span>{lesson.subgroup} подгр.</span>}
        <span>{validity}</span>
      </footer>
    </article>
  );
}
