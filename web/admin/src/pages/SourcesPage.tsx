import { useState } from "react";
import { ExternalLink, RefreshCw } from "lucide-react";
import { api } from "../api";
import {
  ErrorBlock,
  formatDateTime,
  formatDuration,
  intervalLabel,
  LoadingBlock,
  number,
  relativeTime,
  SourceGlyph,
  StatusPill,
  type ToastMessage,
} from "../components";
import { useRemote } from "../hooks";

export function SourcesPage({
  notify,
}: {
  notify: (text: string, tone?: ToastMessage["tone"]) => void;
}) {
  const { data, loading, error, reload } = useRemote(() => api.sources(), []);
  const [busy, setBusy] = useState("");
  const [intervalDrafts, setIntervalDrafts] = useState<Record<string, string>>(
    {},
  );

  async function sync(id: string) {
    setBusy(id);
    try {
      await api.syncSource(id);
      notify("Обновление запущено");
      window.setTimeout(() => void reload(), 800);
    } catch (caught) {
      notify(
        caught instanceof Error
          ? caught.message
          : "Не удалось запустить обновление",
        "error",
      );
    } finally {
      setBusy("");
    }
  }

  async function updateInterval(id: string, seconds: number) {
    setBusy(id);
    try {
      await api.updateSource(id, seconds);
      notify(`Интервал изменён: ${intervalLabel(seconds)}`);
      await reload();
    } catch (caught) {
      notify(
        caught instanceof Error
          ? caught.message
          : "Не удалось изменить интервал",
        "error",
      );
    } finally {
      setBusy("");
    }
  }

  async function commitInterval(id: string, currentSeconds: number) {
    const draft = intervalDrafts[id];
    if (draft === undefined) return;

    const minutes = Number(draft);
    if (!Number.isInteger(minutes) || minutes < 5 || minutes > 10_080) {
      notify("Интервал должен быть целым числом от 5 до 10 080 минут", "error");
      setIntervalDrafts((current) => {
        const next = { ...current };
        delete next[id];
        return next;
      });
      return;
    }

    setIntervalDrafts((current) => {
      const next = { ...current };
      delete next[id];
      return next;
    });
    const seconds = minutes * 60;
    if (seconds !== currentSeconds) await updateInterval(id, seconds);
  }

  if (loading && !data) return <LoadingBlock rows={5} />;
  if (error && !data) return <ErrorBlock message={error} retry={reload} />;

  return (
    <div className="page-stack sources-page">
      <div className="page-intro">
        <div>
          <h2>Подключённые источники</h2>
          <p>Интервал задаёт частоту полной синхронизации каждого сайта.</p>
        </div>
        <button className="button button-ghost" onClick={() => void reload()}>
          <RefreshCw size={16} /> Обновить список
        </button>
      </div>

      <section className="source-list card-surface">
        {(data ?? []).map((source) => {
          const isBusy = busy === source.id || source.running;
          const duration =
            source.latest_finished_at && source.latest_started_at
              ? formatDuration(
                  new Date(source.latest_finished_at).getTime() -
                    new Date(source.latest_started_at).getTime(),
                )
              : "—";
          return (
            <article className="source-row" key={source.id}>
              <div className="source-identity">
                <SourceGlyph name={source.university_name} />
                <div>
                  <h3>{source.university_name}</h3>
                  <p>{source.university_full_name}</p>
                  <span>{source.adapter_type}</span>
                </div>
              </div>

              <div className="source-row-stats">
                <div>
                  <span>Последний запуск</span>
                  <strong>{formatDateTime(source.last_run_at)}</strong>
                  <em>{relativeTime(source.last_run_at)}</em>
                </div>
                <div>
                  <span>Результат</span>
                  <strong>
                    {number.format(source.latest_records)} записей
                  </strong>
                  <em>{duration}</em>
                </div>
                <div>
                  <span>В базе</span>
                  <strong>{number.format(source.lesson_count)} занятий</strong>
                  <em>{number.format(source.group_count)} групп</em>
                </div>
              </div>

              <div className="source-row-actions">
                <StatusPill health={source.health} />
                <label>
                  <span>Интервал, мин</span>
                  <input
                    type="number"
                    min={5}
                    max={10_080}
                    step={1}
                    inputMode="numeric"
                    value={
                      intervalDrafts[source.id] ??
                      String(source.update_interval / 60)
                    }
                    disabled={busy === source.id}
                    aria-label={`Интервал обновления ${source.university_name} в минутах`}
                    onChange={(event) =>
                      setIntervalDrafts((current) => ({
                        ...current,
                        [source.id]: event.target.value,
                      }))
                    }
                    onBlur={() =>
                      void commitInterval(source.id, source.update_interval)
                    }
                    onKeyDown={(event) => {
                      if (event.key === "Enter") event.currentTarget.blur();
                      if (event.key === "Escape") {
                        setIntervalDrafts((current) => {
                          const next = { ...current };
                          delete next[source.id];
                          return next;
                        });
                        event.currentTarget.blur();
                      }
                    }}
                  />
                </label>
                <button
                  className="button button-primary"
                  disabled={isBusy}
                  onClick={() => void sync(source.id)}
                >
                  <RefreshCw size={16} className={isBusy ? "spin" : ""} />{" "}
                  {source.running ? "Обновляется" : "Запустить"}
                </button>
                <a href={source.schedule_url} target="_blank" rel="noreferrer">
                  Сайт расписания <ExternalLink size={13} />
                </a>
              </div>

              {source.last_error && (
                <div className="source-error">
                  <strong>Последняя ошибка</strong>
                  <span>{source.last_error}</span>
                </div>
              )}
            </article>
          );
        })}
      </section>
    </div>
  );
}
