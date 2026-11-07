import runStatusStyles from "../../ui/run-status/RunStatus.module.css";
import { formatDateTime, formatDuration, number, relativeTime } from "../../shared/lib/format";
import { classNames } from "../../../../shared/classNames";
import layoutStyles from "../../ui/layout/Layout.module.css";
import styles from "./OverviewPage.module.css";
import buttonStyles from "../../ui/button/Button.module.css";
import progressStyles from "../../ui/progress/Progress.module.css";
import stateStyles from "../../ui/state/State.module.css";
import { useEffect } from "react";
import {
  BookOpenCheck,
  CalendarClock,
  ChevronRight,
  CircleGauge,
  PencilLine,
  RefreshCw,
  UsersRound,
} from "lucide-react";
import { api } from "../../shared/api";
import { EmptyBlock, ErrorBlock, LoadingBlock, SectionTitle, SourceGlyph, StatusPill } from "../../ui";
import { useRemote } from "../../shared/hooks";
import { DismissibleNotice } from "../../ui/notice/DismissibleNotice";
import type { ViewName } from "../../app/layout/AppLayout";

export function OverviewPage({
  onNavigate,
  onParserRuns,
  canEdit = false,
  canOperate = false,
  canViewUsers = false,
}: {
  canEdit?: boolean;
  canOperate?: boolean;
  canViewUsers?: boolean;
  onNavigate: (view: ViewName) => void;
  onParserRuns: () => void;
}) {
  const { data, loading, error, reload } = useRemote(() => api.dashboard(), []);

  useEffect(() => {
    const timer = window.setInterval(() => void reload(), 15_000);
    return () => window.clearInterval(timer);
  }, [reload]);

  if (loading && !data) return <LoadingBlock rows={6} />;
  if (error && !data) return <ErrorBlock message={error} retry={reload} />;
  if (!data) return null;

  const maxRecords = Math.max(1, ...data.trend.map((point) => point.records));
  const operations = data.operations;
  const sourceSignature = operations.sources_stale + operations.sources_error + operations.sources_quarantined > 0
    ? JSON.stringify([
      operations.sources_stale, operations.sources_error, operations.sources_quarantined,
      data.sources
        .filter((source) => ["stale", "error", "quarantined", "empty"].includes(source.health))
        .map((source) => [source.id, source.health, source.last_error, source.quarantined_count])
        .sort((a, b) => String(a[0]).localeCompare(String(b[0]))),
    ])
    : null;
  const pending = operations.pending_notifications + operations.pending_outbox;
  const failed = operations.failed_notifications + operations.failed_outbox;
  const deliverySignature = pending + failed > 0
    ? JSON.stringify([operations.failed_notifications, operations.failed_outbox, operations.failed_service_updates ?? 0])
    : null;
  const daily = operations.daily_worker;
  const dailyStale = Boolean(daily?.last_finished_at &&
    Date.parse(operations.checked_at) - Date.parse(daily.last_finished_at) > 180_000);
  const dailySignature = !daily ? null : JSON.stringify([
    daily.last_error, daily.last_failures > 0, Boolean(daily.last_finished_at), dailyStale,
  ]);
  const dailyHealthy = Boolean(daily?.last_finished_at && !daily.last_error && !daily.last_failures && !dailyStale);

  return (
    <div className={classNames(layoutStyles["page-stack"], styles["overview-page"])}>
      <div className={classNames(layoutStyles["page-intro"], styles["overview-intro"])}>
        <div>
          <h2>Сводка на сегодня</h2>
          <p>
            Последние полученные данные сервиса. Проверка обновлений — каждые 15
            секунд.
          </p>
        </div>
        <div className={layoutStyles["intro-actions"]}>
          <button
            className={classNames(buttonStyles["button"], buttonStyles["button-ghost"])}
            disabled={loading}
            aria-busy={loading}
            onClick={() => void reload()}
          >
            <RefreshCw size={16} className={loading ? progressStyles["refresh-spinning"] : undefined} /> Обновить
          </button>
          <button
            className={classNames(buttonStyles["button"], buttonStyles["button-primary"])}
            disabled={!canEdit}
            onClick={() => onNavigate("editor")}
          >
            <PencilLine size={16} /> Открыть редактор
          </button>
        </div>
      </div>

      <DismissibleNotice
        id="overview-sources"
        title="Проблемы источников"
        signature={sourceSignature}
        action={
          <button
            className={classNames(buttonStyles["button"], buttonStyles["button-ghost"])}
            disabled={!canOperate}
            onClick={() => onNavigate("sources")}
          >
            Проверить источники
          </button>
        }
      >
        <span>
          Устарели: {operations.sources_stale}. Ошибки: {operations.sources_error}.
          На проверке: {operations.sources_quarantined}.
        </span>
      </DismissibleNotice>
      <DismissibleNotice
        id="overview-delivery"
        title="Доставка сообщений"
        signature={deliverySignature}
        action={
          <button
            className={classNames(buttonStyles["button"], buttonStyles["button-ghost"])}
            onClick={() => onNavigate("logs")}
          >
            Открыть диагностику
          </button>
        }
      >
        <span>
          Ожидают: {pending}. Ошибки: {failed}. Массовая отправка занимает время.
          <br />
          Рассылки: {operations.pending_service_updates ?? 0} ожидают,{" "}
          {operations.failed_service_updates ?? 0} ошибок. Запросы согласия:{" "}
          {operations.pending_service_updates_prompts ?? 0}.
        </span>
      </DismissibleNotice>
      <DismissibleNotice
        id="overview-daily"
        title="Ежедневная отправка расписания"
        signature={dailySignature}
        tone={dailyHealthy ? "neutral" : "warning"}
        icon={CalendarClock}
      >
        <span>
          {daily?.last_finished_at
            ? `Последняя проверка: ${new Date(daily.last_finished_at).toLocaleString("ru-RU")}. Обработано: ${daily.last_processed}.`
            : "Проверка ещё не выполнялась."}
          {dailyStale && " Проверка давно не выполнялась."}
          {daily?.last_error && ` Ошибка: ${daily.last_error}`}
        </span>
      </DismissibleNotice>

      <section className={styles["metric-strip"]}>
        <Metric
          icon={BookOpenCheck}
          value={number.format(data.stats.lessons)}
          label="занятий в расписании"
          note={`${number.format(data.stats.groups)} групп`}
          destination="Расписание"
          onOpen={() => onNavigate("data")}
        />
        <Metric
          icon={UsersRound}
          value={number.format(data.stats.users)}
          label="пользователей бота"
          note={`${number.format(data.stats.subscriptions)} подписок`}
          destination="Пользователи"
          onOpen={canViewUsers ? () => onNavigate("users") : undefined}
        />
        <Metric
          icon={CircleGauge}
          value={`${data.stats.success_rate}%`}
          label="успешных запусков"
          note="за 7 дней"
          destination="Запуски парсеров"
          onOpen={onParserRuns}
        />
        <Metric
          icon={CalendarClock}
          value={String(data.stats.universities)}
          label="университета"
          note="активные источники"
          destination="Источники"
          onOpen={canOperate ? () => onNavigate("sources") : undefined}
        />
      </section>

      <section className={styles["overview-main-grid"]}>
        <article className={classNames(layoutStyles["card-surface"], classNames(styles["source-overview-card"], canOperate ? styles["overview-navigable"] : undefined))}>
          {canOperate && (
            <button
              type="button"
              className={styles["overview-card-link"]}
              aria-label="Открыть источники"
              onClick={() => onNavigate("sources")}
            />
          )}
          <SectionTitle
            title="Источники"
            action={
              canOperate && <span className={styles["overview-card-hint"]} aria-hidden="true">
                Настроить <ChevronRight size={15} />
              </span>
            }
          />
          <div className={styles["source-overview-list"]}>
            {data.sources.map((source) => (
              <article key={source.id}>
                <SourceGlyph name={source.university_name} />
                <div>
                  <strong>{source.university_name}</strong>
                  <span>
                    {source.running
                      ? "Обновляется сейчас"
                      : `Последний запуск ${relativeTime(source.last_run_at)}`}
                  </span>
                  <span>
                    Публикация:{" "}
                    {source.last_published_at
                      ? relativeTime(source.last_published_at)
                      : "ещё не было"}
                  </span>
                </div>
                <div className={styles["source-overview-counts"]}>
                  <strong>{number.format(source.lesson_count)}</strong>
                  <span>занятий</span>
                </div>
                <StatusPill health={source.health} />
              </article>
            ))}
          </div>
        </article>

        <article className={classNames(layoutStyles["card-surface"], styles["activity-card"], styles["overview-navigable"])}>
          <button
            type="button"
            className={styles["overview-card-link"]}
            aria-label="Получено записей — открыть запуски парсеров"
            onClick={onParserRuns}
          />
          <SectionTitle
            eyebrow="7 дней"
            title="Получено записей"
            action={<span className={styles["overview-card-hint"]} aria-hidden="true"><ChevronRight size={18} /></span>}
          />
          <div
            className={styles["bar-chart"]}
            aria-label="Количество полученных записей по дням"
          >
            {data.trend.map((point) => {
              const height = Math.max(
                5,
                Math.round((point.records / maxRecords) * 100),
              );
              const label = new Intl.DateTimeFormat("ru-RU", {
                weekday: "short",
              }).format(new Date(point.date));
              return (
                <div
                  className={styles["bar-column"]}
                  key={point.date}
                  title={`${number.format(point.records)} записей`}
                >
                  <strong>
                    {point.records ? number.format(point.records) : ""}
                  </strong>
                  <div className={styles["bar-track"]}>
                    <i
                      className={point.failed ? stateStyles["has-error"] : undefined}
                      style={{ height: `${height}%` }}
                    />
                  </div>
                  <span>{label}</span>
                </div>
              );
            })}
          </div>
        </article>
      </section>

      <section className={classNames(styles["recent-section"], layoutStyles["card-surface"])}>
        <SectionTitle
          title="Последние запуски"
          action={
            <button className={buttonStyles["text-button"]} onClick={onParserRuns}>
              Вся история <ChevronRight size={15} />
            </button>
          }
        />
        {data.recent_logs.length === 0 ? (
          <EmptyBlock
            title="Запусков пока нет"
            text="История появится после первого обновления."
          />
        ) : (
          <div className={runStatusStyles["run-feed"]}>
            {data.recent_logs.map((log) => (
              <article key={log.id}>
                <div className={classNames(runStatusStyles["run-marker"], styles[`marker-${log.status}`])}>
                  <span />
                </div>
                <div className={runStatusStyles["run-main"]}>
                  <strong>{log.university_name}</strong>
                  <span>{formatDateTime(log.started_at)}</span>
                </div>
                <div className={runStatusStyles["run-records"]}>
                  <strong>{number.format(log.records_fetched)}</strong>
                  <span>записей</span>
                </div>
                <div className={runStatusStyles["run-duration"]}>
                  <strong>{formatDuration(log.duration_ms)}</strong>
                  <StatusPill
                    health={
                      log.status === "failed"
                        ? "failed"
                        : log.status === "quarantined"
                          ? "quarantined"
                          : log.status === "running"
                            ? "running"
                            : "success"
                    }
                  />
                </div>
              </article>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}

function Metric({
  icon: Icon,
  value,
  label,
  note,
  destination,
  onOpen,
}: {
  icon: typeof BookOpenCheck;
  value: string;
  label: string;
  note: string;
  destination: string;
  onOpen?: () => void;
}) {
  return (
    <article data-surface-light className={onOpen ? styles["overview-navigable"] : undefined}>
      {onOpen && (
        <>
          <button
            type="button"
            className={styles["overview-card-link"]}
            aria-label={`${value} ${label} — открыть раздел «${destination}»`}
            onClick={onOpen}
          />
          <ChevronRight className={styles["metric-link-arrow"]} size={16} aria-hidden="true" />
        </>
      )}
      <span className={styles["metric-icon"]}>
        <Icon size={19} />
      </span>
      <div>
        <strong key={value} className={styles["metric-value"]}>{value}</strong>
        <span>{label}</span>
      </div>
      <em>{note}</em>
    </article>
  );
}
