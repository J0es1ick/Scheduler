import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "../api";
import {
  EmptyBlock,
  ErrorBlock,
  LoadingBlock,
  SectionTitle,
  formatDateTime,
} from "../components";
import { useRemote } from "../hooks";
import { useViewState } from "../hooks/useViewState";
import type { ServiceLogPage } from "../types";

const labels: Record<string, string> = {
  bot: "Telegram-бот",
  admin: "Административный API",
  site: "Публичный сайт",
  "parser-worker": "Парсеры",
  "privacy-worker": "Удаление данных",
  backup: "Резервные копии",
  postgres: "PostgreSQL",
  migrator: "Миграции",
  "postgres-bootstrap": "Настройка PostgreSQL",
  "log-reader": "Просмотр журналов",
  notification: "Доставка сообщений",
  reminder: "Напоминания",
  connector: "Интеграции",
  parser: "Парсинг",
  privacy: "Удаление данных",
};

function timeRange(minutes: string) {
  const until = new Date();
  return {
    since: new Date(until.getTime() - Number(minutes) * 60_000).toISOString(),
    until: until.toISOString(),
  };
}

export function ServiceLogs() {
  const [filters, setFilters] = useViewState("ServiceLogs:filters", {
    component: "",
    module: "",
    source: "",
    level: "",
    q: "",
    period: "60",
  });
  const [search, setSearch] = useState(filters.q);
  const [range, setRange] = useState(() => timeRange(filters.period));
  const [cursors, setCursors] = useState<string[]>([""]);
  const [automatic, setAutomatic] = useState(false);
  const [page, setPage] = useState<ServiceLogPage | null>(null);
  const [catalog, setCatalog] = useState<
    Pick<ServiceLogPage, "components" | "modules">
  >({ components: [], modules: [] });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const sequence = useRef(0);
  const sources = useRemote(() => api.sources(), []);
  const cursor = cursors[cursors.length - 1];
  const load = useCallback(async () => {
    const current = ++sequence.current;
    setLoading(true);
    setError("");
    try {
      const result = await api.serviceLogs({
        component: filters.component,
        module: filters.module,
        source: filters.source,
        level: filters.level,
        q: filters.q,
        ...range,
        cursor,
      });
      if (sequence.current === current) {
        setPage(result);
        setCatalog({ components: result.components, modules: result.modules });
      }
    } catch (caught) {
      if (sequence.current === current)
        setError(
          caught instanceof Error
            ? caught.message
            : "Не удалось загрузить журнал",
        );
    } finally {
      if (sequence.current === current) setLoading(false);
    }
  }, [
    filters.component,
    filters.module,
    filters.source,
    filters.level,
    filters.q,
    range,
    cursor,
  ]);

  useEffect(() => {
    const activeSequence = sequence;
    void load();
    return () => {
      activeSequence.current++;
    };
  }, [load]);

  const refresh = useCallback(() => {
    setCursors([""]);
    setRange(timeRange(filters.period));
  }, [filters.period]);

  useEffect(() => {
    if (!automatic) return;
    const timer = window.setInterval(() => {
      if (!document.hidden && !loading) refresh();
    }, 10_000);
    return () => window.clearInterval(timer);
  }, [automatic, loading, refresh]);

  function change(key: keyof typeof filters, value: string) {
    sequence.current++;
    setPage(null);
    setFilters({ ...filters, [key]: value });
    setCursors([""]);
    setRange(timeRange(key === "period" ? value : filters.period));
  }

  return (
    <section
      className="card-surface table-card service-logs"
      aria-label="Журналы компонентов"
    >
      <SectionTitle
        title="Журналы компонентов"
        action={
          <button className="text-button" disabled={loading} onClick={refresh}>
            Обновить журнал
          </button>
        }
      />
      <form
        className="service-log-filters"
        onSubmit={(event) => {
          event.preventDefault();
          change("q", search);
        }}
      >
        <label>
          Компонент
          <select
            aria-label="Компонент"
            value={filters.component}
            onChange={(event) => change("component", event.target.value)}
          >
            <option value="">Все компоненты</option>
            {catalog.components.map((item) => (
              <option key={item.name} value={item.name}>
                {labels[item.name] ?? item.name} · {item.state}
              </option>
            ))}
            {filters.component &&
              !catalog.components.some(
                (item) => item.name === filters.component,
              ) && (
                <option value={filters.component}>
                  {labels[filters.component] ?? filters.component}
                </option>
              )}
          </select>
        </label>
        <label>
          Модуль
          <select
            aria-label="Модуль"
            value={filters.module}
            onChange={(event) => change("module", event.target.value)}
          >
            <option value="">Все модули</option>
            {[
              ...new Set([
                ...catalog.modules,
                ...(filters.module ? [filters.module] : []),
              ]),
            ].map((module) => (
              <option key={module} value={module}>
                {labels[module] ?? module}
              </option>
            ))}
          </select>
        </label>
        <label>
          Парсер / источник
          <select
            aria-label="Парсер / источник"
            value={filters.source}
            onChange={(event) => change("source", event.target.value)}
          >
            <option value="">Все источники</option>
            {(sources.data ?? []).map((source) => (
              <option key={source.id} value={source.id}>
                {source.university_name} · {source.adapter_type}
              </option>
            ))}
          </select>
        </label>
        <label>
          Уровень
          <select
            aria-label="Уровень"
            value={filters.level}
            onChange={(event) => change("level", event.target.value)}
          >
            <option value="">Все уровни</option>
            {[
              ["ERROR", "Ошибки"],
              ["WARN", "Предупреждения"],
              ["INFO", "Информация"],
              ["DEBUG", "Отладка"],
              ["UNKNOWN", "Без уровня"],
            ].map(([value, label]) => (
              <option key={value} value={value}>
                {label}
              </option>
            ))}
          </select>
        </label>
        <label>
          Период
          <select
            aria-label="Период"
            value={filters.period}
            onChange={(event) => change("period", event.target.value)}
          >
            {[
              ["15", "15 минут"],
              ["60", "Час"],
              ["360", "6 часов"],
              ["1440", "Сутки"],
              ["10080", "7 дней"],
            ].map(([value, label]) => (
              <option key={value} value={value}>
                {label}
              </option>
            ))}
          </select>
        </label>
        <label className="service-log-search">
          Текст или request ID
          <input
            type="search"
            maxLength={300}
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder="Например: permission denied"
          />
        </label>
        <button className="button button-primary" type="submit">
          Найти
        </button>
      </form>
      {sources.error && (
        <ErrorBlock message={sources.error} retry={sources.reload} />
      )}
      <div className="service-log-toolbar">
        <label>
          <input
            type="checkbox"
            checked={automatic}
            onChange={(event) => {
              setAutomatic(event.target.checked);
              if (event.target.checked) refresh();
            }}
          />{" "}
          Обновлять каждые 10 секунд
        </label>
        <span>
          {page ? `Проверено: ${formatDateTime(page.checked_at)}` : ""}
          {loading && page ? " · обновление…" : ""}
        </span>
      </div>
      <p className="service-log-hint">
        Часовой пояс: {Intl.DateTimeFormat().resolvedOptions().timeZone}. Сбор
        начинается после подключения журналов. История хранится до 7 дней с
        ограничением объёма.
      </p>
      {error && <ErrorBlock message={error} retry={load} />}
      {error && page && (
        <p role="status">Ниже показаны данные последней успешной загрузки.</p>
      )}
      {page?.warnings.map((warning, index) => (
        <p
          className="service-log-warning"
          role="status"
          key={`${index}:${warning}`}
        >
          {warning}
        </p>
      ))}
      {loading && !page ? (
        <LoadingBlock rows={4} />
      ) : (
        page && (
          <>
            {page.entries.length === 0 ? (
              <EmptyBlock
                title="Записей не найдено"
                text={
                  page.next_cursor
                    ? "В просмотренной части журнала совпадений нет. Перейдите к более ранним записям."
                    : "Измените фильтры или период. Старые записи могли быть удалены при ротации."
                }
              />
            ) : (
              <div className="service-log-entries">
                {page.entries.map((entry) => (
                  <details
                    className={`service-log-entry service-log-${entry.level.toLowerCase()}`}
                    key={entry.id}
                  >
                    <summary>
                      <span className="service-log-meta">
                        <time dateTime={entry.time}>
                          {formatDateTime(entry.time)}
                        </time>
                        <strong>{entry.level}</strong>
                        <span>
                          {labels[entry.component] ?? entry.component}
                          {entry.module !== entry.component
                            ? ` / ${labels[entry.module] ?? entry.module}`
                            : ""}
                        </span>
                      </span>
                      <span className="service-log-message">
                        {entry.message}
                      </span>
                    </summary>
                    {entry.source && (
                      <p>
                        Источник:{" "}
                        {sources.data?.find(
                          (source) => source.id === entry.source,
                        )?.university_name ?? entry.source}
                      </p>
                    )}
                    <pre aria-label="Поля записи">
                      {JSON.stringify(entry.fields, null, 2)}
                    </pre>
                  </details>
                ))}
              </div>
            )}
            <div className="service-log-pagination">
              <button
                className="button button-ghost"
                disabled={loading || cursors.length === 1}
                onClick={() => {
                  setAutomatic(false);
                  setPage(null);
                  setCursors(cursors.slice(0, -1));
                }}
              >
                Позже
              </button>
              <span>
                Страница {cursors.length} · Записей: {page.entries.length}
              </span>
              <button
                className="button button-ghost"
                disabled={loading || !page.next_cursor}
                onClick={() => {
                  setAutomatic(false);
                  setPage(null);
                  setCursors([...cursors, page.next_cursor!]);
                }}
              >
                Раньше
              </button>
            </div>
          </>
        )
      )}
    </section>
  );
}
