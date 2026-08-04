import {
  CalendarPlus,
  Download,
  History,
  RefreshCw,
  RotateCcw,
  Search,
  Trash2,
} from "lucide-react";
import {
  EmptyBlock,
  ErrorBlock,
  LoadingBlock,
  SearchField,
  type ToastMessage,
} from "../components";
import { ChangeLedgerDialog } from "../features/schedule-editor/components/ChangeLedgerDialog";
import { ExportDialog } from "../features/schedule-editor/components/ExportDialog";
import { LessonDialog } from "../features/schedule-editor/components/LessonDialog";
import { ScheduleWeekBoard } from "../features/schedule-editor/components/ScheduleWeekBoard";
import { useScheduleEditor } from "../features/schedule-editor/useScheduleEditor";

export function EditorPage({
  notify,
}: {
  notify: (text: string, tone?: ToastMessage["tone"]) => void;
}) {
  const editor = useScheduleEditor(notify);
  const {
    university,
    groupQuery,
    groupSearchOpen,
    selectedGroupID,
    week,
    lessonQuery,
    dialog,
    deleteTarget,
    restoreTarget,
    changesOpen,
    exportOpen,
    busy,
    universities,
    groups,
    schedule,
    groupResults,
    weekSections,
    editorLessons,
    deletedLessons,
    manualLessons,
  } = editor;

  return (
    <div className="page-stack editor-page">
      <section className="editor-context card-surface">
        <div className="editor-context-copy">
          <span className="eyebrow">Рабочая область</span>
          <h2>
            {schedule.data
              ? `Группа ${schedule.data.group.name}`
              : "Редактор расписания"}
          </h2>
          <p>
            {schedule.data
              ? `${schedule.data.group.university_name} · изменения сразу видны в боте`
              : "Выберите группу, чтобы открыть недельное расписание."}
          </p>
        </div>

        <div className="editor-group-controls">
          <label>
            <span>Университет</span>
            <select
              value={university}
              onChange={(event) => editor.changeUniversity(event.target.value)}
            >
              <option value="">Все университеты</option>
              {(universities.data ?? []).map((item) => (
                <option key={item.id} value={item.id}>
                  {item.name}
                </option>
              ))}
            </select>
          </label>
          <label className="editor-group-search editor-group-search-wide">
            <span>Группа</span>
            <div className="editor-group-search-input">
              <Search size={16} />
              <input
                value={groupQuery}
                onFocus={() => editor.setGroupSearchOpen(true)}
                onBlur={() =>
                  window.setTimeout(() => editor.setGroupSearchOpen(false), 120)
                }
                onChange={(event) => {
                  editor.setGroupQuery(event.target.value);
                  editor.setGroupSearchOpen(true);
                }}
                placeholder="Введите номер или часть названия"
                autoComplete="off"
              />
            </div>
            {groupSearchOpen && groupQuery.trim() && (
              <div className="editor-group-results" role="listbox">
                {groups.loading ? (
                  <span className="editor-group-result-state">
                    Ищем группы…
                  </span>
                ) : groups.error ? (
                  <span className="editor-group-result-state is-error">
                    {groups.error}
                  </span>
                ) : groupResults.length ? (
                  groupResults.map((group) => (
                    <button
                      type="button"
                      role="option"
                      aria-selected={group.id === selectedGroupID}
                      key={group.id}
                      onMouseDown={(event) => event.preventDefault()}
                      onClick={() => editor.selectGroup(group)}
                    >
                      <strong>{group.name}</strong>
                      <span>
                        {group.university_name} · {group.lesson_count} занятий
                      </span>
                    </button>
                  ))
                ) : (
                  <span className="editor-group-result-state">
                    Совпадений не найдено
                  </span>
                )}
                {(groups.data?.pagination?.total ?? 0) > 20 && (
                  <span className="editor-group-result-state">
                    Показаны первые 20 результатов — уточните запрос.
                  </span>
                )}
              </div>
            )}
          </label>
        </div>

        {schedule.data && (
          <div className="editor-context-actions">
            <button
              className="button button-ghost"
              onClick={() => void schedule.reload()}
            >
              <RefreshCw size={16} /> Обновить
            </button>
            <button
              className="button button-primary"
              onClick={() => editor.setDialog({ lesson: null, day: 1 })}
            >
              <CalendarPlus size={17} /> Добавить занятие
            </button>
          </div>
        )}
      </section>

      {!selectedGroupID ? (
        <EmptyBlock
          title="Группа не выбрана"
          text="Найдите группу в поле выше."
        />
      ) : schedule.loading && !schedule.data ? (
        <LoadingBlock rows={6} />
      ) : schedule.error ? (
        <ErrorBlock message={schedule.error} retry={schedule.reload} />
      ) : schedule.data ? (
        <>
          <section className="editor-toolbar">
            <div className="week-switch" aria-label="Показать расписание">
              {(
                [
                  ["all", "Всё"],
                  ["odd", "Нечётная"],
                  ["even", "Чётная"],
                  ["date", "По датам"],
                ] as const
              ).map(([value, label]) => (
                <button
                  key={value}
                  className={week === value ? "is-active" : ""}
                  onClick={() => editor.setWeek(value)}
                >
                  {label}
                </button>
              ))}
            </div>
            <SearchField
              value={lessonQuery}
              onChange={editor.setLessonQuery}
              placeholder="Предмет, преподаватель или аудитория"
            />
            <div className="editor-stats">
              <span>
                <strong>{editorLessons.length}</strong> занятий
              </span>
              <span>
                <strong>{manualLessons.length}</strong> ручных
              </span>
              <span>
                <strong>{deletedLessons.length}</strong> скрыто
              </span>
            </div>
            <div className="editor-toolbar-actions">
              <button
                className="button button-ghost"
                onClick={() => editor.setChangesOpen(true)}
              >
                <History size={16} /> Правки{" "}
                {manualLessons.length + deletedLessons.length > 0 &&
                  `(${manualLessons.length + deletedLessons.length})`}
              </button>
              <button
                className="button button-ghost"
                onClick={() => editor.setExportOpen(true)}
              >
                <Download size={16} /> Выгрузить
              </button>
            </div>
          </section>

          <div className="editor-workspace schedule-week-stack">
            {weekSections.map((section) => (
              <ScheduleWeekBoard
                key={section.key}
                title={section.title}
                note={section.note}
                lessons={section.lessons}
                busy={busy}
                onAdd={(day) => editor.setDialog({ lesson: null, day })}
                onEdit={(lesson, day) => editor.setDialog({ lesson, day })}
                onDelete={editor.setDeleteTarget}
                onRestore={editor.setRestoreTarget}
              />
            ))}
          </div>
        </>
      ) : null}

      {dialog && schedule.data && (
        <LessonDialog
          lesson={dialog.lesson}
          day={dialog.day}
          semesters={schedule.data.semesters}
          busy={busy}
          onClose={() => editor.setDialog(null)}
          onSave={editor.saveLesson}
        />
      )}

      {deleteTarget && (
        <ConfirmDelete
          subject={deleteTarget.subject}
          parsed={
            deleteTarget.origin === "parsed" ||
            Boolean(deleteTarget.base_lesson_id)
          }
          busy={busy}
          onCancel={() => editor.setDeleteTarget(null)}
          onConfirm={() => void editor.deleteLesson()}
        />
      )}

      {restoreTarget && (
        <ConfirmRestore
          subject={restoreTarget.subject}
          busy={busy}
          onCancel={() => editor.setRestoreTarget(null)}
          onConfirm={() => void editor.restoreLesson()}
        />
      )}

      {changesOpen && schedule.data && (
        <ChangeLedgerDialog
          manualLessons={manualLessons}
          deletedLessons={deletedLessons}
          busy={busy}
          onClose={() => editor.setChangesOpen(false)}
          onRestore={(lesson) => {
            editor.setChangesOpen(false);
            editor.setRestoreTarget(lesson);
          }}
        />
      )}

      {exportOpen && schedule.data && (
        <ExportDialog
          schedule={schedule.data}
          onClose={() => editor.setExportOpen(false)}
          notify={notify}
        />
      )}
    </div>
  );
}

function ConfirmDelete({
  subject,
  parsed,
  busy,
  onCancel,
  onConfirm,
}: {
  subject: string;
  parsed: boolean;
  busy: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  return (
    <div className="dialog-backdrop" role="presentation">
      <section
        className="confirm-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="delete-title"
      >
        <span className="dialog-danger-icon">
          <Trash2 size={20} />
        </span>
        <h2 id="delete-title">Удалить занятие?</h2>
        <p>
          <strong>{subject}</strong> больше не будет показываться в расписании
          бота.
        </p>
        {parsed && (
          <p className="dialog-note">
            Версию с сайта можно будет восстановить в журнале изменений.
          </p>
        )}
        <div className="dialog-actions">
          <button
            className="button button-ghost"
            disabled={busy}
            onClick={onCancel}
          >
            Отмена
          </button>
          <button
            className="button button-danger"
            disabled={busy}
            onClick={onConfirm}
          >
            <Trash2 size={16} /> {busy ? "Удаляем…" : "Удалить"}
          </button>
        </div>
      </section>
    </div>
  );
}

function ConfirmRestore({
  subject,
  busy,
  onCancel,
  onConfirm,
}: {
  subject: string;
  busy: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  return (
    <div className="dialog-backdrop" role="presentation">
      <section
        className="confirm-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="restore-title"
      >
        <span className="dialog-neutral-icon">
          <RotateCcw size={20} />
        </span>
        <h2 id="restore-title">Вернуть версию с сайта?</h2>
        <p>
          Ручная правка для <strong>{subject}</strong> будет удалена.
        </p>
        <div className="dialog-actions">
          <button
            className="button button-ghost"
            disabled={busy}
            onClick={onCancel}
          >
            Отмена
          </button>
          <button
            className="button button-primary"
            disabled={busy}
            onClick={onConfirm}
          >
            <RotateCcw size={16} /> {busy ? "Восстанавливаем…" : "Восстановить"}
          </button>
        </div>
      </section>
    </div>
  );
}
