import { formatDateTime, number } from "../../shared/lib/format";
import { classNames } from "../../../../shared/classNames";
import dialogStyles from "../../ui/dialog/Dialog.module.css";
import styles from "./SnapshotReviewDialog.module.css";
import layoutStyles from "../../ui/layout/Layout.module.css";
import stateStyles from "../../ui/state/State.module.css";
import buttonStyles from "../../ui/button/Button.module.css";
import { useEffect, useMemo, useState } from "react";
import {
  ArrowRight,
  BookOpen,
  Check,
  Search,
  ShieldAlert,
  Users,
  X,
} from "lucide-react";
import { api } from "../../shared/api";
import { DialogPortal } from "../../ui";
import type {
  ParserSnapshot,
  SnapshotGroupDiff,
  SnapshotGroupStatus,
  SnapshotPreview,
  SnapshotScheduleComparison,
} from "../../shared/model";
import { hasAlternatingWeeks } from "../schedule-shared/weekSections";
import { formatSnapshotDate, snapshotGroupStatusLabels } from "./model";
import { SnapshotSchedulePanel } from "./SnapshotSchedulePanel";

type GroupFilter = "attention" | "all" | SnapshotGroupStatus;

export function SnapshotReviewDialog({
  snapshot,
  sourceName,
  approvalOnly,
  busy,
  onClose,
  onPublish,
  onReject,
}: {
  snapshot: ParserSnapshot;
  sourceName: string;
  approvalOnly: boolean;
  busy: boolean;
  onClose: () => void;
  onPublish: () => Promise<boolean>;
  onReject: () => Promise<boolean>;
}) {
  const [preview, setPreview] = useState<SnapshotPreview | null>(null);
  const [previewError, setPreviewError] = useState("");
  const [previewLoading, setPreviewLoading] = useState(true);
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<GroupFilter>("attention");
  const [selectedGroupID, setSelectedGroupID] = useState("");
  const [schedule, setSchedule] = useState<SnapshotScheduleComparison | null>(
    null,
  );
  const [scheduleError, setScheduleError] = useState("");
  const [scheduleLoading, setScheduleLoading] = useState(false);

  useEffect(() => {
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };
    window.addEventListener("keydown", closeOnEscape);
    return () => {
      document.body.style.overflow = previousOverflow;
      window.removeEventListener("keydown", closeOnEscape);
    };
  }, [onClose]);

  useEffect(() => {
    let active = true;
    setPreviewLoading(true);
    setPreviewError("");
    api
      .parserSnapshotPreview(snapshot.id)
      .then((payload) => {
        if (!active) return;
        setPreview(payload);
        const firstGroup =
          payload.groups.find((group) => group.status !== "unchanged") ??
          payload.groups[0];
        setSelectedGroupID(firstGroup?.id ?? "");
      })
      .catch((caught: unknown) => {
        if (!active) return;
        setPreviewError(
          caught instanceof Error
            ? caught.message
            : "Не удалось загрузить содержимое снимка",
        );
      })
      .finally(() => {
        if (active) setPreviewLoading(false);
      });
    return () => {
      active = false;
    };
  }, [snapshot.id]);

  useEffect(() => {
    if (!selectedGroupID) {
      setSchedule(null);
      return;
    }
    let active = true;
    setScheduleLoading(true);
    setScheduleError("");
    api
      .parserSnapshotSchedule(snapshot.id, selectedGroupID)
      .then((payload) => {
        if (active) setSchedule(payload);
      })
      .catch((caught: unknown) => {
        if (!active) return;
        setScheduleError(
          caught instanceof Error
            ? caught.message
            : "Не удалось загрузить расписание группы",
        );
      })
      .finally(() => {
        if (active) setScheduleLoading(false);
      });
    return () => {
      active = false;
    };
  }, [selectedGroupID, snapshot.id]);

  const filteredGroups = useMemo(() => {
    const normalized = query.trim().toLocaleLowerCase("ru");
    return (preview?.groups ?? []).filter((group) => {
      const matchesQuery =
        !normalized ||
        group.name.toLocaleLowerCase("ru").includes(normalized) ||
        group.id.toLocaleLowerCase("ru").includes(normalized);
      const matchesFilter =
        filter === "all" ||
        (filter === "attention"
          ? group.status !== "unchanged"
          : group.status === filter);
      return matchesQuery && matchesFilter;
    });
  }, [filter, preview?.groups, query]);

  const splitAlternatingWeeks = schedule
    ? hasAlternatingWeeks([...schedule.current, ...schedule.candidate])
    : false;

  async function publish() {
    if (await onPublish()) onClose();
  }

  async function reject() {
    if (await onReject()) onClose();
  }

  return (
    <DialogPortal>
      <div
        className={classNames(dialogStyles["dialog-backdrop"], styles["snapshot-review-backdrop"])}
        role="presentation"
      >
        <section
          className={styles["snapshot-review-dialog"]}
          role="dialog"
          aria-modal="true"
          aria-labelledby="snapshot-review-title"
        >
          <header className={styles["snapshot-review-header"]}>
            <div className={styles["snapshot-review-title"]}>
              <span className={layoutStyles["eyebrow"]}>
                {approvalOnly
                  ? "Проверка перед активацией"
                  : "Проверка перед публикацией"}
              </span>
              <h2 id="snapshot-review-title">Содержимое нового снимка</h2>
              <p>
                {sourceName} · {formatDateTime(snapshot.created_at)}
              </p>
            </div>
            <button
              className={dialogStyles["dialog-close"]}
              data-dialog-dismiss
              onClick={onClose}
              aria-label="Закрыть"
            >
              <X size={19} />
            </button>
          </header>

          <div className={styles["snapshot-review-body"]}>
            {previewLoading ? (
              <div className={styles["snapshot-review-loading"]}>
                <span className={styles["spinner"]} />
                <strong>Сравниваем снимки</strong>
                <p>Готовим список групп и отличия от опубликованной версии.</p>
              </div>
            ) : previewError || !preview ? (
              <div className={styles["snapshot-review-error"]}>
                <ShieldAlert size={24} />
                <strong>Сравнение недоступно</strong>
                <p>{previewError || "Снимок не найден"}</p>
              </div>
            ) : (
              <>
                <SnapshotSummary preview={preview} />
                <div className={styles["snapshot-review-workspace"]}>
                  <aside className={styles["snapshot-group-browser"]}>
                    <div className={styles["snapshot-group-search"]}>
                      <Search size={16} />
                      <input
                        value={query}
                        onChange={(event) => setQuery(event.target.value)}
                        placeholder="Группа или идентификатор"
                        aria-label="Поиск группы в снимке"
                      />
                    </div>
                    <div className={styles["snapshot-group-filters"]}>
                      <FilterButton
                        active={filter === "attention"}
                        label="Изменения"
                        count={
                          preview.summary.added_groups +
                          preview.summary.removed_groups +
                          preview.summary.changed_groups
                        }
                        onClick={() => setFilter("attention")}
                      />
                      <FilterButton
                        active={filter === "all"}
                        label="Все"
                        count={preview.groups.length}
                        onClick={() => setFilter("all")}
                      />
                      <FilterButton
                        active={filter === "unchanged"}
                        label="Без изменений"
                        count={preview.summary.unchanged_groups}
                        onClick={() => setFilter("unchanged")}
                      />
                    </div>
                    <div className={styles["snapshot-group-list"]}>
                      {filteredGroups.length ? (
                        filteredGroups.map((group) => (
                          <SnapshotGroupButton
                            key={group.id}
                            group={group}
                            selected={selectedGroupID === group.id}
                            onClick={() => setSelectedGroupID(group.id)}
                          />
                        ))
                      ) : (
                        <div className={styles["snapshot-group-empty"]}>
                          <Search size={19} />
                          <strong>Группы не найдены</strong>
                          <p>Измените запрос или выберите другой фильтр.</p>
                        </div>
                      )}
                    </div>
                  </aside>

                  <main className={styles["snapshot-schedule-comparison"]}>
                    {scheduleLoading ? (
                      <div className={styles["snapshot-schedule-loading"]}>
                        <span className={styles["spinner"]} /> Загружаем расписание группы
                      </div>
                    ) : scheduleError ? (
                      <div className={classNames(styles["snapshot-review-error"], stateStyles["is-compact"])}>
                        <ShieldAlert size={20} />
                        <p>{scheduleError}</p>
                      </div>
                    ) : schedule ? (
                      <>
                        <header className={styles["snapshot-comparison-heading"]}>
                          <div>
                            <span>
                              {snapshotGroupStatusLabels[schedule.status]}
                            </span>
                            <h3>{schedule.group_name}</h3>
                            <p>{schedule.group_id}</p>
                          </div>
                          <ArrowRight size={20} />
                        </header>
                        <div className={styles["snapshot-schedule-columns"]}>
                          <SnapshotSchedulePanel
                            title="Опубликовано сейчас"
                            subtitle="Последний снимок, который видят пользователи"
                            lessons={schedule.current}
                            emptyText="В опубликованной версии у группы занятий не было."
                            tone="current"
                            splitAlternatingWeeks={splitAlternatingWeeks}
                          />
                          <SnapshotSchedulePanel
                            title="Получено с сайта"
                            subtitle="Расписание, которое попадёт к пользователям"
                            lessons={schedule.candidate}
                            emptyText="Источник не вернул занятия для этой группы."
                            tone="candidate"
                            splitAlternatingWeeks={splitAlternatingWeeks}
                          />
                        </div>
                      </>
                    ) : (
                      <div className={styles["snapshot-schedule-empty-state"]}>
                        <BookOpen size={25} />
                        <strong>Выберите группу</strong>
                        <p>Здесь появится расписание до и после публикации.</p>
                      </div>
                    )}
                  </main>
                </div>
              </>
            )}
          </div>

          <footer className={styles["snapshot-review-footer"]}>
            <p>
              {approvalOnly
                ? "Одобрение не меняет рабочее расписание. Снимок будет применён вместе с активацией источника одной транзакцией."
                : "Публикация заменит данные источника. Ручные правки редактора останутся отдельным слоем."}
            </p>
            <div>
              <button
                className={classNames(buttonStyles["button"], buttonStyles["button-danger-soft"])}
                disabled={busy}
                onClick={() => void reject()}
              >
                <X size={15} /> Отклонить
              </button>
              <button
                className={classNames(buttonStyles["button"], buttonStyles["button-primary"])}
                disabled={busy || !snapshot.publishable}
                onClick={() => void publish()}
              >
                <Check size={15} />{" "}
                {approvalOnly
                  ? "Одобрить для активации"
                  : "Подтвердить публикацию"}
              </button>
            </div>
          </footer>
        </section>
      </div>
    </DialogPortal>
  );
}

function SnapshotSummary({ preview }: { preview: SnapshotPreview }) {
  return (
    <section className={styles["snapshot-review-summary"]}>
      <div className={styles["snapshot-version-comparison"]}>
        <article>
          <span>Опубликовано</span>
          <strong>{number.format(preview.current_group_count)} групп</strong>
          <p>{number.format(preview.current_lesson_count)} занятий</p>
        </article>
        <ArrowRight size={19} />
        <article className={stateStyles["is-candidate"]}>
          <span>Новый снимок</span>
          <strong>{number.format(preview.candidate_group_count)} групп</strong>
          <p>{number.format(preview.candidate_lesson_count)} занятий</p>
        </article>
        <article className={styles["snapshot-period"]}>
          <span>Период нового снимка</span>
          <strong>
            {formatSnapshotDate(preview.candidate_start_date)} —{" "}
            {formatSnapshotDate(preview.candidate_end_date)}
          </strong>
          <p>
            {preview.comparison_available
              ? "Сравнение с последней опубликованной версией"
              : "Предыдущий снимок для сравнения отсутствует"}
          </p>
        </article>
      </div>
      <div className={styles["snapshot-delta-strip"]}>
        <span className={stateStyles["is-changed"]}>
          <BookOpen size={15} /> Изменено групп:{" "}
          {preview.summary.changed_groups}
        </span>
        <span className={stateStyles["is-added"]}>
          <Users size={15} /> Новых: {preview.summary.added_groups}
        </span>
        <span className={stateStyles["is-removed"]}>
          <Users size={15} /> Исчезло: {preview.summary.removed_groups}
        </span>
        <span>
          +{number.format(preview.summary.added_lessons)} / −
          {number.format(preview.summary.removed_lessons)} занятий
        </span>
      </div>
    </section>
  );
}

function FilterButton({
  active,
  label,
  count,
  onClick,
}: {
  active: boolean;
  label: string;
  count: number;
  onClick: () => void;
}) {
  return (
    <button className={active ? stateStyles["is-active"] : undefined} onClick={onClick}>
      {label} <span>{number.format(count)}</span>
    </button>
  );
}

function SnapshotGroupButton({
  group,
  selected,
  onClick,
}: {
  group: SnapshotGroupDiff;
  selected: boolean;
  onClick: () => void;
}) {
  return (
    <button
      className={classNames(styles["snapshot-group-item"], stateStyles[`is-${group.status}`], selected ? stateStyles["is-selected"] : undefined)}
      onClick={onClick}
    >
      <div>
        <strong>{group.name}</strong>
        <span>{snapshotGroupStatusLabels[group.status]}</span>
      </div>
      <p>
        {group.current_lessons}
        <ArrowRight size={13} />
        {group.candidate_lessons}
      </p>
    </button>
  );
}
