import { useEffect, useState } from "react";
import {
  CalendarDays,
  ChevronLeft,
  ChevronRight,
  Pencil,
  Shield,
  Undo2,
} from "lucide-react";
import { useTheme } from "../app/useTheme";
import { ThemeSwitch } from "../app/ThemeSwitch";
import {
  dateLabel,
  lessonTypes,
  login,
  request,
  shiftDate,
  today,
  type Change,
  type PersonalLesson,
  type Profile,
  type Schedule,
  type Target,
} from "./api";
import { LessonEditor } from "./LessonEditor";
import "./personal.css";

export default function PersonalApp() {
  const theme = useTheme();
  const [profile, setProfile] = useState<Profile | null>(null);
  const [error, setError] = useState("");
  const [attempt, setAttempt] = useState(0);
  const [chosen, setChosen] = useState(false);
  useEffect(() => {
    let active = true;
    window.Telegram?.WebApp?.ready?.();
    window.Telegram?.WebApp?.expand?.();
    void login()
      .then((result) => {
        if (active) {
          setProfile(result);
          setError("");
        }
      })
      .catch((caught) => {
        if (active)
          setError(
            caught instanceof Error ? caught.message : "Не удалось войти",
          );
      });
    return () => {
      active = false;
    };
  }, [attempt]);
  return (
    <main className="personal-app">
      <header className="personal-header">
        <a href="/app" className="personal-brand">
          <CalendarDays size={23} />
          <span>Моё расписание</span>
        </a>
        <ThemeSwitch {...theme} />
      </header>
      {!profile ? (
        <section className="personal-welcome">
          <h1>Расписание под ваш день</h1>
          <p>{error || "Открываем ваш профиль…"}</p>
          {error && (
            <button
              className="button button-primary"
              onClick={() => setAttempt((value) => value + 1)}
            >
              Повторить вход
            </button>
          )}
        </section>
      ) : profile.is_admin && !chosen ? (
        <section className="personal-welcome">
          <p className="personal-eyebrow">{profile.name || "Здравствуйте"}</p>
          <h1>Куда перейдём?</h1>
          <div className="personal-destinations">
            <button onClick={() => setChosen(true)}>
              <CalendarDays />
              <strong>Личное расписание</strong>
              <span>Настроить занятия только для себя</span>
            </button>
            <a href="/#/editor">
              <Shield />
              <strong>Админка</strong>
              <span>Управлять общим расписанием и сервисом</span>
            </a>
          </div>
        </section>
      ) : (
        <>
          <div className="personal-intro">
            {profile.is_admin && (
              <button
                className="button button-ghost"
                onClick={() => setChosen(false)}
              >
                ← К выбору сервиса
              </button>
            )}
            <h1>Ваши правки</h1>
            <p>
              Измените аудиторию, преподавателя или отмените пару. Правки видны
              только вам — в приложении и в боте.
            </p>
          </div>
          <PersonalCalendar profile={profile} />
        </>
      )}
    </main>
  );
}

function PersonalCalendar({ profile }: { profile: Profile }) {
  const [targetID, setTargetID] = useState(
    profile.targets.find((t) => t.primary)?.id || profile.targets[0]?.id || "",
  );
  const target = profile.targets.find((t) => t.id === targetID);
  return !target ? (
    <section className="personal-empty">
      <h2>Сначала выберите расписание</h2>
      <p>
        Откройте бота и завершите настройку через /start. Затем вернитесь в
        приложение.
      </p>
    </section>
  ) : (
    <>
      <label className="personal-target">
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
      <Calendar key={targetID} target={target} />
    </>
  );
}

function Calendar({ target }: { target: Target }) {
  const [date, setDate] = useState(() => today(target.timezone));
  const [revision, setRevision] = useState(0);
  const key = `${date}:${revision}`;
  const [loaded, setLoaded] = useState<{
    key: string;
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
    let active = true;
    void request<Schedule>(
      `schedule?${new URLSearchParams({ target: target.id, from: date, to: shiftDate(date, 6) })}`,
    )
      .then((data) => {
        if (active) setLoaded({ key, data });
      })
      .catch((error) => {
        if (active)
          setLoaded({
            key,
            error:
              error instanceof Error ? error.message : "Расписание недоступно",
          });
      });
    return () => {
      active = false;
    };
  }, [target.id, date, key]);
  const current = loaded?.key === key ? loaded : undefined;
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
      <p className="personal-note">
        {target.university_name} · {target.timezone}
      </p>
      <nav className="personal-date-nav" aria-label="Период расписания">
        <button
          className="button button-ghost"
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
          className="button button-ghost"
          aria-label="Следующие 7 дней"
          onClick={() => setDate(shiftDate(date, 7))}
        >
          <ChevronRight size={18} />
        </button>
        <button
          className="button button-ghost"
          onClick={() => setDate(today(target.timezone))}
        >
          Сегодня
        </button>
        <button
          className="button button-ghost"
          onClick={() => setRevision((n) => n + 1)}
        >
          Обновить
        </button>
      </nav>
      <p role="status" className="personal-note">
        {notice}
      </p>
      {actionError && (
        <p role="alert" className="personal-error">
          {actionError}
        </p>
      )}
      {!current ? (
        <p role="status">Загружаем занятия…</p>
      ) : current.error ? (
        <p role="alert" className="personal-error">
          {current.error}
        </p>
      ) : (
        <>
          <div className="personal-patterns">
            {current.data?.patterns?.map((pattern) => (
              <p
                className="personal-note"
                key={`${pattern.group_id}:${pattern.semester_id}`}
              >
                <strong>
                  {target.role === "teacher" ? `${pattern.group_name} · ` : ""}
                  {{
                    weekly: "Недельное",
                    biweekly: "Двухнедельное",
                    individual: "Индивидуальное",
                  }[pattern.kind] || "Индивидуальное"}{" "}
                  расписание
                </strong>
                {pattern.from && pattern.to
                  ? ` · повторы подтверждены с ${dateLabel(pattern.from)} по ${dateLabel(pattern.to)}`
                  : " · длительный повтор недель не подтверждён"}
                {pattern.exceptions
                  ? ` · недель с отличиями: ${pattern.exceptions}`
                  : ""}
              </p>
            ))}
          </div>
          {current.data?.review && (
            <section
              className="personal-review"
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
              <div className="personal-review-actions">
                <button
                  className="button button-primary"
                  disabled={reviewing || !!removing}
                  onClick={() => void resolveReview("keep")}
                >
                  Сохранить в совпавших слотах
                </button>
                <button
                  className="button button-ghost"
                  disabled={reviewing || !!removing}
                  onClick={() => void resolveReview("discard")}
                >
                  Сбросить затронутые правки
                </button>
              </div>
            </section>
          )}
          <div className="personal-days">
            {current.data?.days.map((day) => (
              <section key={day.date} className="personal-day">
                <h2>
                  <span>
                    {new Date(day.date + "T12:00:00").toLocaleDateString(
                      "ru-RU",
                      { weekday: "long" },
                    )}
                  </span>
                  {dateLabel(day.date)}
                </h2>
                <div className="personal-day-lessons">
                  {!day.lessons.length ? (
                    <p className="personal-empty-day">Занятий нет</p>
                  ) : (
                    [...day.lessons]
                      .sort((a, b) =>
                        a.lesson.TimeStart.localeCompare(b.lesson.TimeStart),
                      )
                      .map((item) => (
                        <article
                          className={`personal-lesson${item.cancelled ? " is-cancelled" : ""}`}
                          key={item.original.ID}
                        >
                          <div className="personal-time">
                            <strong>{item.lesson.TimeStart}</strong>
                            <span>{item.lesson.TimeEnd}</span>
                          </div>
                          <div className="personal-lesson-copy">
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
                            <p>
                              {item.lesson.Teacher || "Преподаватель не указан"}
                            </p>
                            <p>
                              {item.lesson.Room
                                ? `Аудитория ${item.lesson.Room}`
                                : "Аудитория не указана"}
                            </p>
                            {item.changes.length > 0 && (
                              <span className="personal-tag">
                                {item.cancelled
                                  ? "Отменено для вас"
                                  : "Личная правка"}
                              </span>
                            )}
                          </div>
                          <button
                            className="button button-ghost personal-edit"
                            aria-label={`Изменить ${item.lesson.Subject}, ${dateLabel(day.date)}`}
                            disabled={!!current.data?.review || reviewing}
                            onClick={() => setEditing({ date: day.date, item })}
                          >
                            <Pencil size={16} />
                            <span>Изменить</span>
                          </button>
                        </article>
                      ))
                  )}
                </div>
              </section>
            ))}
          </div>
          <details className="personal-changes">
            <summary>Мои правки · {current.data?.changes.length || 0}</summary>
            <p className="personal-note">
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
                    className="button button-ghost"
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
