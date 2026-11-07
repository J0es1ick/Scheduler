import { classNames } from "../../../../shared/classNames";
import styles from "./PersonalCalendar.module.css";
import buttonStyles from "../../ui/button/Button.module.css";
import personalUIStyles from "../personal-shared/PersonalUI.module.css";
import stateStyles from "../../ui/state/State.module.css";
import { useEffect, useState } from "react";
import { NotebookPen, Share2, Check, ChevronLeft, ChevronRight, Pencil, MapPin, RefreshCw, Repeat2, Undo2, UserRound } from "lucide-react";

import { LoadingBlock } from "../../ui";
import { dateLabel, lessonTypes, request, shiftDate, today, type Change, type PersonalLesson, type Profile, type Schedule, type Target } from "../personal-shared/api";
import { LessonEditor } from "./lesson-editor/LessonEditor";
import { NowNext } from "../personal-agenda/NowNext";

import { NoteEditor, type Note } from "../personal-notes/Notes";

export function PersonalCalendar({ profile }: { profile: Profile }) {
  const [targetID, setTargetID] = useState(
    profile.targets.find((item) => item.id === new URLSearchParams(window.location.search).get("target"))?.id || profile.targets.find((t) => t.primary)?.id || profile.targets[0]?.id || "",
  );
  const target = profile.targets.find((t) => t.id === targetID);
  return !target ? (
    <section className={styles["personal-empty"]}>
      <h2>Сначала выберите расписание</h2>
      <p>
        Откройте бота и завершите настройку через /start. Затем вернитесь в
        приложение.
      </p>
    </section>
  ) : (
    <>
      <div className={styles["personal-context"]}>
        <label className={styles["personal-target"]}>
          Расписание
          <select value={targetID} onChange={(e) => setTargetID(e.target.value)}>
            {profile.targets.map((t) => (
              <option key={t.id} value={t.id}>
                {t.name}
                {t.primary ? " · основное" : ""}
              </option>
            ))}
          </select>
        </label>
        <div className={styles["personal-context-meta"]}>
          <span>{target.role === "teacher" ? "Преподаватель" : "Студент"}</span>
          <strong>{target.university_name}</strong>
          <small>Время вуза · {target.timezone}</small>
        </div>
      </div>
      {target.share_url && <a className={classNames(buttonStyles["button"], buttonStyles["button-ghost"], styles["personal-share"])} href={`https://t.me/share/url?${new URLSearchParams({ url: target.share_url, text: `${target.university_name} · ${target.name}` })}`} target="_blank" rel="noreferrer"><Share2 size={17} />Поделиться группой</a>}
      <Calendar key={targetID} target={target} />
    </>
  );
}

function Calendar({ target }: { target: Target }) {
  const [date, setDate] = useState(() => { const value = new URLSearchParams(window.location.search).get("date") || ""; const parsed = new Date(`${value}T12:00:00Z`); return !Number.isNaN(parsed.getTime()) && parsed.toISOString().slice(0, 10) === value ? value : today(target.timezone); });
  const [noteEditing, setNoteEditing] = useState<{ date: string; item: PersonalLesson; note?: Note }>();
  const [notes, setNotes] = useState<Note[]>([]);
  const [notesError, setNotesError] = useState(false);
  const [revision, setRevision] = useState(0);
  const key = `${date}:${revision}`;
  useEffect(() => { let active = true; void request<Note[]>("notes").then((items) => { if (active) { setNotes(items); setNotesError(false); } }).catch(() => { if (active) setNotesError(true); }); return () => { active = false; }; }, [revision]);
  const [loaded, setLoaded] = useState<{
    key: string;
    date: string;
    data?: Schedule;
    error?: string;
  }>();
  const [editing, setEditing] = useState<{
    date: string;
    item: PersonalLesson;
  }>();
  const [reviewing, setReviewing] = useState(false);
  const [removing, setRemoving] = useState("");
  const [actionError, setActionError] = useState("");
  const [notice, setNotice] = useState("");
  useEffect(() => {
    if (!notice) return;
    const timeout = window.setTimeout(() => setNotice(""), 6000);
    return () => window.clearTimeout(timeout);
  }, [notice]);
  useEffect(() => {
    let active = true;
    void request<Schedule>(
      `schedule?${new URLSearchParams({ target: target.id, from: date, to: shiftDate(date, 6) })}`,
    )
      .then((data) => {
        if (active) setLoaded({ key, date, data });
      })
      .catch((error) => {
        if (active)
          setLoaded((previous) => ({
            key,
            date,
            data: previous?.date === date ? previous.data : undefined,
            error:
              error instanceof Error ? error.message : "Расписание недоступно",
          }));
      });
    return () => {
      active = false;
    };
  }, [target.id, date, key]);
  const current = loaded?.date === date ? loaded : undefined;
  const loading = loaded?.key !== key;
  const localToday = today(target.timezone);
  async function remove(change: Change) {
    if (
      !window.confirm(
        `Удалить личную правку ${change.scope === "day" ? "на " + dateLabel(change.valid_from) : "с " + dateLabel(change.valid_from) + " до " + dateLabel(change.valid_to)}? Остальные правки сохранятся.`,
      )
    )
      return;
    setRemoving(change.id);
    setActionError("");
    try {
      await request(
        `changes/${encodeURIComponent(change.id)}?version=${change.version}`,
        "DELETE",
      );
      setRevision((n) => n + 1);
      setNotice("Правка удалена");
    } catch (error) {
      setActionError(
        error instanceof Error ? error.message : "Не удалось удалить",
      );
    } finally {
      setRemoving("");
    }
  }
  async function resolveReview(action: "keep" | "discard") {
    const schedule = current?.data;
    if (!schedule?.review) return;
    if (
      action === "discard" &&
      !window.confirm(
        "Сбросить правки, затронутые обновлением? После этого вы сможете настроить занятия заново.",
      )
    )
      return;
    setReviewing(true);
    setActionError("");
    try {
      await request("review", "POST", {
        target_id: target.id,
        publication: schedule.publication,
        action,
        versions: Object.fromEntries(
          schedule.review.items.map((item) => [item.id, item.version]),
        ),
      });
      setRevision((n) => n + 1);
      setNotice(
        action === "keep"
          ? "Правки перенесены на совпавшие слоты. Несовпавшие правки сброшены."
          : "Правки сброшены. Можно настроить занятия заново.",
      );
    } catch (error) {
      setActionError(
        error instanceof Error
          ? error.message
          : "Не удалось согласовать правки",
      );
    } finally {
      setReviewing(false);
    }
  }
  return (
    <>
      <NowNext target={target} revision={revision} />
      {notesError && <p role="status" className={personalUIStyles["personal-note"]}>Не удалось загрузить заметки к занятиям. <button className={buttonStyles["text-button"]} onClick={() => setRevision((value) => value + 1)}>Повторить</button></p>}
      <div className={styles["personal-calendar-controls"]}>
        <nav className={styles["personal-date-nav"]} aria-label="Период расписания">
          <div className={styles["personal-period-picker"]}>
            <button
              className={classNames(buttonStyles["button"], buttonStyles["button-ghost"])}
              aria-label="Предыдущие 7 дней"
              onClick={() => setDate(shiftDate(date, -7))}
            >
              <ChevronLeft size={18} />
            </button>
            <label>
              Начиная с
              <input
                aria-label="Начиная с"
                type="date"
                value={date}
                onChange={(e) => {
                  if (/^\d{4}-\d{2}-\d{2}$/.test(e.target.value))
                    setDate(e.target.value);
                }}
              />
            </label>
            <button
              className={classNames(buttonStyles["button"], buttonStyles["button-ghost"])}
              aria-label="Следующие 7 дней"
              onClick={() => setDate(shiftDate(date, 7))}
            >
              <ChevronRight size={18} />
            </button>
          </div>
          <div className={styles["personal-period-actions"]}>
            <button
              className={classNames(buttonStyles["button"], buttonStyles["button-ghost"])}
              onClick={() => setDate(today(target.timezone))}
            >
              Сегодня
            </button>
            <button
              className={classNames(buttonStyles["button"], buttonStyles["button-ghost"])}
              aria-label="Обновить расписание"
              disabled={loading}
              onClick={() => setRevision((n) => n + 1)}
            >
              <RefreshCw size={17} className={loading ? stateStyles["is-loading"] : undefined} aria-hidden="true" />
              <span>Обновить</span>
            </button>
          </div>
        </nav>
        {current?.data && current.data.days.length > 1 && (
          <nav className={styles["personal-week-strip"]} aria-label="Дни недели">
            {current.data.days.map((day) => (
              <button
                key={day.date}
                aria-label={`Перейти к ${dateLabel(day.date)}`}
                aria-current={day.date === localToday ? "date" : undefined}
                onClick={(event) => {
                  const heading = document.getElementById(`personal-day-${day.date}`);
                  heading?.scrollIntoView({ behavior: window.matchMedia("(prefers-reduced-motion: reduce)").matches ? "instant" : "smooth", block: "start" });
                  if (event.detail === 0) heading?.focus({ preventScroll: true });
                }}
              >
                <span>{new Date(`${day.date}T12:00:00`).toLocaleDateString("ru-RU", { weekday: "short" })}</span>
                <strong>{Number(day.date.slice(-2))}</strong>
                <i className={day.lessons.some((lesson) => !lesson.cancelled) ? stateStyles["has-lessons"] : undefined} aria-hidden="true" />
              </button>
            ))}
          </nav>
        )}
      </div>
      <div role="status" aria-live="polite" className={styles["personal-notice"]} data-visible={!!notice}>
        {notice && <><Check size={18} aria-hidden="true" /><span>{notice}</span></>}
      </div>
      {actionError && (
        <p role="alert" className={personalUIStyles["personal-error"]}>
          {actionError}
        </p>
      )}
      {current?.error && <p role="alert" className={personalUIStyles["personal-error"]}>{current.error} <button className={buttonStyles["text-button"]} onClick={() => setRevision((n) => n + 1)}>Повторить загрузку</button></p>}
      {!current?.data && loading && <div className={styles["personal-schedule-loading"]}><LoadingBlock rows={3} /></div>}
      {current?.data && (
        <>
          <div className={styles["personal-patterns"]}>
            {current.data?.patterns?.map((pattern) => (
              <details
                className={styles["personal-pattern"]}
                key={`${pattern.group_id}:${pattern.semester_id}`}
              >
                <summary><Repeat2 size={16} aria-hidden="true" /><strong>
                  {target.role === "teacher" ? `${pattern.group_name} · ` : ""}
                  {{
                    weekly: "Недельное",
                    biweekly: "Двухнедельное",
                    individual: "Индивидуальное",
                  }[pattern.kind] || "Индивидуальное"}{" "}
                  расписание
                </strong><ChevronRight size={16} aria-hidden="true" /></summary>
                <p>
                  {pattern.from && pattern.to
                    ? `Повторы подтверждены с ${dateLabel(pattern.from)} по ${dateLabel(pattern.to)}`
                    : "Длительный повтор недель не подтверждён"}
                  {pattern.exceptions
                    ? ` · недель с отличиями: ${pattern.exceptions}`
                    : ""}
                </p>
              </details>
            ))}
          </div>
          {current.data?.review && (
            <section
              className={styles["personal-review"]}
              aria-label="Согласование правок"
            >
              <h2>Вуз обновил расписание</h2>
              <p>
                Ваши правки приостановлены. Можно перенести их на занятия в тех
                же слотах: группа, день, время и подгруппа должны совпасть.
                Предмет и преподаватель в новом расписании могут отличаться.
              </p>
              <p>
                Совпало занятий: {current.data.review.kept}. Нет однозначного
                совпадения: {current.data.review.dropped}.
              </p>
              <ul>
                {current.data.review.items.map((item) => (
                  <li key={item.id}>
                    {item.subject || "Занятие"} · перенести: {item.kept},
                    сбросить: {item.dropped}
                  </li>
                ))}
              </ul>
              <div className={styles["personal-review-actions"]}>
                <button
                  className={classNames(buttonStyles["button"], buttonStyles["button-primary"])}
                  disabled={reviewing || !!removing}
                  onClick={() => void resolveReview("keep")}
                >
                  Сохранить в совпавших слотах
                </button>
                <button
                  className={classNames(buttonStyles["button"], buttonStyles["button-ghost"])}
                  disabled={reviewing || !!removing}
                  onClick={() => void resolveReview("discard")}
                >
                  Сбросить затронутые правки
                </button>
              </div>
            </section>
          )}
          <div className={styles["personal-days"]} aria-busy={loading} key={date}>
            {current.data?.days.map((day) => (
              <section key={day.date} className={styles["personal-day"]}>
                <h2 id={`personal-day-${day.date}`} tabIndex={-1}>
                  <span>
                    {new Date(day.date + "T12:00:00").toLocaleDateString(
                      "ru-RU",
                      { weekday: "long" },
                    )}
                  </span>
                  {dateLabel(day.date)}
                  {day.date === localToday && <small className={styles["personal-today"]}>Сегодня</small>}
                </h2>
                <div className={styles["personal-day-lessons"]}>
                  {!day.lessons.length ? (
                    <p className={personalUIStyles["personal-empty-day"]}>{current.data?.freshness && !current.data.freshness.LastSuccess ? "Расписание ещё не опубликовано" : "Занятий нет"}</p>
                  ) : (
                    [...day.lessons]
                      .sort((a, b) =>
                        a.lesson.TimeStart.localeCompare(b.lesson.TimeStart),
                      )
                      .map((item) => (
                        <article
                          className={classNames(styles["personal-lesson"], item.cancelled ? stateStyles["is-cancelled"] : undefined)}
                          key={item.original.ID}
                        >
                          <div className={styles["personal-time"]}>
                            <strong>{item.lesson.TimeStart}</strong>
                            <span>{item.lesson.TimeEnd}</span>
                          </div>
                          <div className={styles["personal-lesson-copy"]}>
                            <small>
                              {lessonTypes[item.lesson.Type] || "Занятие"}
                              {item.group_name && target.role === "teacher"
                                ? ` · ${item.group_name}`
                                : ""}
                              {item.lesson.Subgroup
                                ? ` · подгруппа ${item.lesson.Subgroup}`
                                : ""}
                            </small>
                            <h3>{item.lesson.Subject}</h3>
                            <p><UserRound size={14} aria-hidden="true" />
                              {item.lesson.Teacher || "Преподаватель не указан"}
                            </p>
                            <p><MapPin size={14} aria-hidden="true" />
                              {item.lesson.Room
                                ? `Аудитория ${item.lesson.Room}`
                                : "Аудитория не указана"}
                            </p>
                            {item.changes.length > 0 && (
                              <span className={personalUIStyles["personal-tag"]}>
                                {item.cancelled
                                  ? "Отменено для вас"
                                  : "Личная правка"}
                              </span>
                            )}
                          </div>
                          <div className={styles["personal-lesson-actions"]}>
                          <button className={classNames(buttonStyles["button"], buttonStyles["button-ghost"])} disabled={loading} onClick={() => setNoteEditing({ date: day.date, item })}><NotebookPen size={16} />Заметка</button>
                          <button
                            className={classNames(buttonStyles["button"], buttonStyles["button-ghost"], styles["personal-edit"])}
                            aria-label={`Изменить ${item.lesson.Subject}, ${dateLabel(day.date)}`}
                            disabled={loading || !!current.data?.review || reviewing}
                            onClick={() => setEditing({ date: day.date, item })}
                          >
                            <Pencil size={16} aria-hidden="true" />
                            <span>Изменить</span>
                          </button>
                          </div>
                          {notes.filter((note) => !note.completed && note.target_id === target.id && (note.current_date || note.lesson_date.slice(0, 10)) === day.date && (note.current_lesson_id === item.original.ID || note.lesson_id === (item.original.personal_key || item.original.ID))).map((note) => <button className={styles["personal-attached-note"]} key={note.id} onClick={() => setNoteEditing({ date: day.date, item, note })}><NotebookPen size={14} /><span>{note.body}</span>{note.due_date && <small>до {dateLabel(note.due_date)}</small>}</button>)}
                        </article>
                      ))
                  )}
                </div>
              </section>
            ))}
          </div>
          <details className={styles["personal-changes"]}>
            <summary><Undo2 size={18} aria-hidden="true" /><span>Мои правки · {current.data?.changes.length || 0}</span><ChevronRight size={18} aria-hidden="true" /></summary>
            <p className={personalUIStyles["personal-note"]}>
              Правка даты важнее правки семестра. Удалите правку, чтобы вернуть
              данные вуза или предыдущее правило. Правки снятых с публикации
              занятий хранятся здесь.
            </p>
            {current.data?.changes.map((change) => {
              const lesson = current.data?.days
                .flatMap((d) => d.lessons)
                .find(
                  (l) =>
                    (l.original.personal_key || l.original.ID) ===
                    change.lesson_id,
                );
              return (
                <article key={change.id}>
                  <div>
                    <strong>
                      {lesson?.original.Subject ||
                        change.patch.subject ||
                        "Занятие вне выбранного периода"}
                    </strong>
                    <p>
                      {dateLabel(change.valid_from)}
                      {change.scope !== "day"
                        ? ` — ${dateLabel(change.valid_to)}`
                        : ""}{" "}
                      {change.needs_review ? " · ожидает согласования" : ""}
                      {change.scope === "selected"
                        ? ` · выбранных дат: ${change.occurrences?.length || 0}`
                        : ""}
                      ·{" "}
                      {change.cancelled
                        ? "Отмена"
                        : Object.keys(change.patch)
                          .map(
                            (field) =>
                              ({
                                subject: "предмет",
                                teacher: "преподаватель",
                                room: "аудитория",
                                type: "тип",
                                time_start: "начало",
                                time_end: "окончание",
                              })[field] || field,
                          )
                          .join(", ") || "Без отмены"}
                    </p>
                  </div>
                  <button
                    className={classNames(buttonStyles["button"], buttonStyles["button-ghost"])}
                    disabled={!!removing || reviewing}
                    aria-label={`Удалить правку ${dateLabel(change.valid_from)}`}
                    onClick={() => void remove(change)}
                  >
                    <Undo2 size={16} />
                    {removing === change.id ? "Удаление…" : "Удалить"}
                  </button>
                </article>
              );
            })}
          </details>
        </>
      )}
      {noteEditing && <NoteEditor note={noteEditing.note} selection={noteEditing.note ? undefined : { target: target.id, ...noteEditing }} onClose={() => setNoteEditing(undefined)} onSaved={() => { setNoteEditing(undefined); setRevision((n) => n + 1); setNotice("Заметка сохранена"); }} />}
      {editing && (
        <LessonEditor
          key={`${editing.date}:${editing.item.original.ID}`}
          {...editing}
          target={target.id}
          publication={current?.data?.publication || ""}
          onClose={() => setEditing(undefined)}
          onSaved={() => {
            setEditing(undefined);
            setRevision((n) => n + 1);
            setNotice("Правка сохранена. Расписание в боте тоже обновлено.");
          }}
        />
      )}
    </>
  );
}
