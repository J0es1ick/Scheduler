import { useState } from "react";
import {
  ArchiveRestore,
  Check,
  Eye,
  ExternalLink,
  Info,
  PlugZap,
  Power,
  PowerOff,
  RefreshCw,
  RotateCcw,
  ShieldAlert,
  Trash2,
  X,
} from "lucide-react";
import { api } from "../api";
import {
  EmptyBlock,
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
import { SnapshotReviewDialog } from "../features/snapshot-review/SnapshotReviewDialog";
import { useRemote } from "../hooks";
import type { ParserSnapshot, SourceView } from "../types";

const adapterLabels: Record<string, string> = {
  isuct: "ISUCT",
  ispu: "ISPU",
  external_push: "Внешний коннектор",
  "managed:ivgpu": "Управляемый парсер",
  declarative_snapshot: "JSON по HTTPS",
};

export function SourcesPage({
  notify,
}: {
  notify: (text: string, tone?: ToastMessage["tone"]) => void;
}) {
  const { data, loading, error, reload } = useRemote(
    async () => ({
      sources: await api.sources(),
      snapshots: await api.parserSnapshots("", ""),
    }),
    [],
  );
  const [busy, setBusy] = useState("");
  const [intervalDrafts, setIntervalDrafts] = useState<Record<string, string>>(
    {},
  );
  const [reviewing, setReviewing] = useState<{
    snapshot: ParserSnapshot;
    sourceName: string;
  } | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<SourceView | null>(null);
  const [listView, setListView] = useState<"active" | "archived">("active");

  const allSources = data?.sources ?? [];
  const activeSources = allSources.filter(
    (source) => source.lifecycle_status !== "archived",
  );
  const archivedSources = allSources.filter(
    (source) => source.lifecycle_status === "archived",
  );
  const visibleSources = listView === "archived" ? archivedSources : activeSources;

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
      await api.updateSource(id, { update_interval: seconds });
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

  async function toggleSource(source: SourceView) {
    const nextState = !source.is_enabled;
    setBusy(source.id);
    try {
      await api.updateSource(source.id, { is_enabled: nextState });
      notify(
        nextState
          ? "Источник включён и будет обновлён ближайшим проходом воркера"
          : "Источник отключён. Опубликованное расписание сохранено",
      );
      await reload();
    } catch (caught) {
      notify(
        caught instanceof Error
          ? caught.message
          : "Не удалось изменить состояние источника",
        "error",
      );
    } finally {
      setBusy("");
    }
  }

  async function removeSource(source: SourceView) {
    setBusy(source.id);
    try {
      await api.deleteSource(source.id);
      setDeleteTarget(null);
      notify(`Источник ${source.university_name} перенесён в архив`);
      await reload();
    } catch (caught) {
      notify(
        caught instanceof Error ? caught.message : "Не удалось архивировать источник",
        "error",
      );
    } finally {
      setBusy("");
    }
  }

  async function restoreSource(source: SourceView) {
    setBusy(source.id);
    try {
      await api.restoreSource(source.id);
      notify(
        `${source.university_name} возвращён из архива. Перед автоматическим обновлением проверьте настройки и включите источник.`,
      );
      await reload();
    } catch (caught) {
      notify(
        caught instanceof Error
          ? caught.message
          : "Не удалось восстановить источник",
        "error",
      );
    } finally {
      setBusy("");
    }
  }

  async function publishSnapshot(id: string) {
    if (
      !window.confirm(
        "Опубликовать этот снимок несмотря на обнаруженные отклонения?",
      )
    ) {
      return false;
    }
    setBusy(id);
    try {
      await api.publishParserSnapshot(
        id,
        "Отклонения проверены администратором",
      );
      notify("Снимок опубликован");
      await reload();
      return true;
    } catch (caught) {
      notify(
        caught instanceof Error
          ? caught.message
          : "Не удалось опубликовать снимок",
        "error",
      );
      return false;
    } finally {
      setBusy("");
    }
  }

  async function rejectSnapshot(id: string) {
    const reason = window.prompt("Почему снимок отклонён?");
    if (!reason?.trim()) return false;
    setBusy(id);
    try {
      await api.rejectParserSnapshot(id, reason.trim());
      notify("Снимок отклонён, рабочие данные не изменены");
      await reload();
      return true;
    } catch (caught) {
      notify(
        caught instanceof Error
          ? caught.message
          : "Не удалось отклонить снимок",
        "error",
      );
      return false;
    } finally {
      setBusy("");
    }
  }

  async function rollback(id: string) {
    if (
      !window.confirm(
        "Восстановить предыдущий опубликованный снимок? Подписчики получат уведомления о фактических изменениях.",
      )
    ) {
      return;
    }
    setBusy(id);
    try {
      await api.rollbackSource(id);
      notify("Предыдущий снимок восстановлен");
      await reload();
    } catch (caught) {
      notify(
        caught instanceof Error ? caught.message : "Не удалось выполнить откат",
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

      <nav className="lifecycle-tabs" aria-label="Состояние источников">
        <button
          className={listView === "active" ? "is-active" : ""}
          type="button"
          onClick={() => setListView("active")}
        >
          Рабочие <span>{activeSources.length}</span>
        </button>
        <button
          className={listView === "archived" ? "is-active" : ""}
          type="button"
          onClick={() => setListView("archived")}
        >
          Архив <span>{archivedSources.length}</span>
        </button>
      </nav>

      <section className="source-list card-surface">
        {!visibleSources.length && (
          <EmptyBlock
            title={listView === "archived" ? "Архив пуст" : "Рабочих источников нет"}
            text={
              listView === "archived"
                ? "Архивированные источники появятся здесь и их можно будет восстановить."
                : "Подключите источник или восстановите ранее архивированный."
            }
          />
        )}
        {visibleSources.map((source) => {
          const archived = source.lifecycle_status === "archived";
          const externalConnector = source.adapter_type === "external_push";
          const reviewable =
            data?.snapshots.filter(
              (snapshot) =>
                snapshot.data_source_id === source.id &&
                (snapshot.status === "quarantined" ||
                  snapshot.status === "staged"),
            ) ?? [];
          const isBusy = busy === source.id || source.running;
          const duration =
            source.latest_finished_at && source.latest_started_at
              ? formatDuration(
                  new Date(source.latest_finished_at).getTime() -
                    new Date(source.latest_started_at).getTime(),
                )
              : "—";
          return (
            <article
              className={`source-row ${source.is_enabled ? "" : "is-disabled"} ${source.running ? "is-running" : ""} ${archived ? "is-archived" : ""}`}
              key={source.id}
            >
              <div className="source-identity">
                <SourceGlyph name={source.university_name} />
                <div>
                  <h3>{source.university_name}</h3>
                  <p>{source.university_full_name}</p>
                  {source.insecure_transport && (
                    <span className="source-transport-warning">
                      <Info size={13} /> Официальный сайт использует HTTP
                    </span>
                  )}
                  <span className="source-adapter-label">
                    {adapterLabels[source.adapter_type] ?? source.adapter_type}
                  </span>
                </div>
              </div>

              <div className="source-row-stats">
                <div>
                  <span>
                    {!source.is_enabled
                      ? "Автообновление"
                      : source.last_error
                      ? "Следующая попытка"
                      : "Последний запуск"}
                  </span>
                  <strong>
                    {!source.is_enabled
                      ? "Отключено"
                      : formatDateTime(
                          source.last_error
                            ? source.next_retry_at
                            : source.last_run_at,
                        )}
                  </strong>
                  <em>
                    {!source.is_enabled
                      ? "Данные в базе сохранены"
                      : source.last_error
                      ? `${number.format(source.consecutive_failures)} ошибок подряд`
                      : relativeTime(source.last_run_at)}
                  </em>
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
                {archived ? (
                  <>
                    <span className="source-archive-status">
                      Архивирован {formatDateTime(source.archived_at)}
                    </span>
                    <button
                      className="button button-primary"
                      disabled={busy === source.id}
                      onClick={() => void restoreSource(source)}
                    >
                      <ArchiveRestore size={15} /> Восстановить
                    </button>
                  </>
                ) : (
                  <>
                    <StatusPill health={source.health} />
                    {externalConnector ? (
                      <>
                        <div className="source-external-control">
                          <span>Обновление</span>
                          <strong>Управляется внешним коннектором</strong>
                        </div>
                        <button
                          className="button button-ghost"
                          disabled={isBusy || !source.current_snapshot_id}
                          onClick={() => void rollback(source.id)}
                        >
                          <RotateCcw size={15} /> Откатить снимок
                        </button>
                        <button
                          className="button button-primary"
                          onClick={() => {
                            window.location.hash = "/connectors";
                          }}
                        >
                          <PlugZap size={15} /> Открыть коннектор
                        </button>
                      </>
                    ) : (
                      <>
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
                              void commitInterval(
                                source.id,
                                source.update_interval,
                              )
                            }
                            onKeyDown={(event) => {
                              if (event.key === "Enter")
                                event.currentTarget.blur();
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
                          disabled={isBusy || !source.is_enabled}
                          onClick={() => void sync(source.id)}
                        >
                          <RefreshCw
                            size={16}
                            className={isBusy ? "spin" : ""}
                          />{" "}
                          {source.running ? "Обновляется" : "Запустить"}
                        </button>
                        <button
                          className="button button-ghost"
                          disabled={isBusy || !source.current_snapshot_id}
                          onClick={() => void rollback(source.id)}
                        >
                          <RotateCcw size={15} /> Откатить
                        </button>
                        <button
                          className="button button-ghost"
                          disabled={isBusy}
                          onClick={() => void toggleSource(source)}
                        >
                          {source.is_enabled ? (
                            <>
                              <PowerOff size={15} /> Отключить
                            </>
                          ) : (
                            <>
                              <Power size={15} /> Включить
                            </>
                          )}
                        </button>
                        <button
                          className="button button-danger-soft"
                          disabled={isBusy}
                          onClick={() => setDeleteTarget(source)}
                        >
                          <Trash2 size={15} /> В архив
                        </button>
                      </>
                    )}
                  </>
                )}
                {source.schedule_url && (
                  <a href={source.schedule_url} target="_blank" rel="noreferrer">
                    Сайт расписания <ExternalLink size={13} />
                  </a>
                )}
              </div>

              {!archived && source.last_error && (
                <div className="source-error">
                  <strong>Последняя ошибка</strong>
                  <span>{source.last_error}</span>
                  {source.diagnostic_id && (
                    <details className="source-diagnostic">
                      <summary>Диагностика ответа источника</summary>
                      <dl>
                        <div>
                          <dt>Категория</dt>
                          <dd>{source.diagnostic_category}</dd>
                        </div>
                        <div>
                          <dt>Получено</dt>
                          <dd>
                            {formatDateTime(source.diagnostic_created_at)}
                          </dd>
                        </div>
                        <div>
                          <dt>HTTP</dt>
                          <dd>
                            {source.diagnostic_http_status || "нет ответа"}
                          </dd>
                        </div>
                        <div>
                          <dt>Тип ответа</dt>
                          <dd>
                            {source.diagnostic_content_type || "не указан"}
                          </dd>
                        </div>
                        <div>
                          <dt>Размер</dt>
                          <dd>
                            {number.format(source.diagnostic_response_size)}{" "}
                            байт
                          </dd>
                        </div>
                        <div>
                          <dt>Совпадений</dt>
                          <dd>
                            {number.format(source.diagnostic_occurrences)}
                          </dd>
                        </div>
                        <div>
                          <dt>Контрольная группа</dt>
                          <dd>{source.diagnostic_group_id || "—"}</dd>
                        </div>
                        <div>
                          <dt>SHA-256</dt>
                          <dd className="diagnostic-hash">
                            {source.diagnostic_response_sha256 || "—"}
                          </dd>
                        </div>
                      </dl>
                      {source.diagnostic_response_preview && (
                        <pre>{source.diagnostic_response_preview}</pre>
                      )}
                    </details>
                  )}
                </div>
              )}

              {!archived && reviewable.map((snapshot) => (
                <div className="snapshot-quarantine" key={snapshot.id}>
                  <div className="snapshot-quarantine-heading">
                    <ShieldAlert size={20} />
                    <div>
                      <strong>
                        {snapshot.status === "staged"
                          ? "Тестовый снимок коннектора ожидает проверки"
                          : "Новый снимок ожидает проверки"}
                      </strong>
                      <span>
                        {formatDateTime(snapshot.created_at)} ·{" "}
                        {number.format(snapshot.group_count)} групп ·{" "}
                        {number.format(snapshot.lesson_count)} занятий
                      </span>
                    </div>
                  </div>
                  <ul>
                    {snapshot.anomaly_reasons.map((reason) => (
                      <li
                        key={`${snapshot.id}-${reason.code}-${reason.message}`}
                      >
                        {reason.message}
                      </li>
                    ))}
                  </ul>
                  {!snapshot.publishable && (
                    <p>
                      Снимок содержит структурные ошибки и не может быть
                      опубликован вручную.
                    </p>
                  )}
                  <div className="snapshot-quarantine-actions">
                    <button
                      className="button button-ghost"
                      disabled={busy === snapshot.id}
                      onClick={() =>
                        setReviewing({
                          snapshot,
                          sourceName: source.university_name,
                        })
                      }
                    >
                      <Eye size={15} /> Изучить данные
                    </button>
                    <button
                      className="button button-danger-soft"
                      disabled={busy === snapshot.id}
                      onClick={() => void rejectSnapshot(snapshot.id)}
                    >
                      <X size={15} /> Отклонить
                    </button>
                    <button
                      className="button button-primary"
                      disabled={busy === snapshot.id || !snapshot.publishable}
                      onClick={() => void publishSnapshot(snapshot.id)}
                    >
                      <Check size={15} /> Подтвердить публикацию
                    </button>
                  </div>
                </div>
              ))}
            </article>
          );
        })}
      </section>
      {reviewing && (
        <SnapshotReviewDialog
          snapshot={reviewing.snapshot}
          sourceName={reviewing.sourceName}
          busy={busy === reviewing.snapshot.id}
          onClose={() => setReviewing(null)}
          onPublish={() => publishSnapshot(reviewing.snapshot.id)}
          onReject={() => rejectSnapshot(reviewing.snapshot.id)}
        />
      )}
      {deleteTarget && (
        <DeleteSourceDialog
          source={deleteTarget}
          busy={busy === deleteTarget.id}
          onCancel={() => setDeleteTarget(null)}
          onConfirm={() => void removeSource(deleteTarget)}
        />
      )}
    </div>
  );
}

function DeleteSourceDialog({
  source,
  busy,
  onCancel,
  onConfirm,
}: {
  source: SourceView;
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
        aria-labelledby="delete-source-title"
      >
        <span className="dialog-danger-icon"><Trash2 size={19} /></span>
        <h2 id="delete-source-title">Архивировать источник {source.university_name}?</h2>
        <p>
          Источник перестанет запускаться и исчезнет из рабочего списка. Настройки,
          история запусков, диагностика и снимки сохранятся.
        </p>
        <p className="dialog-note">
          Уже опубликованные группы и занятия останутся в базе и перестанут
          автоматически обновляться. Источник можно восстановить на вкладке «Архив».
        </p>
        <div className="dialog-actions">
          <button className="button button-ghost" disabled={busy} onClick={onCancel}>
            Отмена
          </button>
          <button className="button button-danger" disabled={busy} onClick={onConfirm}>
            <Trash2 size={15} /> {busy ? "Архивация…" : "Перенести в архив"}
          </button>
        </div>
      </section>
    </div>
  );
}
