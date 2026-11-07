import { classNames } from "../../../../../shared/classNames";
import dialogStyles from "../../../ui/dialog/Dialog.module.css";
import styles from "./ChangeLedgerDialog.module.css";
import stateStyles from "../../../ui/state/State.module.css";
import { Check, History, RotateCcw, X } from "lucide-react";
import { DialogPortal } from "../../../ui";
import type { EditorLesson } from "../../../shared/model";
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
    <DialogPortal>
      <div className={dialogStyles["dialog-backdrop"]} role="presentation">
        <section
          className={styles["ledger-dialog"]}
          role="dialog"
          aria-modal="true"
          aria-labelledby="ledger-title"
        >
          <header>
            <div className={styles["change-ledger-head"]}>
              <History size={19} />
              <div>
                <h2 id="ledger-title">Ручные изменения</h2>
                <p>Подтверждённые правки итогового расписания</p>
              </div>
            </div>
            <button
              className={dialogStyles["dialog-close"]}
              data-dialog-dismiss
              onClick={onClose}
              aria-label="Закрыть"
            >
              <X size={18} />
            </button>
          </header>
          {empty ? (
            <p className={styles["ledger-empty"]}>
              Расписание совпадает с данными источника.
            </p>
          ) : (
            <div className={styles["ledger-list"]}>
              {additions.map((lesson) => (
                <article key={lesson.id}>
                  <span className={classNames(styles["ledger-mark"], stateStyles["is-added"])}>ДОБ</span>
                  <div>
                    <strong>{lesson.subject}</strong>
                    <span>
                      {days[lessonDay(lesson) - 1]} · добавлено вручную
                    </span>
                  </div>
                </article>
              ))}
              {overrides.map((lesson) => (
                <article key={lesson.id}>
                  <span className={styles["ledger-mark"]}>ИЗМ</span>
                  <div>
                    <strong>{lesson.subject}</strong>
                    <span>
                      {days[lessonDay(lesson) - 1]} · изменено вручную
                    </span>
                  </div>
                  <button
                    disabled={busy}
                    onClick={() => onRestore(lesson)}
                    title="Вернуть версию с сайта"
                  >
                    <RotateCcw size={15} />
                  </button>
                </article>
              ))}
              {deletedLessons.map((lesson) => (
                <article key={lesson.id}>
                  <span className={classNames(styles["ledger-mark"], stateStyles["is-deleted"])}>СКР</span>
                  <div>
                    <strong>{lesson.subject}</strong>
                    <span>скрыто из расписания</span>
                  </div>
                  <button
                    disabled={busy}
                    onClick={() => onRestore(lesson)}
                    title="Вернуть занятие"
                  >
                    <RotateCcw size={15} />
                  </button>
                </article>
              ))}
            </div>
          )}
          <footer className={styles["ledger-note"]}>
            <Check size={16} />
            <span>
              Бот использует только уже подтверждённые изменения из этого
              списка.
            </span>
          </footer>
        </section>
      </div>
    </DialogPortal>
  );
}
