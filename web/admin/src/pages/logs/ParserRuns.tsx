import { formatDateTime, formatDuration, number } from "../../shared/lib/format";
import { classNames } from "../../../../shared/classNames";
import layoutStyles from "../../ui/layout/Layout.module.css";
import logsPageStyles from "./LogsPage.module.css";
import styles from "./ParserRuns.module.css";
import tableStyles from "../../ui/table/Table.module.css";
import { useViewState } from "../../shared/hooks/useViewState";
import {
  AlertTriangle,
  CheckCircle2,
  Filter,
  LoaderCircle,
} from "lucide-react";
import { api } from "../../shared/api";
import { EmptyBlock, ErrorBlock, LoadingBlock, SectionTitle, StatusPill } from "../../ui";
import { useRemote } from "../../shared/hooks";

export function ParserRuns() {
  const [source, setSource] = useViewState("LogsPage:source", "");
  const [status, setStatus] = useViewState("LogsPage:status", "");
  const sources = useRemote(() => api.sources(), []);
  const logs = useRemote(() => api.logs(source, status), [source, status]);

  const items = logs.data ?? [];
  const successful = items.filter((item) => item.status === "success").length;
  const failed = items.filter((item) => item.status === "failed").length;
  const averageDuration = items.length
    ? items.reduce((sum, item) => sum + item.duration_ms, 0) / items.length
    : 0;

  return (
    <div className={classNames(layoutStyles["page-stack"], logsPageStyles["logs-page"])}>
      <section className={styles["log-summary"]}>
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

      <section className={classNames(layoutStyles["card-surface"], tableStyles["table-card"])}>
        <SectionTitle
          title="Запуски парсеров"
          action={
            <div className={styles["filter-row"]}>
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
                <option value="quarantined">Карантин</option>
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
          <div className={classNames(tableStyles["responsive-table"], styles["logs-table"])}>
            <div className={tableStyles["table-head"]}>
              <span>Источник</span>
              <span>Начало</span>
              <span>Длительность</span>
              <span>Записи</span>
              <span>Результат</span>
            </div>
            {items.map((log) => (
              <article className={tableStyles["table-row"]} key={log.id}>
                <div data-label="Источник">
                  <i className={classNames(styles["log-dot"], styles[`log-${log.status}`])} />
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
                          : log.status === "quarantined"
                            ? "quarantined"
                            : "running"
                    }
                  />
                  {log.error_message && (
                    <span className={styles["row-error"]} title={log.error_message}>
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
