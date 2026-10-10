import { useEffect, useRef, useState, type FormEvent } from "react";
import {
  dateLabel,
  lessonTypes,
  request,
  type Patch,
  type PersonalLesson,
} from "./api";

export function LessonEditor({
  item,
  date,
  target,
  publication,
  onClose,
  onSaved,
}: {
  item: PersonalLesson;
  date: string;
  target: string;
  publication: string;
  onClose: () => void;
  onSaved: () => void;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const [scope, setScope] = useState<"day" | "semester" | "selected">("day");
  const [dates, setDates] = useState<string[]>([date]);
  const [cancelled, setCancelled] = useState(item.cancelled);
  const [form, setForm] = useState(item.lesson);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const dirty =
    JSON.stringify(form) !== JSON.stringify(item.lesson) ||
    cancelled !== item.cancelled ||
    scope !== "day";
  const close = () => {
    if (
      !busy &&
      (!dirty || window.confirm("Закрыть без сохранения изменений?"))
    )
      onClose();
  };
  useEffect(() => {
    dialog.current?.showModal();
  }, []);
  useEffect(() => {
    const before = (event: BeforeUnloadEvent) => {
      if (dirty) event.preventDefault();
    };
    window.addEventListener("beforeunload", before);
    return () => window.removeEventListener("beforeunload", before);
  }, [dirty]);
  async function save(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError("");
    const patch: Patch = {};
    if (form.Subject !== item.original.Subject) patch.subject = form.Subject;
    if (form.Teacher !== item.original.Teacher) patch.teacher = form.Teacher;
    if (form.Room !== item.original.Room) patch.room = form.Room;
    if (form.Type !== item.original.Type) patch.type = form.Type;
    if (
      form.TimeStart !== item.original.TimeStart ||
      form.TimeEnd !== item.original.TimeEnd
    ) {
      patch.time_start = form.TimeStart;
      patch.time_end = form.TimeEnd;
    }
    const existing = item.changes.find(
      (change) =>
        change.scope === scope && change.valid_from.slice(0, 10) === date,
    );
    try {
      await request("changes", "POST", {
        id: existing?.id || "",
        version: existing?.version || 0,
        target_id: target,
        publication,
        dates: scope === "selected" ? dates : [],
        lesson_id: item.original.personal_key || item.original.ID,
        date,
        scope,
        cancelled,
        patch,
      });
      onSaved();
    } catch (caught) {
      setError(
        caught instanceof Error ? caught.message : "Не удалось сохранить",
      );
    } finally {
      setBusy(false);
    }
  }
  return (
    <dialog
      className="personal-dialog"
      ref={dialog}
      onCancel={(event) => {
        event.preventDefault();
        close();
      }}
      aria-labelledby="personal-edit-title"
    >
      <form onSubmit={(event) => void save(event)}>
        <header>
          <div>
            <small>{dateLabel(date)} · только для вас</small>
            <h2 id="personal-edit-title">Изменить занятие</h2>
          </div>
          <button
            className="button button-ghost"
            type="button"
            onClick={close}
            disabled={busy}
            aria-label="Закрыть"
          >
            ×
          </button>
        </header>
        <fieldset disabled={busy}>
          <label>
            Предмет
            <input
              required
              maxLength={300}
              value={form.Subject}
              onChange={(e) => setForm({ ...form, Subject: e.target.value })}
            />
          </label>
          <label>
            Преподаватель
            <input
              maxLength={300}
              value={form.Teacher}
              onChange={(e) => setForm({ ...form, Teacher: e.target.value })}
            />
          </label>
          <div className="personal-form-row">
            <label>
              Аудитория
              <input
                maxLength={300}
                value={form.Room}
                onChange={(e) => setForm({ ...form, Room: e.target.value })}
              />
            </label>
            <label>
              Тип
              <select
                value={form.Type}
                onChange={(e) => setForm({ ...form, Type: e.target.value })}
              >
                {Object.entries(lessonTypes).map(([value, label]) => (
                  <option key={value} value={value}>
                    {label}
                  </option>
                ))}
              </select>
            </label>
          </div>
          <div className="personal-form-row">
            <label>
              Начало
              <input
                type="time"
                required
                value={form.TimeStart}
                onChange={(e) =>
                  setForm({ ...form, TimeStart: e.target.value })
                }
              />
            </label>
            <label>
              Окончание
              <input
                type="time"
                required
                value={form.TimeEnd}
                onChange={(e) => setForm({ ...form, TimeEnd: e.target.value })}
              />
            </label>
          </div>
          <label className="personal-check">
            <input
              type="checkbox"
              checked={cancelled}
              onChange={(e) => setCancelled(e.target.checked)}
            />
            Отменить занятие для меня
          </label>
          <label>
            Применить
            <select
              aria-label="Применить"
              value={scope}
              onChange={(e) =>
                setScope(e.target.value as "day" | "semester" | "selected")
              }
            >
              <option value="day">Только {dateLabel(date)}</option>
              {item.can_repeat && (
                <>
                  <option value="semester">
                    Все подтверждённые повторы с этой даты
                  </option>
                  <option value="selected">Выбрать даты повторов</option>
                </>
              )}
            </select>
          </label>
          {scope === "selected" && (
            <fieldset className="personal-repeat-dates">
              <legend>Даты повторов</legend>
              {(item.repeats || []).map((repeat) => (
                <label className="personal-check" key={repeat.date}>
                  <input
                    type="checkbox"
                    checked={dates.includes(repeat.date)}
                    onChange={(e) =>
                      setDates(
                        e.target.checked
                          ? [...dates, repeat.date]
                          : dates.filter((value) => value !== repeat.date),
                      )
                    }
                  />
                  {dateLabel(repeat.date)}
                </label>
              ))}
            </fieldset>
          )}
          <p className="personal-note">
            {scope === "selected"
              ? `Выбрано дат: ${dates.length}.`
              : scope === "semester"
                ? `Подтверждено повторов: ${item.repeats?.length || 0}. До ${dateLabel(item.repeats?.at(-1)?.date || item.semester_end)}. Правки отдельных дат имеют приоритет.`
                : "Другие даты останутся без изменений."}{" "}
            Правки учитываются в боте, экспорте, ежедневной отправке и
            напоминаниях.
          </p>
          <details>
            <summary>По данным вуза</summary>
            <p>
              {item.original.Subject} · {item.original.TimeStart}–
              {item.original.TimeEnd}
              <br />
              {item.original.Teacher || "Преподаватель не указан"} ·{" "}
              {item.original.Room || "Аудитория не указана"}
            </p>
          </details>
        </fieldset>
        {error && (
          <p role="alert" className="personal-error">
            {error}
          </p>
        )}
        <footer>
          <button
            className="button button-ghost"
            type="button"
            onClick={close}
            disabled={busy}
          >
            Отмена
          </button>
          <button
            className="button button-primary"
            type="submit"
            disabled={busy || (scope === "selected" && !dates.length)}
          >
            {busy ? "Сохранение…" : "Сохранить"}
          </button>
        </footer>
      </form>
    </dialog>
  );
}
