import { type FormEvent, useEffect, useMemo, useState } from "react";
import { Check, X } from "lucide-react";
import type { EditorLesson, SemesterOption } from "../../../types";
import { days, formFromLesson, lessonTypes, type LessonForm, weekLabels } from "../model";

export function LessonDialog({
  lesson,
  day,
  semesters,
  busy,
  onClose,
  onSave,
}: {
  lesson: EditorLesson | null;
  day: number;
  semesters: SemesterOption[];
  busy: boolean;
  onClose: () => void;
  onSave: (form: LessonForm) => Promise<void>;
}) {
  const [form, setForm] = useState<LessonForm>(() => formFromLesson(lesson, day, semesters));
  const [reviewing, setReviewing] = useState(false);
  const initial = useMemo(() => JSON.stringify(formFromLesson(lesson, day, semesters)), [lesson, day, semesters]);
  const dirty = JSON.stringify(form) !== initial;

  useEffect(() => {
    const guard = (event: BeforeUnloadEvent) => {
      if (!dirty) return;
      event.preventDefault();
    };
    window.addEventListener("beforeunload", guard);
    return () => window.removeEventListener("beforeunload", guard);
  }, [dirty]);

  function patch<K extends keyof LessonForm>(key: K, value: LessonForm[K]) {
    setForm((current) => ({ ...current, [key]: value }));
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
    <div className="dialog-backdrop" role="presentation">
      <section className="lesson-dialog" role="dialog" aria-modal="true" aria-labelledby="lesson-dialog-title">
        <header>
          <div>
            <span className="eyebrow">{lesson ? "Редактирование" : "Новое занятие"}</span>
            <h2 id="lesson-dialog-title">{lesson ? lesson.subject : days[form.day_of_week - 1]}</h2>
            {lesson?.base_lesson_id && <p>Правка сохранится поверх версии с сайта.</p>}
          </div>
          <button className="dialog-close" onClick={close} aria-label="Закрыть"><X size={18} /></button>
        </header>

        {reviewing ? (
          <ManualReview form={form} busy={busy} onBack={() => setReviewing(false)} onConfirm={() => onSave(form)} />
        ) : (
          <form onSubmit={submit}>
            <div className="form-grid">
              <label className="field field-wide">
                <span>Предмет</span>
                <input required maxLength={300} value={form.subject} onChange={(event) => patch("subject", event.target.value)} placeholder="Название дисциплины" />
              </label>
              <label className="field">
                <span>Тип занятия</span>
                <select value={form.type} onChange={(event) => patch("type", event.target.value)}>
                  {lessonTypes.map(([value, label]) => <option key={value} value={value}>{label}</option>)}
                </select>
              </label>
              <label className="field">
                <span>Семестр</span>
                <select value={form.semester_id} onChange={(event) => patch("semester_id", event.target.value)} required>
                  {semesters.map((semester) => <option key={semester.id} value={semester.id}>{semester.name}</option>)}
                </select>
              </label>
              <label className="field">
                <span>Повторение</span>
                <select value={form.week_type} onChange={(event) => patch("week_type", event.target.value as EditorLesson["week_type"])}>
                  {Object.entries(weekLabels).map(([value, label]) => <option key={value} value={value}>{label}</option>)}
                </select>
              </label>
              {form.week_type === "date" ? (
                <label className="field">
                  <span>Дата занятия</span>
                  <input type="date" required value={form.special_date} onChange={(event) => patch("special_date", event.target.value)} />
                </label>
              ) : (
                <label className="field">
                  <span>День недели</span>
                  <select value={form.day_of_week} onChange={(event) => patch("day_of_week", Number(event.target.value))}>
                    {days.map((name, index) => <option key={name} value={index + 1}>{name}</option>)}
                  </select>
                </label>
              )}
              <label className="field">
                <span>Начало</span>
                <input type="time" required value={form.time_start} onChange={(event) => patch("time_start", event.target.value)} />
              </label>
              <label className="field">
                <span>Окончание</span>
                <input type="time" required value={form.time_end} onChange={(event) => patch("time_end", event.target.value)} />
              </label>
              {form.week_type !== "date" && (
                <>
                  <label className="field"><span>Действует с</span><input type="date" value={form.valid_from} onChange={(event) => patch("valid_from", event.target.value)} /></label>
                  <label className="field"><span>Действует до</span><input type="date" value={form.valid_to} onChange={(event) => patch("valid_to", event.target.value)} /></label>
                </>
              )}
              <label className="field field-wide">
                <span>Преподаватель</span>
                <input maxLength={200} value={form.teacher} onChange={(event) => patch("teacher", event.target.value)} placeholder="Фамилия и инициалы" />
              </label>
              <label className="field">
                <span>Аудитория</span>
                <input maxLength={100} value={form.room} onChange={(event) => patch("room", event.target.value)} placeholder="Например, А-305" />
              </label>
              <label className="field">
                <span>Подгруппа</span>
                <select value={form.subgroup} onChange={(event) => patch("subgroup", Number(event.target.value))}>
                  <option value={0}>Вся группа</option>
                  <option value={1}>Подгруппа 1</option>
                  <option value={2}>Подгруппа 2</option>
                  <option value={3}>Подгруппа 3</option>
                </select>
              </label>
            </div>
            <footer>
              <span>{dirty ? "Есть несохранённые изменения" : "Изменений нет"}</span>
              <div className="dialog-actions">
                <button type="button" className="button button-ghost" onClick={close}>Отмена</button>
                <button className="button button-primary" disabled={busy || !form.subject.trim()}>
                  <Check size={16} /> Проверить изменения
                </button>
              </div>
            </footer>
          </form>
        )}
      </section>
    </div>
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
          <p>После подтверждения бот начнёт использовать эту запись вместо данных источника.</p>
        </div>
      </div>
      <dl>
        <div><dt>Предмет</dt><dd>{form.subject}</dd></div>
        <div>
          <dt>Когда</dt>
          <dd>{form.week_type === "date" ? form.special_date : days[form.day_of_week - 1]} · {form.time_start}–{form.time_end}</dd>
        </div>
        <div><dt>Преподаватель</dt><dd>{form.teacher || "Не указан"}</dd></div>
        <div><dt>Аудитория</dt><dd>{form.room || "Не указана"}</dd></div>
      </dl>
      <div className="dialog-actions">
        <button type="button" className="button button-ghost" disabled={busy} onClick={onBack}>Вернуться к форме</button>
        <button type="button" className="button button-primary" disabled={busy} onClick={() => void onConfirm()}>
          <Check size={16} /> {busy ? "Применяем…" : "Подтвердить и применить"}
        </button>
      </div>
    </div>
  );
}
