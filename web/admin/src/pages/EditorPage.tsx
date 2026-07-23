import { FormEvent, useEffect, useMemo, useState } from "react";
import {
  CalendarPlus,
  CalendarRange,
  Check,
  Clock3,
  Download,
  FileJson,
  FileSpreadsheet,
  History,
  MapPin,
  Pencil,
  Plus,
  RefreshCw,
  RotateCcw,
  Search,
  Trash2,
  UserRound,
  X,
} from "lucide-react";
import { api } from "../api";
import {
  EmptyBlock,
  ErrorBlock,
  LoadingBlock,
  SearchField,
  type ToastMessage,
} from "../components";
import { useDebounced, useRemote } from "../hooks";
import type {
  EditorLesson,
  EditorSchedule,
  GroupView,
  LessonMutationPayload,
  SemesterOption,
} from "../types";

const days = [
  "Понедельник",
  "Вторник",
  "Среда",
  "Четверг",
  "Пятница",
  "Суббота",
  "Воскресенье",
];

const daysAfterPreposition = [
  "понедельник",
  "вторник",
  "среду",
  "четверг",
  "пятницу",
  "субботу",
  "воскресенье",
];

const lessonTypes = [
  ["lecture", "Лекция"],
  ["practice", "Практика"],
  ["lab", "Лабораторная"],
  ["seminar", "Семинар"],
  ["exam", "Экзамен"],
  ["credit", "Зачёт"],
  ["consultation", "Консультация"],
] as const;

const lessonTypeLabels = Object.fromEntries(lessonTypes);

const weekLabels: Record<EditorLesson["week_type"], string> = {
  every: "Каждую неделю",
  odd: "Нечётная неделя",
  even: "Чётная неделя",
  date: "Точная дата",
};

type WeekFilter = "all" | "odd" | "even" | "date";

type LessonForm = {
  semester_id: string;
  day_of_week: number;
  special_date: string;
  time_start: string;
  time_end: string;
  week_type: EditorLesson["week_type"];
  subject: string;
  type: string;
  teacher: string;
  room: string;
  subgroup: number;
  valid_from: string;
  valid_to: string;
};

export function EditorPage({
  notify,
}: {
  notify: (text: string, tone?: ToastMessage["tone"]) => void;
}) {
  const [university, setUniversity] = useState("");
  const [groupQuery, setGroupQuery] = useState("");
  const [groupSearchOpen, setGroupSearchOpen] = useState(false);
  const [selectedGroupID, setSelectedGroupID] = useState("");
  const [week, setWeek] = useState<WeekFilter>("all");
  const [lessonQuery, setLessonQuery] = useState("");
  const [dialog, setDialog] = useState<{
    lesson: EditorLesson | null;
    day: number;
  } | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<EditorLesson | null>(null);
  const [restoreTarget, setRestoreTarget] = useState<EditorLesson | null>(null);
  const [changesOpen, setChangesOpen] = useState(false);
  const [exportOpen, setExportOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const debouncedGroupQuery = useDebounced(groupQuery);

  const universities = useRemote(() => api.universities(), []);
  const groups = useRemote(
    () =>
      api.groups({
        page: 1,
        pageSize: 20,
        q: debouncedGroupQuery,
        university,
      }),
    [debouncedGroupQuery, university],
    { enabled: debouncedGroupQuery.trim().length > 0 },
  );
  const schedule = useRemote<EditorSchedule | null>(
    () =>
      selectedGroupID
        ? api.editorSchedule(selectedGroupID)
        : Promise.resolve(null),
    [selectedGroupID],
  );
  const groupResults = groups.data?.items ?? [];

  const searchedLessons = useMemo(() => {
    const query = lessonQuery.trim().toLocaleLowerCase("ru-RU");
    return (schedule.data?.lessons ?? []).filter((lesson) => {
      const haystack =
        `${lesson.subject} ${lesson.teacher} ${lesson.room}`.toLocaleLowerCase(
          "ru-RU",
        );
      return !query || haystack.includes(query);
    });
  }, [lessonQuery, schedule.data]);

  const weekSections = useMemo(() => {
    const alternating = searchedLessons.some(
      (lesson) => lesson.week_type === "odd" || lesson.week_type === "even",
    );
    if (week === "odd" || week === "even") {
      return [
        {
          key: week,
          title: week === "odd" ? "Нечётная неделя" : "Чётная неделя",
          note: "Занятия каждой недели показаны отдельно.",
          lessons: searchedLessons.filter(
            (lesson) =>
              lesson.week_type === "every" || lesson.week_type === week,
          ),
        },
      ];
    }
    if (week === "date") {
      return [
        {
          key: "date",
          title: "Занятия по датам",
          note: "Разовые занятия и события.",
          lessons: searchedLessons.filter(
            (lesson) => lesson.week_type === "date",
          ),
        },
      ];
    }
    if (!alternating) {
      return [{ key: "all", title: "", note: "", lessons: searchedLessons }];
    }
    const sections = [
      {
        key: "odd",
        title: "Нечётная неделя",
        note: "Общие занятия включены в обе недели.",
        lessons: searchedLessons.filter(
          (lesson) =>
            lesson.week_type === "every" || lesson.week_type === "odd",
        ),
      },
      {
        key: "even",
        title: "Чётная неделя",
        note: "Общие занятия включены в обе недели.",
        lessons: searchedLessons.filter(
          (lesson) =>
            lesson.week_type === "every" || lesson.week_type === "even",
        ),
      },
    ];
    const dated = searchedLessons.filter(
      (lesson) => lesson.week_type === "date",
    );
    if (dated.length)
      sections.push({
        key: "date",
        title: "Занятия по датам",
        note: "Разовые занятия и события.",
        lessons: dated,
      });
    return sections;
  }, [searchedLessons, week]);

  const editorLessons = schedule.data?.lessons ?? [];
  const deletedLessons = schedule.data?.deleted_lessons ?? [];
  const manualLessons = editorLessons.filter(
    (lesson) => lesson.origin === "manual",
  );

  async function reloadSchedule() {
    await schedule.reload();
  }

  async function saveLesson(form: LessonForm) {
    if (!schedule.data || !dialog) return;
    setBusy(true);
    const payload: LessonMutationPayload = {
      group_id: schedule.data.group.id,
      ...form,
      expected_updated_at: dialog.lesson?.updated_at,
    };
    try {
      if (dialog.lesson) {
        await api.updateEditorLesson(dialog.lesson.id, payload);
        notify("Занятие обновлено");
      } else {
        await api.createEditorLesson(payload);
        notify("Занятие добавлено");
      }
      setDialog(null);
      await reloadSchedule();
    } catch (caught) {
      notify(
        caught instanceof Error
          ? caught.message
          : "Не удалось сохранить занятие",
        "error",
      );
    } finally {
      setBusy(false);
    }
  }

  async function deleteLesson() {
    if (!deleteTarget) return;
    setBusy(true);
    try {
      await api.deleteEditorLesson(deleteTarget);
      notify(
        deleteTarget.base_lesson_id || deleteTarget.origin === "parsed"
          ? "Занятие скрыто. Исходную версию можно восстановить"
          : "Занятие удалено",
      );
      setDeleteTarget(null);
      await reloadSchedule();
    } catch (caught) {
      notify(
        caught instanceof Error ? caught.message : "Не удалось удалить занятие",
        "error",
      );
    } finally {
      setBusy(false);
    }
  }

  async function restoreLesson() {
    if (!restoreTarget) return;
    setBusy(true);
    try {
      await api.restoreEditorLesson(restoreTarget.id);
      notify("Версия с сайта восстановлена");
      setRestoreTarget(null);
      await reloadSchedule();
    } catch (caught) {
      notify(
        caught instanceof Error
          ? caught.message
          : "Не удалось восстановить занятие",
        "error",
      );
    } finally {
      setBusy(false);
    }
  }

  function changeUniversity(value: string) {
    setUniversity(value);
    setSelectedGroupID("");
    setGroupQuery("");
    setGroupSearchOpen(false);
  }

  function selectGroup(group: GroupView) {
    setSelectedGroupID(group.id);
    setGroupQuery(group.name);
    setGroupSearchOpen(false);
  }

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
              onChange={(event) => changeUniversity(event.target.value)}
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
                onFocus={() => setGroupSearchOpen(true)}
                onBlur={() =>
                  window.setTimeout(() => setGroupSearchOpen(false), 120)
                }
                onChange={(event) => {
                  setGroupQuery(event.target.value);
                  setGroupSearchOpen(true);
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
                      onClick={() => selectGroup(group)}
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
              onClick={() => void reloadSchedule()}
            >
              <RefreshCw size={16} /> Обновить
            </button>
            <button
              className="button button-primary"
              onClick={() => setDialog({ lesson: null, day: 1 })}
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
                  onClick={() => setWeek(value)}
                >
                  {label}
                </button>
              ))}
            </div>
            <SearchField
              value={lessonQuery}
              onChange={setLessonQuery}
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
                onClick={() => setChangesOpen(true)}
              >
                <History size={16} /> Правки{" "}
                {manualLessons.length + deletedLessons.length > 0 &&
                  `(${manualLessons.length + deletedLessons.length})`}
              </button>
              <button
                className="button button-ghost"
                onClick={() => setExportOpen(true)}
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
                onAdd={(day) => setDialog({ lesson: null, day })}
                onEdit={(lesson, day) => setDialog({ lesson, day })}
                onDelete={setDeleteTarget}
                onRestore={setRestoreTarget}
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
          onClose={() => setDialog(null)}
          onSave={saveLesson}
        />
      )}

      {deleteTarget && (
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
              <strong>{deleteTarget.subject}</strong> больше не будет
              показываться в расписании бота.
            </p>
            {deleteTarget.origin === "parsed" || deleteTarget.base_lesson_id ? (
              <p className="dialog-note">
                Версию с сайта можно будет восстановить в журнале изменений.
              </p>
            ) : null}
            <div className="dialog-actions">
              <button
                className="button button-ghost"
                disabled={busy}
                onClick={() => setDeleteTarget(null)}
              >
                Отмена
              </button>
              <button
                className="button button-danger"
                disabled={busy}
                onClick={() => void deleteLesson()}
              >
                <Trash2 size={16} /> {busy ? "Удаляем…" : "Удалить"}
              </button>
            </div>
          </section>
        </div>
      )}

      {restoreTarget && (
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
              Ручная правка для <strong>{restoreTarget.subject}</strong> будет
              удалена.
            </p>
            <div className="dialog-actions">
              <button
                className="button button-ghost"
                disabled={busy}
                onClick={() => setRestoreTarget(null)}
              >
                Отмена
              </button>
              <button
                className="button button-primary"
                disabled={busy}
                onClick={() => void restoreLesson()}
              >
                <RotateCcw size={16} />{" "}
                {busy ? "Восстанавливаем…" : "Восстановить"}
              </button>
            </div>
          </section>
        </div>
      )}

      {changesOpen && schedule.data && (
        <ChangeLedgerDialog
          manualLessons={manualLessons}
          deletedLessons={deletedLessons}
          busy={busy}
          onClose={() => setChangesOpen(false)}
          onRestore={(lesson) => {
            setChangesOpen(false);
            setRestoreTarget(lesson);
          }}
        />
      )}

      {exportOpen && schedule.data && (
        <ExportDialog
          schedule={schedule.data}
          onClose={() => setExportOpen(false)}
          notify={notify}
        />
      )}
    </div>
  );
}

function ChangeLedgerDialog({
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
      <section
        className="ledger-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="ledger-title"
      >
        <header>
          <div className="change-ledger-head">
            <History size={19} />
            <div>
              <h2 id="ledger-title">Ручные изменения</h2>
              <p>Подтверждённые правки итогового расписания</p>
            </div>
          </div>
          <button
            className="dialog-close"
            onClick={onClose}
            aria-label="Закрыть"
          >
            <X size={18} />
          </button>
        </header>
        {empty ? (
          <p className="ledger-empty">
            Расписание совпадает с данными источника.
          </p>
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
                <span className="ledger-mark is-deleted">СКР</span>
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
        <footer className="ledger-note">
          <Check size={16} />
          <span>
            Бот использует только уже подтверждённые изменения из этого списка.
          </span>
        </footer>
      </section>
    </div>
  );
}

function ExportDialog({
  schedule,
  onClose,
  notify,
}: {
  schedule: EditorSchedule;
  onClose: () => void;
  notify: (text: string, tone?: ToastMessage["tone"]) => void;
}) {
  const exportOptions = [
    {
      format: "json" as const,
      icon: FileJson,
      title: "JSON",
      text: "Полная структура и служебные поля",
    },
    {
      format: "csv" as const,
      icon: FileSpreadsheet,
      title: "CSV",
      text: "Таблица для Excel и других редакторов",
    },
    {
      format: "ics" as const,
      icon: CalendarRange,
      title: "iCalendar",
      text: "Импорт в календарное приложение",
    },
  ];

  function download(format: "json" | "csv" | "ics") {
    try {
      downloadSchedule(schedule, format);
      notify(`Расписание выгружено в ${format.toUpperCase()}`);
      onClose();
    } catch (caught) {
      notify(
        caught instanceof Error
          ? caught.message
          : "Не удалось выгрузить расписание",
        "error",
      );
    }
  }

  return (
    <div className="dialog-backdrop" role="presentation">
      <section
        className="export-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="export-title"
      >
        <header>
          <div>
            <span className="eyebrow">Резервная копия</span>
            <h2 id="export-title">Выгрузить расписание</h2>
            <p>
              Группа {schedule.group.name} · {schedule.lessons.length}{" "}
              {pluralLessons(schedule.lessons.length)}
            </p>
          </div>
          <button
            className="dialog-close"
            onClick={onClose}
            aria-label="Закрыть"
          >
            <X size={18} />
          </button>
        </header>
        <div className="export-options">
          {exportOptions.map(({ format, icon: Icon, title, text }) => (
            <button key={format} onClick={() => download(format)}>
              <span>
                <Icon size={19} />
              </span>
              <div>
                <strong>{title}</strong>
                <p>{text}</p>
              </div>
              <Download size={17} />
            </button>
          ))}
        </div>
      </section>
    </div>
  );
}

function ScheduleWeekBoard({
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
    <section className={`schedule-week-section ${title ? "has-title" : ""}`}>
      {title && (
        <header className="schedule-week-heading">
          <div>
            <span>Учебная неделя</span>
            <h3>{title}</h3>
          </div>
          <p>{note}</p>
        </header>
      )}
      <div
        className="week-board"
        aria-label={title || "Расписание по дням недели"}
      >
        {days.map((dayName, index) => {
          const day = index + 1;
          const dayLessons = lessons
            .filter((lesson) => lessonDay(lesson) === day)
            .sort((a, b) => a.time_start.localeCompare(b.time_start));
          return (
            <article className="day-column" key={dayName}>
              <header>
                <div>
                  <span>{String(day).padStart(2, "0")}</span>
                  <h3>{dayName}</h3>
                  <p>
                    {dayLessons.length} {pluralLessons(dayLessons.length)}
                  </p>
                </div>
                <button
                  className="day-add"
                  onClick={() => onAdd(day)}
                  aria-label={`Добавить занятие в ${daysAfterPreposition[index]}`}
                >
                  <Plus size={16} />
                </button>
              </header>
              <div className="day-lessons">
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
                  <button className="day-empty" onClick={() => onAdd(day)}>
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
      className={`board-lesson type-${lesson.type} ${lesson.origin === "manual" ? "is-manual" : ""}`}
    >
      <div className="board-lesson-head">
        <span className="board-time">
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
      <span className="board-type">
        {lessonTypeLabels[lesson.type] ?? lesson.type}
      </span>
      <h4>{lesson.subject}</h4>
      <div className="board-details">
        <span>
          <UserRound size={13} /> {lesson.teacher || "Не указан"}
        </span>
        <span>
          <MapPin size={13} /> {lesson.room || "Не указана"}
        </span>
      </div>
      <div className="board-meta">
        <span>{weekLabels[lesson.week_type]}</span>
        {lesson.subgroup > 0 && <span>{lesson.subgroup} подгр.</span>}
        {lesson.origin === "manual" && (
          <span className="manual-badge">ручная правка</span>
        )}
      </div>
      {lesson.base_lesson_id && (
        <button
          className="restore-inline"
          onClick={onRestore}
          disabled={disabled}
        >
          <RotateCcw size={13} /> Вернуть с сайта
        </button>
      )}
    </article>
  );
}

function LessonDialog({
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
  const [form, setForm] = useState<LessonForm>(() =>
    formFromLesson(lesson, day, semesters),
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
          <button className="dialog-close" onClick={close} aria-label="Закрыть">
            <X size={18} />
          </button>
        </header>

        {reviewing ? (
          <div className="manual-review">
            <div className="manual-review-notice">
              <Check size={18} />
              <div>
                <strong>Подтвердите применение ручной версии</strong>
                <p>
                  После подтверждения бот начнёт использовать эту запись вместо
                  данных источника.
                </p>
              </div>
            </div>
            <dl>
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
                onClick={() => setReviewing(false)}
              >
                Вернуться к форме
              </button>
              <button
                type="button"
                className="button button-primary"
                disabled={busy}
                onClick={() => void onSave(form)}
              >
                <Check size={16} />{" "}
                {busy ? "Применяем…" : "Подтвердить и применить"}
              </button>
            </div>
          </div>
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
                  onChange={(event) => patch("semester_id", event.target.value)}
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
                  value={form.week_type}
                  onChange={(event) =>
                    patch(
                      "week_type",
                      event.target.value as EditorLesson["week_type"],
                    )
                  }
                >
                  {Object.entries(weekLabels).map(([value, label]) => (
                    <option key={value} value={value}>
                      {label}
                    </option>
                  ))}
                </select>
              </label>
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
                  onChange={(event) => patch("time_start", event.target.value)}
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

            <footer>
              <span>
                {dirty ? "Есть несохранённые изменения" : "Изменений нет"}
              </span>
              <div className="dialog-actions">
                <button
                  type="button"
                  className="button button-ghost"
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
  );
}

function downloadSchedule(
  schedule: EditorSchedule,
  format: "json" | "csv" | "ics",
) {
  const baseName = `schedule-${schedule.group.name}`
    .replace(/[<>:"/\\|?*]+/g, "-")
    .replace(/\s+/g, "-");
  let contents = "";
  let mimeType = "application/octet-stream";

  if (format === "json") {
    contents = JSON.stringify(
      {
        exported_at: new Date().toISOString(),
        group: schedule.group,
        semesters: schedule.semesters,
        lessons: schedule.lessons,
        deleted_lessons: schedule.deleted_lessons,
      },
      null,
      2,
    );
    mimeType = "application/json;charset=utf-8";
  } else if (format === "csv") {
    const rows = [
      [
        "Группа",
        "День",
        "Дата",
        "Начало",
        "Окончание",
        "Неделя",
        "Предмет",
        "Тип",
        "Преподаватель",
        "Аудитория",
        "Подгруппа",
        "Действует с",
        "Действует до",
        "Источник",
      ],
      ...schedule.lessons.map((lesson) => [
        schedule.group.name,
        days[lessonDay(lesson) - 1],
        datePart(lesson.special_date),
        lesson.time_start,
        lesson.time_end,
        weekLabels[lesson.week_type],
        lesson.subject,
        lessonTypeLabels[lesson.type] ?? lesson.type,
        lesson.teacher,
        lesson.room,
        lesson.subgroup ? String(lesson.subgroup) : "вся группа",
        datePart(lesson.valid_from),
        datePart(lesson.valid_to),
        lesson.origin === "manual" ? "ручная правка" : "сайт",
      ]),
    ];
    contents = `\uFEFF${rows.map((row) => row.map(csvCell).join(";")).join("\r\n")}`;
    mimeType = "text/csv;charset=utf-8";
  } else {
    const semester = schedule.semesters[0];
    const now = new Date()
      .toISOString()
      .replace(/[-:]/g, "")
      .replace(/\.\d{3}Z$/, "Z");
    const events = schedule.lessons.flatMap((lesson) => {
      const firstDate = firstLessonDate(lesson, semester);
      if (!firstDate) return [];
      const lines = [
        "BEGIN:VEVENT",
        `UID:${icsEscape(lesson.id)}@scheduler`,
        `DTSTAMP:${now}`,
        `DTSTART:${icsDateTime(firstDate, lesson.time_start)}`,
        `DTEND:${icsDateTime(firstDate, lesson.time_end)}`,
        `SUMMARY:${icsEscape(lesson.subject)}`,
      ];
      if (lesson.room) lines.push(`LOCATION:${icsEscape(lesson.room)}`);
      const description = [
        lesson.teacher,
        lessonTypeLabels[lesson.type],
        lesson.subgroup ? `Подгруппа ${lesson.subgroup}` : "",
      ]
        .filter(Boolean)
        .join(" · ");
      if (description) lines.push(`DESCRIPTION:${icsEscape(description)}`);
      if (lesson.week_type !== "date") {
        const interval = lesson.week_type === "every" ? 1 : 2;
        const until = datePart(lesson.valid_to) || datePart(semester?.end_date);
        lines.push(
          `RRULE:FREQ=WEEKLY;INTERVAL=${interval}${until ? `;UNTIL=${until.replaceAll("-", "")}T235959` : ""}`,
        );
      }
      lines.push("END:VEVENT");
      return lines;
    });
    contents = [
      "BEGIN:VCALENDAR",
      "VERSION:2.0",
      "PRODID:-//Scheduler//Admin export//RU",
      "CALSCALE:GREGORIAN",
      ...events,
      "END:VCALENDAR",
      "",
    ].join("\r\n");
    mimeType = "text/calendar;charset=utf-8";
  }

  const url = URL.createObjectURL(new Blob([contents], { type: mimeType }));
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = `${baseName}.${format}`;
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  window.setTimeout(() => URL.revokeObjectURL(url), 1000);
}

function csvCell(value: string) {
  return `"${String(value ?? "").replaceAll('"', '""')}"`;
}

function icsEscape(value: string) {
  return value
    .replaceAll("\\", "\\\\")
    .replaceAll("\n", "\\n")
    .replaceAll(",", "\\,")
    .replaceAll(";", "\\;");
}

function icsDateTime(date: string, time: string) {
  return `${date.replaceAll("-", "")}T${time.replace(":", "")}00`;
}

function firstLessonDate(lesson: EditorLesson, semester?: SemesterOption) {
  if (lesson.special_date) return datePart(lesson.special_date);
  const initial = datePart(lesson.valid_from) || datePart(semester?.start_date);
  if (!initial) return "";
  const cursor = new Date(`${initial}T12:00:00`);
  const target = lessonDay(lesson) % 7;
  while (cursor.getDay() !== target) cursor.setDate(cursor.getDate() + 1);
  return `${cursor.getFullYear()}-${String(cursor.getMonth() + 1).padStart(2, "0")}-${String(cursor.getDate()).padStart(2, "0")}`;
}

function formFromLesson(
  lesson: EditorLesson | null,
  day: number,
  semesters: SemesterOption[],
): LessonForm {
  const semester = semesters[0];
  if (!lesson) {
    return {
      semester_id: semester?.id ?? "",
      day_of_week: day,
      special_date: "",
      time_start: "09:00",
      time_end: "10:30",
      week_type: "every",
      subject: "",
      type: "lecture",
      teacher: "",
      room: "",
      subgroup: 0,
      valid_from: datePart(semester?.start_date),
      valid_to: datePart(semester?.end_date),
    };
  }
  return {
    semester_id: lesson.semester_id,
    day_of_week: lessonDay(lesson),
    special_date: datePart(lesson.special_date),
    time_start: lesson.time_start,
    time_end: lesson.time_end,
    week_type: lesson.week_type,
    subject: lesson.subject,
    type: lesson.type,
    teacher: lesson.teacher,
    room: lesson.room,
    subgroup: lesson.subgroup,
    valid_from: datePart(lesson.valid_from),
    valid_to: datePart(lesson.valid_to),
  };
}

function datePart(value?: string | null) {
  return value ? value.slice(0, 10) : "";
}

function lessonDay(lesson: EditorLesson) {
  if (lesson.day_of_week) return lesson.day_of_week;
  if (!lesson.special_date) return 1;
  const day = new Date(`${datePart(lesson.special_date)}T12:00:00`).getDay();
  return day === 0 ? 7 : day;
}

function pluralLessons(value: number) {
  const mod10 = value % 10;
  const mod100 = value % 100;
  if (mod10 === 1 && mod100 !== 11) return "занятие";
  if (mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14))
    return "занятия";
  return "занятий";
}
