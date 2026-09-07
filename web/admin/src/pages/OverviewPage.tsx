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
import { api } from "../api";
import {
  EmptyBlock,
  ErrorBlock,
  formatDateTime,
  formatDuration,
  LoadingBlock,
  number,
  relativeTime,
  SectionTitle,
  SourceGlyph,
  StatusPill,
} from "../components";
import { useRemote } from "../hooks";
import type { ViewName } from "../app/layout/AppLayout";

export function OverviewPage({
  onNavigate,
  canEdit = false,
  canOperate = false,
}: {
  canEdit?: boolean;
  canOperate?: boolean;
  onNavigate: (view: ViewName) => void;
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

  return (
    <div className="page-stack overview-page">
      <div className="page-intro overview-intro">
        <div>
          <h2>Сводка на сегодня</h2>
          <p>
            Последние полученные данные сервиса. Проверка обновлений — каждые 15
            секунд.
          </p>
        </div>
        <div className="intro-actions">
          <button className="button button-ghost" onClick={() => void reload()}>
            <RefreshCw size={16} /> Обновить
          </button>
          <button
            className="button button-primary"
            disabled={!canEdit}
            onClick={() => onNavigate("editor")}
          >
            <PencilLine size={16} /> Открыть редактор
          </button>
        </div>
      </div>

      {data.operations.sources_stale +
        data.operations.sources_error +
        data.operations.sources_quarantined >
        0 && (
        <section className="operations-warning">
          <CircleGauge size={20} />
          <div>
            <strong>Проблемы источников</strong>
            <span>
              Устарели: {data.operations.sources_stale}. Ошибки:{" "}
              {data.operations.sources_error}. На проверке:{" "}
              {data.operations.sources_quarantined}.
            </span>
          </div>
          <button
            className="button button-ghost"
            disabled={!canOperate}
            onClick={() => onNavigate("sources")}
          >
            Проверить источники
          </button>
        </section>
      )}
      {data.operations.pending_notifications +
        data.operations.pending_outbox +
        data.operations.failed_notifications +
        data.operations.failed_outbox >
        0 && (
        <section className="operations-warning">
          <CircleGauge size={20} />
          <div>
            <strong>Доставка сообщений</strong>
            <span>
              Ожидают:{" "}
              {data.operations.pending_notifications +
                data.operations.pending_outbox}
              . Ошибки:{" "}
              {data.operations.failed_notifications +
                data.operations.failed_outbox}
              . Массовая отправка занимает время.
            </span>
          </div>
          <button
            className="button button-ghost"
            onClick={() => onNavigate("logs")}
          >
            Открыть диагностику
          </button>
        </section>
      )}

      <section className="metric-strip">
        <Metric
          icon={BookOpenCheck}
          value={number.format(data.stats.lessons)}
          label="занятий в расписании"
          note={`${number.format(data.stats.groups)} групп`}
        />
        <Metric
          icon={UsersRound}
          value={number.format(data.stats.users)}
          label="пользователей бота"
          note={`${number.format(data.stats.subscriptions)} подписок`}
        />
        <Metric
          icon={CircleGauge}
          value={`${data.stats.success_rate}%`}
          label="успешных запусков"
          note="за 7 дней"
        />
        <Metric
          icon={CalendarClock}
          value={String(data.stats.universities)}
          label="университета"
          note="активные источники"
        />
      </section>

      <section className="overview-main-grid">
        <article className="card-surface source-overview-card">
          <SectionTitle
            title="Источники"
            action={
              <button
                className="text-button"
                disabled={!canOperate}
                onClick={() => onNavigate("sources")}
              >
                Настроить <ChevronRight size={15} />
              </button>
            }
          />
          <div className="source-overview-list">
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
                <div className="source-overview-counts">
                  <strong>{number.format(source.lesson_count)}</strong>
                  <span>занятий</span>
                </div>
                <StatusPill health={source.health} />
              </article>
            ))}
          </div>
        </article>

        <article className="card-surface activity-card">
          <SectionTitle eyebrow="7 дней" title="Получено записей" />
          <div
            className="bar-chart"
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
                  className="bar-column"
                  key={point.date}
                  title={`${number.format(point.records)} записей`}
                >
                  <strong>
                    {point.records ? number.format(point.records) : ""}
                  </strong>
                  <div className="bar-track">
                    <i
                      className={point.failed ? "has-error" : ""}
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

      <section className="recent-section card-surface">
        <SectionTitle
          title="Последние запуски"
          action={
            <button className="text-button" onClick={() => onNavigate("logs")}>
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
          <div className="run-feed">
            {data.recent_logs.map((log) => (
              <article key={log.id}>
                <div className={`run-marker marker-${log.status}`}>
                  <span />
                </div>
                <div className="run-main">
                  <strong>{log.university_name}</strong>
                  <span>{formatDateTime(log.started_at)}</span>
                </div>
                <div className="run-records">
                  <strong>{number.format(log.records_fetched)}</strong>
                  <span>записей</span>
                </div>
                <div className="run-duration">
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
}: {
  icon: typeof BookOpenCheck;
  value: string;
  label: string;
  note: string;
}) {
  return (
    <article>
      <span className="metric-icon">
        <Icon size={19} />
      </span>
      <div>
        <strong>{value}</strong>
        <span>{label}</span>
      </div>
      <em>{note}</em>
    </article>
  );
}
