import { formatDate, number } from "../../shared/lib/format";
import { classNames } from "../../../../shared/classNames";
import layoutStyles from "../../ui/layout/Layout.module.css";
import styles from "./DataPage.module.css";
import stateStyles from "../../ui/state/State.module.css";
import formStyles from "../../ui/form/Form.module.css";
import tableStyles from "../../ui/table/Table.module.css";
import buttonStyles from "../../ui/button/Button.module.css";
import dialogStyles from "../../ui/dialog/Dialog.module.css";
import { useViewState } from "../../shared/hooks/useViewState";
import { useEffect, useRef, useState } from "react";
import {
  AlertTriangle,
  BookOpen,
  CalendarDays,
  Eye,
  Layers3,
  PauseCircle,
  PlayCircle,
  Trash2,
  UsersRound,
} from "lucide-react";
import { api } from "../../shared/api";
import { DialogPortal, EmptyBlock, ErrorBlock, LoadingBlock, PaginationControls, SearchField, SectionTitle, SourceGlyph, type ToastMessage } from "../../ui";
import { useDebounced, useRemote } from "../../shared/hooks";
import type { GroupView } from "../../shared/model";

const lessonLabels: Record<string, string> = {
  lecture: "Лекция",
  practice: "Практика",
  lab: "Лабораторная",
  seminar: "Семинар",
  exam: "Экзамен",
  credit: "Зачёт",
  consultation: "Консультация",
  other: "Другое",
};

type GroupStatus = "active" | "inactive" | "all";
type GroupOrder = "name" | "newest" | "oldest";
type GroupAction = "activate" | "deactivate" | "delete";

export function DataPage({
  canManage = false,
  notify,
}: {
  canManage?: boolean;
  notify: (text: string, tone?: ToastMessage["tone"]) => void;
}) {
  const [tab, setTab] = useViewState<"groups" | "lessons">(
    "DataPage:tab",
    "groups",
  );
  const [query, setQuery] = useViewState("DataPage:query", "");
  const [university, setUniversity] = useViewState("DataPage:university", "");
  const [groupStatus, setGroupStatus] = useViewState<GroupStatus>(
    "DataPage:groupStatus",
    "active",
  );
  const [groupOrder, setGroupOrder] = useViewState<GroupOrder>(
    "DataPage:groupOrder",
    "name",
  );
  const [page, setPage] = useViewState("DataPage:page", 1);
  const [selectedGroup, setSelectedGroup] = useViewState<GroupView | null>(
    "DataPage:selectedGroup",
    null,
  );
  const [groupAction, setGroupAction] = useState<{
    group: GroupView;
    action: GroupAction;
  } | null>(null);
  const [busyGroup, setBusyGroup] = useState("");
  const debounced = useDebounced(query);
  const universities = useRemote(() => api.universities(), []);
  const groups = useRemote(
    () =>
      api.groups({
        page,
        q: debounced,
        university,
        status: groupStatus,
        order: groupOrder,
      }),
    [page, debounced, university, groupStatus, groupOrder],
    { enabled: tab === "groups" },
  );
  const lessons = useRemote(
    () =>
      api.lessons({ page, q: debounced, university, group: selectedGroup?.id }),
    [page, debounced, university, selectedGroup?.id],
    { enabled: tab === "lessons" },
  );

  const initialPage = useRef(true);
  useEffect(() => {
    if (initialPage.current) {
      initialPage.current = false;
      return;
    }
    setPage(1);
  }, [
    debounced,
    university,
    groupStatus,
    groupOrder,
    tab,
    selectedGroup?.id,
    setPage,
  ]);

  const openGroup = (group: GroupView) => {
    setSelectedGroup(group);
    setQuery("");
    setTab("lessons");
  };

  const switchTab = (nextTab: "groups" | "lessons") => {
    setTab(nextTab);
    setQuery("");
    if (nextTab === "groups") setSelectedGroup(null);
  };

  async function applyGroupAction() {
    if (!groupAction) return;
    const { group, action } = groupAction;
    setBusyGroup(group.id);
    try {
      if (action === "delete") {
        await api.deleteGroup(group.id);
        notify(`Группа ${group.name} удалена`);
      } else {
        const active = action === "activate";
        await api.updateGroup(group.id, active);
        notify(
          active
            ? `Группа ${group.name} снова активна`
            : `Группа ${group.name} отключена`,
        );
      }
      setGroupAction(null);
      await groups.reload();
    } catch (caught) {
      notify(
        caught instanceof Error
          ? caught.message
          : "Не удалось изменить состояние группы",
        "error",
      );
    } finally {
      setBusyGroup("");
    }
  }

  return (
    <div className={classNames(layoutStyles["page-stack"], styles["data-page"])}>
      <div
        className={classNames(styles["data-toolbar"], tab === "groups" ? stateStyles["has-group-status"] : undefined)}
      >
        <div className={formStyles["segmented-control"]}>
          <button
            className={tab === "groups" ? stateStyles["is-active"] : undefined}
            onClick={() => switchTab("groups")}
          >
            <UsersRound size={17} /> Группы
          </button>
          <button
            className={tab === "lessons" ? stateStyles["is-active"] : undefined}
            onClick={() => switchTab("lessons")}
          >
            <BookOpen size={17} /> Занятия
          </button>
        </div>
        <SearchField
          value={query}
          onChange={setQuery}
          placeholder={
            tab === "groups"
              ? "Номер группы или ID"
              : "Предмет, преподаватель, аудитория"
          }
        />
        <select
          className={formStyles["select-control"]}
          value={university}
          onChange={(event) => setUniversity(event.target.value)}
          aria-label="Университет"
        >
          <option value="">Все университеты</option>
          {(universities.data ?? []).map((item) => (
            <option value={item.id} key={item.id}>
              {item.name}
            </option>
          ))}
        </select>
        {tab === "groups" && (
          <>
            <select
              className={formStyles["select-control"]}
              value={groupStatus}
              onChange={(event) =>
                setGroupStatus(event.target.value as GroupStatus)
              }
              aria-label="Состояние групп"
            >
              <option value="active">Активные группы</option>
              <option value="inactive">Неактивные группы</option>
              <option value="all">Все группы</option>
            </select>
            <select
              className={formStyles["select-control"]}
              value={groupOrder}
              onChange={(event) =>
                setGroupOrder(event.target.value as GroupOrder)
              }
              aria-label="Сортировка групп"
            >
              <option value="name">По названию</option>
              <option value="newest">Сначала новые</option>
              <option value="oldest">Сначала старые</option>
            </select>
          </>
        )}
      </div>

      {selectedGroup && tab === "lessons" && (
        <div className={styles["active-filter"]}>
          <SourceGlyph name={selectedGroup.university_name} small />
          <span>
            Расписание группы <strong>{selectedGroup.name}</strong>
          </span>
          <button onClick={() => setSelectedGroup(null)}>Показать все</button>
        </div>
      )}

      {tab === "groups" ? (
        <section className={classNames(layoutStyles["card-surface"], tableStyles["table-card"])}>
          <SectionTitle
            eyebrow="Справочник"
            title="Учебные группы"
            action={
              <span className={styles["directory-count"]}>
                {number.format(groups.data?.pagination.total ?? 0)} групп
              </span>
            }
          />
          {groups.loading && !groups.data ? (
            <LoadingBlock rows={6} />
          ) : groups.error ? (
            <ErrorBlock message={groups.error} retry={groups.reload} />
          ) : !groups.data?.items.length ? (
            <EmptyBlock
              title="Группы не найдены"
              text="Попробуйте изменить университет, состояние или поисковый запрос."
            />
          ) : (
            <>
              <div className={classNames(tableStyles["responsive-table"], styles["groups-table"])}>
                <div className={tableStyles["table-head"]}>
                  <span>Группа</span>
                  <span>Состояние</span>
                  <span>Университет</span>
                  <span>Связанные данные</span>
                  <span>Появилась</span>
                  <span>Действия</span>
                </div>
                {groups.data.items.map((group) => (
                  <div
                    className={classNames(tableStyles["table-row"], styles["group-directory-row"], group.is_active ? undefined : stateStyles["is-inactive"])}
                    key={group.id}
                  >
                    <div data-label="Группа">
                      <SourceGlyph name={group.university_name} small />
                      <button
                        className={styles["group-name-button"]}
                        onClick={() => openGroup(group)}
                      >
                        <strong>{group.name}</strong>
                        <span>{group.id}</span>
                      </button>
                    </div>
                    <div data-label="Состояние">
                      <GroupLifecycleStatus group={group} />
                    </div>
                    <div data-label="Университет">
                      <strong>{group.university_name}</strong>
                    </div>
                    <div data-label="Связанные данные">
                      <strong>
                        {number.format(group.lesson_count)} занятий ·{" "}
                        {number.format(group.subscription_count)} подписок
                      </strong>
                      <span>
                        Основная у {number.format(group.default_group_count)} ·
                        чатов {number.format(group.chat_count)} · правок{" "}
                        {number.format(group.override_count)}
                      </span>
                    </div>
                    <div data-label="Появилась">
                      <strong>{formatDate(group.created_at)}</strong>
                      <span>Обновлена {formatDate(group.updated_at)}</span>
                    </div>
                    <div className={styles["group-row-actions"]} data-label="Действия">
                      {canManage &&
                        (group.is_active ? (
                          <button
                            className={classNames(buttonStyles["button"], buttonStyles["button-ghost"])}
                            onClick={() =>
                              setGroupAction({ group, action: "deactivate" })
                            }
                          >
                            <PauseCircle size={15} /> Отключить
                          </button>
                        ) : group.manually_disabled && group.source_active ? (
                          <button
                            className={classNames(buttonStyles["button"], buttonStyles["button-ghost"])}
                            onClick={() =>
                              setGroupAction({ group, action: "activate" })
                            }
                          >
                            <PlayCircle size={15} /> Включить
                          </button>
                        ) : null)}
                      {canManage && !group.source_active && (
                        <button
                          className={classNames(buttonStyles["button"], buttonStyles["button-danger-soft"])}
                          onClick={() =>
                            setGroupAction({ group, action: "delete" })
                          }
                        >
                          <Trash2 size={15} /> Удалить
                        </button>
                      )}
                      <button
                        className={classNames(buttonStyles["button"], buttonStyles["button-ghost"])}
                        onClick={() => openGroup(group)}
                      >
                        <Eye size={15} /> Занятия
                      </button>
                    </div>
                  </div>
                ))}
              </div>
              <PaginationControls
                pagination={groups.data.pagination}
                onPage={setPage}
              />
            </>
          )}
        </section>
      ) : (
        <section className={styles["lesson-browser"]}>
          <SectionTitle
            eyebrow="Снимок"
            title={
              selectedGroup ? `Группа ${selectedGroup.name}` : "Все занятия"
            }
          />
          {lessons.loading && !lessons.data ? (
            <LoadingBlock rows={6} />
          ) : lessons.error ? (
            <ErrorBlock message={lessons.error} retry={lessons.reload} />
          ) : !lessons.data?.items.length ? (
            <EmptyBlock
              title="Занятия не найдены"
              text="В актуальном снимке нет подходящих записей."
            />
          ) : (
            <>
              <div className={styles["lesson-grid"]}>
                {lessons.data.items.map((lesson) => (
                  <article className={styles["lesson-card"]} key={lesson.id}>
                    <div className={styles["lesson-time"]}>
                      <strong>{lesson.time_start}</strong>
                      <span>{lesson.time_end}</span>
                      <i />
                    </div>
                    <div className={styles["lesson-main"]}>
                      <span className={styles["lesson-type"]}>
                        {lessonLabels[lesson.type] ?? lesson.type}
                      </span>
                      <h3>{lesson.subject}</h3>
                      <p>
                        {lesson.teacher || "Преподаватель не указан"} ·{" "}
                        {lesson.room || "аудитория не указана"}
                      </p>
                      <div>
                        <span>
                          <Layers3 size={14} /> {lesson.group_name}
                        </span>
                        <span>
                          <CalendarDays size={14} />{" "}
                          {lesson.special_date
                            ? formatDate(lesson.special_date)
                            : `${formatDate(lesson.valid_from)} — ${formatDate(lesson.valid_to)}`}
                        </span>
                      </div>
                    </div>
                  </article>
                ))}
              </div>
              <PaginationControls
                pagination={lessons.data.pagination}
                onPage={setPage}
              />
            </>
          )}
        </section>
      )}

      {groupAction && (
        <GroupLifecycleDialog
          group={groupAction.group}
          action={groupAction.action}
          busy={busyGroup === groupAction.group.id}
          onCancel={() => setGroupAction(null)}
          onConfirm={() => void applyGroupAction()}
        />
      )}
    </div>
  );
}

function GroupLifecycleStatus({ group }: { group: GroupView }) {
  if (group.is_active) {
    return (
      <span className={classNames(styles["group-lifecycle"], stateStyles["is-active"])}>
        <strong>Активна</strong>
        <span>есть в актуальном снимке</span>
      </span>
    );
  }
  if (group.manually_disabled) {
    return (
      <span className={classNames(styles["group-lifecycle"], stateStyles["is-manual"])}>
        <strong>Отключена вручную</strong>
        <span>
          {group.source_active
            ? "источник продолжает публиковать"
            : "источник больше не публикует"}
        </span>
      </span>
    );
  }
  return (
    <span className={classNames(styles["group-lifecycle"], stateStyles["is-archived"])}>
      <strong>Неактивна</strong>
      <span>нет в актуальном снимке</span>
    </span>
  );
}

function GroupLifecycleDialog({
  group,
  action,
  busy,
  onCancel,
  onConfirm,
}: {
  group: GroupView;
  action: GroupAction;
  busy: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  const deleting = action === "delete";
  const activating = action === "activate";
  return (
    <DialogPortal>
      <div className={dialogStyles["dialog-backdrop"]} role="presentation">
        <section
          className={classNames(dialogStyles["confirm-dialog"], styles["group-lifecycle-dialog"])}
          role="dialog"
          aria-modal="true"
          aria-labelledby="group-lifecycle-title"
        >
          <span
            className={deleting ? dialogStyles["dialog-danger-icon"] : dialogStyles["dialog-warning-icon"]}
          >
            {deleting ? <Trash2 size={19} /> : <AlertTriangle size={19} />}
          </span>
          <h2 id="group-lifecycle-title">
            {deleting
              ? `Удалить группу ${group.name}?`
              : activating
                ? `Включить группу ${group.name}?`
                : `Отключить группу ${group.name}?`}
          </h2>
          {deleting ? (
            <>
              <p>
                Группа отсутствует в последнем снимке источника. Она и её
                рабочие данные будут удалены без возможности восстановления.
              </p>
              <div className={styles["group-impact-summary"]}>
                <span>{number.format(group.lesson_count)} занятий</span>
                <span>{number.format(group.subscription_count)} подписок</span>
                <span>
                  {number.format(group.default_group_count)} основных групп
                </span>
                <span>{number.format(group.chat_count)} групповых чатов</span>
                <span>{number.format(group.override_count)} ручных правок</span>
              </div>
              <p className={dialogStyles["dialog-note"]}>
                Подписки будут удалены, пользователи потеряют выбор основной
                группы, а настройки связанных чатов — очищены. Исторические
                снимки и журнал аудита сохранятся.
              </p>
            </>
          ) : activating ? (
            <p>
              Ручное отключение будет снято. Группа снова появится у
              пользователей, потому что она присутствует в актуальном снимке.
            </p>
          ) : (
            <>
              <p>
                Группа исчезнет из поиска и расписания пользователей, даже если
                источник продолжит её публиковать.
              </p>
              <p className={dialogStyles["dialog-note"]}>
                Занятия, подписки, ручные правки и настройки чатов сохранятся.
                Группу можно будет включить позднее.
              </p>
            </>
          )}
          <div className={dialogStyles["dialog-actions"]}>
            <button
              className={classNames(buttonStyles["button"], buttonStyles["button-ghost"])}
              disabled={busy}
              data-dialog-dismiss
              onClick={onCancel}
            >
              Отмена
            </button>
            <button
              className={classNames(buttonStyles["button"], deleting ? buttonStyles["button-danger"] : buttonStyles["button-primary"])}
              disabled={busy}
              onClick={onConfirm}
            >
              {busy
                ? "Применение…"
                : deleting
                  ? "Удалить навсегда"
                  : activating
                    ? "Включить"
                    : "Отключить"}
            </button>
          </div>
        </section>
      </div>
    </DialogPortal>
  );
}
