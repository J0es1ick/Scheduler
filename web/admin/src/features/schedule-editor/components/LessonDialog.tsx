import { type FormEvent, useEffect, useMemo, useState } from "react";
import { Check, X } from "lucide-react";
import { api } from "../../../api";
import { DialogPortal } from "../../../components";
import type { EditorLesson, SemesterOption } from "../../../types";
import {
  days,
  formFromLesson,
  lessonTypes,
  type LessonForm,
  weekLabels,
} from "../model";

export function LessonDialog({
  lesson,
  conflict,
  onRebase,
  day,
  semesters,
  busy,
  onClose,
  onSave,
}: {
  lesson: EditorLesson | null;
  conflict?: EditorLesson | null;
  onRebase: () => void;
  day: number;
  semesters: SemesterOption[];
  busy: boolean;
  onClose: () => void;
  onSave: (form: LessonForm) => Promise<void>;
}) {
  const [form, setForm] = useState<LessonForm>(() =>
    formFromLesson(lesson, day, semesters),
  );
  const [preview, setPreview] = useState<string[] | null>(null);
  const [previewError, setPreviewError] = useState("");
  const [previewBusy, setPreviewBusy] = useState(false);
  const [previewFrom, setPreviewFrom] = useState(() =>
    (
      lesson?.valid_from ??
      semesters[0]?.start_date ??
      new Date().toISOString()
    ).slice(0, 10),
  );
  const [reviewing, setReviewing] = useState(false);
  const initial = useMemo(
    () => JSON.stringify(formFromLesson(lesson, day, semesters)),
    [lesson, day, semesters],
  );
  const dirty = JSON.stringify(form) !== initial;

  useEffect(() => {
    const guard = (event: BeforeUnloadEvent) => {
      if (!dirty) return;
      event.preventDefault();
    };
    if (dirty) window.Telegram?.WebApp?.enableClosingConfirmation?.();
    window.addEventListener("beforeunload", guard);
    return () => {
      window.removeEventListener("beforeunload", guard);
      window.Telegram?.WebApp?.disableClosingConfirmation?.();
    };
  }, [dirty]);

  function patch<K extends keyof LessonForm>(key: K, value: LessonForm[K]) {
    setPreview(null);
    setForm((current) => ({ ...current, [key]: value }));
  }

  const semester = semesters.find((item) => item.id === form.semester_id);
  const customCycle =
    form.week_type === "every" && !!form.recurrence?.cycle_length;
  function repetition(value: string) {
    const anchor =
      (semester?.start_date ?? new Date().toISOString()).slice(0, 10) +
      "T00:00:00Z";
    setPreview(null);
    setForm((current) => ({
      ...current,
      week_type:
        value === "cycle" ? "every" : (value as EditorLesson["week_type"]),
      recurrence:
        value === "cycle"
          ? { cycle_length: 3, cycle_weeks: [1], anchor_date: anchor }
          : value === "odd" || value === "even"
            ? {
                cycle_length: 2,
                cycle_weeks: [value === "odd" ? 1 : 2],
                anchor_date: anchor,
              }
            : {},
    }));
  }
  async function calculateDates() {
    setPreviewBusy(true);
    setPreviewError("");
    setPreview(null);
    try {
      setPreview(
        (
          await api.previewEditorLesson(
            {
              ...form,
              group_id: lesson?.group_id ?? "",
              subject: form.subject || "Предпросмотр",
            },
            previewFrom,
            112,
            lesson?.id,
          )
        ).dates,
      );
    } catch (error) {
      setPreviewError(
        error instanceof Error ? error.message : "Не удалось рассчитать даты",
      );
    } finally {
      setPreviewBusy(false);
    }
  }
  function close() {
    if (dirty && !window.confirm("Закрыть форму без сохранения?")) return;
    onClose();
  }

  function submit(event: FormEvent) {
    event.preventDefault();
    setReviewing(true);
  }

  return (
    <DialogPortal>
      <div className="dialog-backdrop" role="presentation">
        <section
          className="lesson-dialog"
          role="dialog"
          aria-modal="true"
          aria-labelledby="lesson-dialog-title"
        >
          <header>
            <div>
              <span className="eyebrow">
                {lesson ? "Редактирование" : "Новое занятие"}
              </span>
              <h2 id="lesson-dialog-title">
                {lesson ? lesson.subject : days[form.day_of_week - 1]}
              </h2>
              {lesson?.base_lesson_id && (
                <p>Правка сохранится поверх версии с сайта.</p>
              )}
            </div>
            <button
              className="dialog-close"
              data-dialog-dismiss
              onClick={close}
              aria-label="Закрыть"
            >
              <X size={18} />
            </button>
          </header>

          {conflict !== undefined && (
            <div className="conflict-comparison" role="alert">
              <strong>Запись изменилась на сервере. Ваш ввод сохранён.</strong>
              {conflict ? (
                <>
                  <table>
                    <thead>
                      <tr>
                        <th>Поле</th>
                        <th>Ваш вариант</th>
                        <th>На сервере</th>
                      </tr>
                    </thead>
                    <tbody>
                      {(
                        [
                          "subject",
                          "teacher",
                          "room",
                          "time_start",
                          "time_end",
                          "week_type",
                          "recurrence",
                        ] as const
                      ).map((key) => (
                        <tr key={key}>
                          <th>
                            {
                              {
                                subject: "Предмет",
                                teacher: "Преподаватель",
                                room: "Аудитория",
                                time_start: "Начало",
                                time_end: "Конец",
                                week_type: "Повторение",
                                recurrence: "Цикл",
                              }[key]
                            }
                          </th>
                          <td>
                            {typeof form[key] === "object"
                              ? JSON.stringify(form[key])
                              : form[key]}
                          </td>
                          <td>
                            {typeof conflict[key] === "object"
                              ? JSON.stringify(conflict[key])
                              : conflict[key]}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                  <button
                    type="button"
                    className="button button-ghost"
                    onClick={() => {
                      onRebase();
                      setReviewing(false);
                    }}
                  >
                    Сравнение выполнено — продолжить с моим вводом
                  </button>
                </>
              ) : (
                <p>Занятие удалено. Закройте редактор и обновите расписание.</p>
              )}
            </div>
          )}
          {reviewing ? (
            <ManualReview
              form={form}
              busy={busy || conflict !== undefined}
              onBack={() => setReviewing(false)}
              onConfirm={() => onSave(form)}
            />
          ) : (
            <form onSubmit={submit}>
              <div className="form-grid">
                <label className="field field-wide">
                  <span>Предмет</span>
                  <input
                    required
                    maxLength={300}
                    value={form.subject}
                    onChange={(event) => patch("subject", event.target.value)}
                    placeholder="Название дисциплины"
                  />
                </label>
                <label className="field">
                  <span>Тип занятия</span>
                  <select
                    value={form.type}
                    onChange={(event) => patch("type", event.target.value)}
                  >
                    {lessonTypes.map(([value, label]) => (
                      <option key={value} value={value}>
                        {label}
                      </option>
                    ))}
                  </select>
                </label>
                <label className="field">
                  <span>Семестр</span>
                  <select
                    value={form.semester_id}
                    onChange={(event) =>
                      patch("semester_id", event.target.value)
                    }
                    required
                  >
                    {semesters.map((semester) => (
                      <option key={semester.id} value={semester.id}>
                        {semester.name}
                      </option>
                    ))}
                  </select>
                </label>
                <label className="field">
                  <span>Повторение</span>
                  <select
                    value={customCycle ? "cycle" : form.week_type}
                    onChange={(event) => repetition(event.target.value)}
                  >
                    {Object.entries(weekLabels).map(([value, label]) => (
                      <option key={value} value={value}>
                        {label}
                      </option>
                    ))}
                    <option value="cycle">Произвольный цикл</option>
                  </select>
                </label>
                {form.week_type !== "date" && (
                  <div className="field field-wide recurrence-summary">
                    <span>
                      Начало отсчёта недель:{" "}
                      {(
                        form.recurrence?.anchor_date ??
                        semester?.start_date ??
                        "—"
                      ).slice(0, 10)}
                    </span>
                    {!form.recurrence?.cycle_length &&
                      lesson &&
                      form.week_type !== "every" && (
                        <small>
                          Историческое правило: первая дата{" "}
                          {form.valid_from || "по семестру"}. Оно сохранится при
                          изменении реквизитов.
                        </small>
                      )}
                    {customCycle && (
                      <>
                        <label>
                          Длина цикла (недель)
                          <input
                            type="number"
                            min={2}
                            max={16}
                            required
                            value={form.recurrence?.cycle_length ?? 3}
                            onChange={(event) =>
                              patch("recurrence", {
                                ...form.recurrence,
                                cycle_length: Number(event.target.value),
                                cycle_weeks: (
                                  form.recurrence?.cycle_weeks ?? [1]
                                ).filter(
                                  (week) => week <= Number(event.target.value),
                                ),
                              })
                            }
                          />
                        </label>
                        <fieldset>
                          <legend>Недели с занятиями</legend>
                          {Array.from(
                            { length: form.recurrence?.cycle_length ?? 3 },
                            (_, index) => index + 1,
                          ).map((week) => (
                            <label key={week}>
                              <input
                                type="checkbox"
                                checked={
                                  form.recurrence?.cycle_weeks?.includes(
                                    week,
                                  ) ?? false
                                }
                                onChange={(event) =>
                                  patch("recurrence", {
                                    ...form.recurrence,
                                    cycle_weeks: event.target.checked
                                      ? [
                                          ...(form.recurrence?.cycle_weeks ??
                                            []),
                                          week,
                                        ].sort((a, b) => a - b)
                                      : form.recurrence?.cycle_weeks?.filter(
                                          (value) => value !== week,
                                        ),
                                  })
                                }
                              />
                              {week}
                            </label>
                          ))}
                        </fieldset>
                      </>
                    )}
                    {!!form.recurrence?.cycle_length && (
                      <label>
                        Начало отсчёта
                        <input
                          type="date"
                          required
                          value={
                            form.recurrence.anchor_date?.slice(0, 10) ?? ""
                          }
                          onChange={(event) =>
                            patch("recurrence", {
                              ...form.recurrence,
                              anchor_date: event.target.value + "T00:00:00Z",
                            })
                          }
                        />
                      </label>
                    )}
                  </div>
                )}
                {form.week_type === "date" ? (
                  <label className="field">
                    <span>Дата занятия</span>
                    <input
                      type="date"
                      required
                      value={form.special_date}
                      onChange={(event) =>
                        patch("special_date", event.target.value)
                      }
                    />
                  </label>
                ) : (
                  <label className="field">
                    <span>День недели</span>
                    <select
                      value={form.day_of_week}
                      onChange={(event) =>
                        patch("day_of_week", Number(event.target.value))
                      }
                    >
                      {days.map((name, index) => (
                        <option key={name} value={index + 1}>
                          {name}
                        </option>
                      ))}
                    </select>
                  </label>
                )}
                <label className="field">
                  <span>Начало</span>
                  <input
                    type="time"
                    required
                    value={form.time_start}
                    onChange={(event) =>
                      patch("time_start", event.target.value)
                    }
                  />
                </label>
                <label className="field">
                  <span>Окончание</span>
                  <input
                    type="time"
                    required
                    value={form.time_end}
                    onChange={(event) => patch("time_end", event.target.value)}
                  />
                </label>
                {form.week_type !== "date" && (
                  <>
                    <label className="field">
                      <span>Действует с</span>
                      <input
                        type="date"
                        value={form.valid_from}
                        onChange={(event) =>
                          patch("valid_from", event.target.value)
                        }
                      />
                    </label>
                    <label className="field">
                      <span>Действует до</span>
                      <input
                        type="date"
                        value={form.valid_to}
                        onChange={(event) =>
                          patch("valid_to", event.target.value)
                        }
                      />
                    </label>
                  </>
                )}
                <label className="field field-wide">
                  <span>Преподаватель</span>
                  <input
                    maxLength={200}
                    value={form.teacher}
                    onChange={(event) => patch("teacher", event.target.value)}
                    placeholder="Фамилия и инициалы"
                  />
                </label>
                <label className="field">
                  <span>Аудитория</span>
                  <input
                    maxLength={100}
                    value={form.room}
                    onChange={(event) => patch("room", event.target.value)}
                    placeholder="Например, А-305"
                  />
                </label>
                <label className="field">
                  <span>Подгруппа</span>
                  <select
                    value={form.subgroup}
                    onChange={(event) =>
                      patch("subgroup", Number(event.target.value))
                    }
                  >
                    <option value={0}>Вся группа</option>
                    <option value={1}>Подгруппа 1</option>
                    <option value={2}>Подгруппа 2</option>
                    <option value={3}>Подгруппа 3</option>
                  </select>
                </label>
              </div>
              <div className="calendar-preview">
                <label className="field">
                  <span>Просмотр дат за 112 дней, начиная с</span>
                  <input
                    type="date"
                    value={previewFrom}
                    onChange={(event) => {
                      setPreviewFrom(event.target.value);
                      setPreview(null);
                    }}
                  />
                </label>
                <button
                  type="button"
                  className="button button-ghost"
                  disabled={previewBusy}
                  onClick={() => void calculateDates()}
                >
                  {previewBusy ? "Рассчитываем…" : "Рассчитать даты занятий"}
                </button>
                {previewError && <p role="alert">{previewError}</p>}
                {preview && (
                  <p role="status">
                    {preview.length
                      ? preview.join(" · ")
                      : "В выбранном диапазоне занятий нет"}
                  </p>
                )}
              </div>
              <footer>
                <span>
                  {dirty ? "Есть несохранённые изменения" : "Изменений нет"}
                </span>
                <div className="dialog-actions">
                  <button
                    type="button"
                    className="button button-ghost"
                    data-dialog-dismiss
                    onClick={close}
                  >
                    Отмена
                  </button>
                  <button
                    className="button button-primary"
                    disabled={busy || !form.subject.trim()}
                  >
                    <Check size={16} /> Проверить изменения
                  </button>
                </div>
              </footer>
            </form>
          )}
        </section>
      </div>
    </DialogPortal>
  );
}

function ManualReview({
  form,
  busy,
  onBack,
  onConfirm,
}: {
  form: LessonForm;
  busy: boolean;
  onBack: () => void;
  onConfirm: () => Promise<void>;
}) {
  return (
    <div className="manual-review">
      <div className="manual-review-notice">
        <Check size={18} />
        <div>
          <strong>Подтвердите применение ручной версии</strong>
          <p>
            После подтверждения бот начнёт использовать эту запись вместо данных
            источника.
          </p>
        </div>
      </div>
      <dl>
        <div>
          <dt>Повторение</dt>
          <dd>
            {form.recurrence?.cycle_length
              ? `Цикл ${form.recurrence.cycle_length}: недели ${form.recurrence.cycle_weeks?.join(", ")}; отсчёт ${form.recurrence.anchor_date?.slice(0, 10)}`
              : weekLabels[form.week_type]}
          </dd>
        </div>
        <div>
          <dt>Предмет</dt>
          <dd>{form.subject}</dd>
        </div>
        <div>
          <dt>Когда</dt>
          <dd>
            {form.week_type === "date"
              ? form.special_date
              : days[form.day_of_week - 1]}{" "}
            · {form.time_start}–{form.time_end}
          </dd>
        </div>
        <div>
          <dt>Преподаватель</dt>
          <dd>{form.teacher || "Не указан"}</dd>
        </div>
        <div>
          <dt>Аудитория</dt>
          <dd>{form.room || "Не указана"}</dd>
        </div>
      </dl>
      <div className="dialog-actions">
        <button
          type="button"
          className="button button-ghost"
          disabled={busy}
          onClick={onBack}
        >
          Вернуться к форме
        </button>
        <button
          type="button"
          className="button button-primary"
          disabled={busy}
          onClick={() => void onConfirm()}
        >
          <Check size={16} /> {busy ? "Применяем…" : "Подтвердить и применить"}
        </button>
      </div>
    </div>
  );
}
