import { Check, History, RotateCcw, X } from "lucide-react";
import type { EditorLesson } from "../../../types";
import { days, lessonDay } from "../model";

export function ChangeLedgerDialog({
  manualLessons,
  deletedLessons,
  busy,
  onClose,
  onRestore,
}: {
  manualLessons: EditorLesson[];
  deletedLessons: EditorLesson[];
  busy: boolean;
  onClose: () => void;
  onRestore: (lesson: EditorLesson) => void;
}) {
  const overrides = manualLessons.filter((lesson) => lesson.base_lesson_id);
  const additions = manualLessons.filter((lesson) => !lesson.base_lesson_id);
  const empty = !manualLessons.length && !deletedLessons.length;

  return (
    <div className="dialog-backdrop" role="presentation">
      <section className="ledger-dialog" role="dialog" aria-modal="true" aria-labelledby="ledger-title">
        <header>
          <div className="change-ledger-head">
            <History size={19} />
            <div>
              <h2 id="ledger-title">Ручные изменения</h2>
              <p>Подтверждённые правки итогового расписания</p>
            </div>
          </div>
          <button className="dialog-close" onClick={onClose} aria-label="Закрыть">
            <X size={18} />
          </button>
        </header>
        {empty ? (
          <p className="ledger-empty">Расписание совпадает с данными источника.</p>
        ) : (
          <div className="ledger-list">
            {additions.map((lesson) => (
              <article key={lesson.id}>
                <span className="ledger-mark is-added">ДОБ</span>
                <div>
                  <strong>{lesson.subject}</strong>
                  <span>{days[lessonDay(lesson) - 1]} · добавлено вручную</span>
                </div>
              </article>
            ))}
            {overrides.map((lesson) => (
              <article key={lesson.id}>
                <span className="ledger-mark">ИЗМ</span>
                <div>
                  <strong>{lesson.subject}</strong>
                  <span>{days[lessonDay(lesson) - 1]} · изменено вручную</span>
                </div>
                <button disabled={busy} onClick={() => onRestore(lesson)} title="Вернуть версию с сайта">
                  <RotateCcw size={15} />
                </button>
              </article>
            ))}
            {deletedLessons.map((lesson) => (
              <article key={lesson.id}>
                <span className="ledger-mark is-deleted">СКР</span>
                <div>
                  <strong>{lesson.subject}</strong>
                  <span>скрыто из расписания</span>
                </div>
                <button disabled={busy} onClick={() => onRestore(lesson)} title="Вернуть занятие">
                  <RotateCcw size={15} />
                </button>
              </article>
            ))}
          </div>
        )}
        <footer className="ledger-note">
          <Check size={16} />
          <span>Бот использует только уже подтверждённые изменения из этого списка.</span>
        </footer>
      </section>
    </div>
  );
}
