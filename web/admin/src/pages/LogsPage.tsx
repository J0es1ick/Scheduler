import {
  AlertTriangle,
  CheckCircle2,
  Filter,
  LoaderCircle,
} from "lucide-react";
import { api } from "../api";
import {
  EmptyBlock,
  ErrorBlock,
  formatDateTime,
  formatDuration,
  LoadingBlock,
  number,
  SectionTitle,
  StatusPill,
} from "../components";
import { useRemote } from "../hooks";
import { useState } from "react";

export function LogsPage() {
  const [source, setSource] = useState("");
  const [status, setStatus] = useState("");
  const sources = useRemote(() => api.sources(), []);
  const logs = useRemote(() => api.logs(source, status), [source, status]);

  const items = logs.data ?? [];
  const successful = items.filter((item) => item.status === "success").length;
  const failed = items.filter((item) => item.status === "failed").length;
  const averageDuration = items.length
    ? items.reduce((sum, item) => sum + item.duration_ms, 0) / items.length
    : 0;

  return (
    <div className="page-stack logs-page">
      <section className="log-summary">
        <article>
          <CheckCircle2 size={21} />
          <div>
            <strong>{successful}</strong>
            <span>успешных</span>
          </div>
        </article>
        <article>
          <AlertTriangle size={21} />
          <div>
            <strong>{failed}</strong>
            <span>с ошибкой</span>
          </div>
        </article>
        <article>
          <LoaderCircle size={21} />
          <div>
            <strong>{formatDuration(averageDuration)}</strong>
            <span>среднее время</span>
          </div>
        </article>
      </section>

      <section className="card-surface table-card">
        <SectionTitle
          title="Запуски парсеров"
          action={
            <div className="filter-row">
              <Filter size={16} />
              <select
                value={source}
                onChange={(event) => setSource(event.target.value)}
              >
                <option value="">Все источники</option>
                {(sources.data ?? []).map((item) => (
                  <option value={item.id} key={item.id}>
                    {item.university_name}
                  </option>
                ))}
              </select>
              <select
                value={status}
                onChange={(event) => setStatus(event.target.value)}
              >
                <option value="">Все статусы</option>
                <option value="success">Успешно</option>
                <option value="failed">Ошибка</option>
                <option value="running">Выполняется</option>
              </select>
            </div>
          }
        />
        {logs.loading && !logs.data ? (
          <LoadingBlock rows={5} />
        ) : logs.error ? (
          <ErrorBlock message={logs.error} retry={logs.reload} />
        ) : items.length === 0 ? (
          <EmptyBlock
            title="Ничего не найдено"
            text="Измените фильтры или запустите парсер."
          />
        ) : (
          <div className="responsive-table logs-table">
            <div className="table-head">
              <span>Источник</span>
              <span>Начало</span>
              <span>Длительность</span>
              <span>Записи</span>
              <span>Результат</span>
            </div>
            {items.map((log) => (
              <article className="table-row" key={log.id}>
                <div data-label="Источник">
                  <i className={`log-dot log-${log.status}`} />
                  <div>
                    <strong>{log.university_name}</strong>
                    <span>{log.data_source_id}</span>
                  </div>
                </div>
                <div data-label="Начало">
                  <strong>{formatDateTime(log.started_at)}</strong>
                  <span>{log.finished_at ? "завершён" : "в процессе"}</span>
                </div>
                <div data-label="Длительность">
                  <strong>{formatDuration(log.duration_ms)}</strong>
                </div>
                <div data-label="Записи">
                  <strong>{number.format(log.records_fetched)}</strong>
                </div>
                <div data-label="Результат">
                  <StatusPill
                    health={
                      log.status === "success"
                        ? "success"
                        : log.status === "failed"
                          ? "failed"
                          : "running"
                    }
                  />
                  {log.error_message && (
                    <span className="row-error" title={log.error_message}>
                      {log.error_message}
                    </span>
                  )}
                </div>
              </article>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}
