import { classNames } from "../../../../../shared/classNames";
import styles from "./ScheduleWeekBoard.module.css";
import stateStyles from "../../../ui/state/State.module.css";
import lessonTypeStyles from "../../../ui/lesson-type/LessonType.module.css";
import {
  Clock3,
  MapPin,
  Pencil,
  Plus,
  RotateCcw,
  Trash2,
  UserRound,
} from "lucide-react";
import type { EditorLesson } from "../../../shared/model";
import {
  days,
  daysAfterPreposition,
  lessonDay,
  lessonTypeLabels,
  pluralLessons,
  recurrenceLabel,
} from "../model";

export function ScheduleWeekBoard({
  title,
  note,
  lessons,
  busy,
  onAdd,
  onEdit,
  onDelete,
  onRestore,
}: {
  title: string;
  note: string;
  lessons: EditorLesson[];
  busy: boolean;
  onAdd: (day: number) => void;
  onEdit: (lesson: EditorLesson, day: number) => void;
  onDelete: (lesson: EditorLesson) => void;
  onRestore: (lesson: EditorLesson) => void;
}) {
  return (
    <section className={classNames(styles["schedule-week-section"], title ? stateStyles["has-title"] : undefined)}>
      {title && (
        <header className={styles["schedule-week-heading"]}>
          <div>
            <span>Учебная неделя</span>
            <h3>{title}</h3>
          </div>
          <p>{note}</p>
        </header>
      )}
      <div
        className={styles["week-board"]}
        aria-label={title || "Расписание по дням недели"}
      >
        {days.map((dayName, index) => {
          const day = index + 1;
          const dayLessons = lessons
            .filter((lesson) => lessonDay(lesson) === day)
            .sort((a, b) => a.time_start.localeCompare(b.time_start));
          return (
            <article className={styles["day-column"]} key={dayName}>
              <header>
                <div>
                  <span>{String(day).padStart(2, "0")}</span>
                  <h3>{dayName}</h3>
                  <p>
                    {dayLessons.length} {pluralLessons(dayLessons.length)}
                  </p>
                </div>
                <button
                  className={styles["day-add"]}
                  onClick={() => onAdd(day)}
                  aria-label={`Добавить занятие в ${daysAfterPreposition[index]}`}
                >
                  <Plus size={16} />
                </button>
              </header>
              <div className={styles["day-lessons"]}>
                {dayLessons.length ? (
                  dayLessons.map((lesson) => (
                    <LessonCard
                      key={lesson.id}
                      lesson={lesson}
                      disabled={busy}
                      onEdit={() => onEdit(lesson, day)}
                      onDelete={() => onDelete(lesson)}
                      onRestore={() => onRestore(lesson)}
                    />
                  ))
                ) : (
                  <button className={styles["day-empty"]} onClick={() => onAdd(day)}>
                    <Plus size={15} />
                    <span>Добавить первое занятие</span>
                  </button>
                )}
              </div>
            </article>
          );
        })}
      </div>
    </section>
  );
}

function LessonCard({
  lesson,
  disabled,
  onEdit,
  onDelete,
  onRestore,
}: {
  lesson: EditorLesson;
  disabled: boolean;
  onEdit: () => void;
  onDelete: () => void;
  onRestore: () => void;
}) {
  return (
    <article
      className={classNames(styles["board-lesson"], lessonTypeStyles[`type-${lesson.type}`], lesson.origin === "manual" ? stateStyles["is-manual"] : undefined)}
    >
      <div className={styles["board-lesson-head"]}>
        <span className={styles["board-time"]}>
          <Clock3 size={13} /> {lesson.time_start}–{lesson.time_end}
        </span>
        <div>
          <button
            onClick={onEdit}
            disabled={disabled}
            aria-label="Редактировать"
          >
            <Pencil size={14} />
          </button>
          <button onClick={onDelete} disabled={disabled} aria-label="Удалить">
            <Trash2 size={14} />
          </button>
        </div>
      </div>
      <span className={styles["board-type"]}>
        {lessonTypeLabels[lesson.type] ?? lesson.type}
      </span>
      <h4>{lesson.subject}</h4>
      <div className={styles["board-details"]}>
        <span>
          <UserRound size={13} /> {lesson.teacher || "Не указан"}
        </span>
        <span>
          <MapPin size={13} /> {lesson.room || "Не указана"}
        </span>
      </div>
      <div className={styles["board-meta"]}>
        <span>{recurrenceLabel(lesson)}</span>
        {lesson.subgroup > 0 && <span>{lesson.subgroup} подгр.</span>}
        {lesson.origin === "manual" && (
          <span className={styles["manual-badge"]}>ручная правка</span>
        )}
      </div>
      {lesson.base_lesson_id && (
        <button
          className={styles["restore-inline"]}
          onClick={onRestore}
          disabled={disabled}
        >
          <RotateCcw size={13} /> Вернуть с сайта
        </button>
      )}
    </article>
  );
}
