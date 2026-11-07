import runStatusStyles from "../../ui/run-status/RunStatus.module.css";
import { formatDateTime } from "../../shared/lib/format";
import { classNames } from "../../../../shared/classNames";
import layoutStyles from "../../ui/layout/Layout.module.css";
import styles from "./ConnectorsPage.module.css";
import buttonStyles from "../../ui/button/Button.module.css";
import tabsStyles from "../../ui/tabs/Tabs.module.css";
import stateStyles from "../../ui/state/State.module.css";
import tableStyles from "../../ui/table/Table.module.css";
import statusPillStyles from "../../ui/status/StatusPill.module.css";
import dialogStyles from "../../ui/dialog/Dialog.module.css";
import formStyles from "../../ui/form/Form.module.css";
import { useViewState } from "../../shared/hooks/useViewState";
import { useUnsavedChanges } from "../../shared/hooks/useUnsavedChanges";
import { useMemo, useState } from "react";
import {
  ArrowLeft,
  ArrowRight,
  Braces,
  Check,
  Clipboard,
  CloudDownload,
  ExternalLink,
  FileJson,
  KeyRound,
  Pause,
  Play,
  PlugZap,
  Plus,
  RefreshCw,
  Send,
  ServerCog,
  ShieldCheck,
  SlidersHorizontal,
  TestTube2,
  X,
} from "lucide-react";
import { api } from "../../shared/api";
import { DialogPortal, EmptyBlock, ErrorBlock, LoadingBlock, SectionTitle, type ToastMessage } from "../../ui";
import { useRemote } from "../../shared/hooks";
import type {
  ConnectorClient,
  ConnectorCredentials,
  ConnectorRun,
  ConnectorStatus,
  IntegrationMode,
  ManagedParserCatalogItem,
  SourceQualityPolicy,
  SnapshotPreview,
} from "../../shared/model";

const statusLabels: Record<ConnectorStatus, string> = {
  draft: "Черновик",
  testing: "Тестирование",
  pending_review: "Ожидает проверки",
  active: "Активна",
  suspended: "Приостановлена",
  archived: "Архив",
};

const modeLabels: Record<IntegrationMode, string> = {
  managed_parser: "Управляемый парсер",
  declarative_pull: "JSON по HTTPS",
  external_push: "Внешний сервер",
};

type Draft = {
  integration_mode: IntegrationMode;
  parser_id: string;
  declarative_url: string;
  update_interval: number;
  university_id: string;
  university_name: string;
  university_full_name: string;
  schedule_url: string;
  timezone: string;
  locale: string;
  display_name: string;
  description: string;
  maintainer_name: string;
  maintainer_url: string;
};

const emptyDraft: Draft = {
  integration_mode: "managed_parser",
  parser_id: "",
  declarative_url: "",
  update_interval: 3600,
  university_id: "",
  university_name: "",
  university_full_name: "",
  schedule_url: "",
  timezone: "Europe/Moscow",
  locale: "ru-RU",
  display_name: "",
  description: "",
  maintainer_name: "",
  maintainer_url: "",
};

export function ConnectorsPage({
  notify,
}: {
  notify: (text: string, tone?: ToastMessage["tone"]) => void;
}) {
  const connectors = useRemote(() => api.connectors(), []);
  const catalog = useRemote(() => api.connectorCatalog(), []);
  const [activation, setActivation] = useState<{
    item: ConnectorClient;
    preview: SnapshotPreview;
  } | null>(null);
  const [wizardOpen, setWizardOpen] = useState(false);
  const [credentials, setCredentials] = useState<ConnectorCredentials | null>(
    null,
  );
  const [runs, setRuns] = useState<Record<string, ConnectorRun[]>>({});
  const [expanded, setExpanded] = useState("");
  const [busy, setBusy] = useState("");
  const [policyTarget, setPolicyTarget] = useState<ConnectorClient | null>(
    null,
  );
  const [listView, setListView] = useViewState<
    "working" | "draft" | "archived"
  >("connectors:list", "working");

  const allConnectors = connectors.data ?? [];
  const draftConnectors = allConnectors.filter(
    (item) => item.status === "draft",
  );
  const archivedConnectors = allConnectors.filter(
    (item) => item.status === "archived",
  );
  const workingConnectors = allConnectors.filter(
    (item) => item.status !== "draft" && item.status !== "archived",
  );
  const visibleConnectors =
    listView === "draft"
      ? draftConnectors
      : listView === "archived"
        ? archivedConnectors
        : workingConnectors;

  async function reloadAll() {
    await Promise.all([connectors.reload(), catalog.reload()]);
  }

  async function transition(
    item: ConnectorClient,
    status: ConnectorStatus,
    preview?: SnapshotPreview,
  ) {
    setBusy(item.id);
    try {
      if (status === "active" && !preview) {
        setActivation({
          item,
          preview: await api.connectorActivation(item.id),
        });
        return;
      }
      await api.updateConnector(item.id, {
        status,
        ...(preview
          ? {
              snapshot_id: preview.snapshot_id,
              expected_current_snapshot_id: preview.current_snapshot_id,
            }
          : {}),
      });
      setActivation(null);
      notify(`Состояние изменено: ${statusLabels[status]}`);
      await reloadAll();
      if (expanded === item.id && item.integration_mode === "external_push")
        await loadRuns(item.id);
    } catch (error) {
      notify(
        error instanceof Error
          ? error.message
          : "Не удалось изменить состояние",
        "error",
      );
    } finally {
      setBusy("");
    }
  }

  async function runTest(item: ConnectorClient) {
    setBusy(item.id);
    try {
      await api.syncSource(item.data_source_id);
      notify("Тестовый запуск начат. Снимок появится в разделе «Источники»");
    } catch (error) {
      notify(
        error instanceof Error ? error.message : "Не удалось запустить тест",
        "error",
      );
    } finally {
      setBusy("");
    }
  }

  async function loadRuns(id: string) {
    try {
      const items = await api.connectorRuns(id);
      setRuns((current) => ({ ...current, [id]: items }));
    } catch (error) {
      notify(
        error instanceof Error
          ? error.message
          : "Не удалось загрузить отправки",
        "error",
      );
    }
  }

  async function toggleRuns(id: string) {
    const next = expanded === id ? "" : id;
    setExpanded(next);
    if (next && !runs[id]) await loadRuns(id);
  }

  async function rotateKey(id: string) {
    if (!window.confirm("Старый ключ перестанет работать сразу. Продолжить?"))
      return;
    setBusy(id);
    try {
      const result = await api.rotateConnectorKey(id);
      setCredentials(result.credentials);
      notify("Ключ заменён. Сохраните новый закрытый ключ");
      await reloadAll();
    } catch (error) {
      notify(
        error instanceof Error ? error.message : "Не удалось заменить ключ",
        "error",
      );
    } finally {
      setBusy("");
    }
  }

  return (
    <div className={classNames(layoutStyles["page-stack"], styles["connectors-page"])}>
      <div className={layoutStyles["page-intro"]}>
        <div>
          <h2>Интеграции расписаний</h2>
          <p>
            Scheduler может сам запускать принятый парсер, забирать готовый JSON
            или принимать подписанные снимки от внешнего сервера.
          </p>
        </div>
        <button
          className={classNames(buttonStyles["button"], buttonStyles["button-primary"])}
          onClick={() => setWizardOpen(true)}
        >
          <Plus size={16} /> Подключить источник
        </button>
      </div>

      <div className={styles["integration-mode-summary"]}>
        <article>
          <ServerCog size={19} />
          <div>
            <strong>Управляемый парсер</strong>
            <span>
              Основной путь: код проходит review, запуск и мониторинг берёт на
              себя Scheduler.
            </span>
          </div>
        </article>
        <article>
          <FileJson size={19} />
          <div>
            <strong>JSON по HTTPS</strong>
            <span>
              Без кода, если источник уже отдаёт Schedule Snapshot v1.
            </span>
          </div>
        </article>
        <article>
          <CloudDownload size={19} />
          <div>
            <strong>Внешний сервер</strong>
            <span>
              Для организаций, которые хотят самостоятельно отправлять
              подписанные снимки.
            </span>
          </div>
        </article>
      </div>

      <nav className={tabsStyles["lifecycle-tabs"]} aria-label="Состояние интеграций">
        <button
          className={listView === "working" ? stateStyles["is-active"] : undefined}
          onClick={() => setListView("working")}
        >
          В работе <span>{workingConnectors.length}</span>
        </button>
        <button
          className={listView === "draft" ? stateStyles["is-active"] : undefined}
          onClick={() => setListView("draft")}
        >
          Черновики <span>{draftConnectors.length}</span>
        </button>
        <button
          className={listView === "archived" ? stateStyles["is-active"] : undefined}
          onClick={() => setListView("archived")}
        >
          Архив <span>{archivedConnectors.length}</span>
        </button>
      </nav>

      <section className={classNames(layoutStyles["card-surface"], tableStyles["table-card"])}>
        <SectionTitle
          title={
            listView === "draft"
              ? "Черновики интеграций"
              : listView === "archived"
                ? "Архив интеграций"
                : "Интеграции в работе"
          }
        />
        {connectors.loading && !connectors.data ? (
          <LoadingBlock rows={4} />
        ) : connectors.error ? (
          <ErrorBlock message={connectors.error} retry={connectors.reload} />
        ) : !visibleConnectors.length ? (
          <EmptyBlock
            title={
              listView === "draft"
                ? "Черновиков нет"
                : listView === "archived"
                  ? "Архив пуст"
                  : "Рабочих интеграций пока нет"
            }
            text="Мастер предложит подходящий способ подключения и создаст тестовый контур."
          />
        ) : (
          <div className={styles["connector-list"]}>
            {visibleConnectors.map((item) => (
              <article className={styles["connector-card"]} key={item.id}>
                <header>
                  <span className={styles["connector-icon"]}>
                    {item.integration_mode === "managed_parser" ? (
                      <ServerCog size={19} />
                    ) : item.integration_mode === "declarative_pull" ? (
                      <FileJson size={19} />
                    ) : (
                      <PlugZap size={19} />
                    )}
                  </span>
                  <div>
                    <h3>{item.display_name}</h3>
                    <p>
                      {item.university_name} · {item.university_id}
                    </p>
                  </div>
                  <span className={classNames(styles["connector-status"], statusPillStyles[`status-${item.status}`])}>
                    {statusLabels[item.status]}
                  </span>
                </header>
                <div className={styles["integration-mode-badge"]}>
                  {modeLabels[item.integration_mode]}
                </div>
                {item.description && (
                  <p className={styles["connector-description"]}>{item.description}</p>
                )}
                <div className={styles["connector-facts"]}>
                  <span>
                    <strong>
                      {item.integration_mode === "external_push"
                        ? "Последний запрос"
                        : "Последний запуск"}
                    </strong>
                    {formatDateTime(item.last_seen_at)}
                  </span>
                  <span>
                    <strong>Последний снимок</strong>
                    {formatDateTime(item.last_snapshot_at)}
                  </span>
                  {item.integration_mode === "external_push" ? (
                    <>
                      <span>
                        <strong>Ключ</strong>
                        {item.key_id.slice(0, 18)}…
                      </span>
                      <span>
                        <strong>Лимит</strong>
                        {item.rate_limit_per_minute} запросов/мин.
                      </span>
                    </>
                  ) : (
                    <span>
                      <strong>Исполнение</strong>в инфраструктуре Scheduler
                    </span>
                  )}
                </div>
                <footer>
                  {item.integration_mode === "external_push" ? (
                    <button
                      className={classNames(buttonStyles["button"], buttonStyles["button-ghost"])}
                      onClick={() => void toggleRuns(item.id)}
                    >
                      <RefreshCw size={15} />{" "}
                      {expanded === item.id ? "Скрыть отправки" : "Отправки"}
                    </button>
                  ) : item.status === "testing" ||
                    item.status === "pending_review" ? (
                    <button
                      className={classNames(buttonStyles["button"], buttonStyles["button-ghost"])}
                      disabled={busy === item.id}
                      onClick={() => void runTest(item)}
                    >
                      <TestTube2 size={15} /> Запустить тест
                    </button>
                  ) : null}
                  {(item.status === "testing" ||
                    item.status === "pending_review") && (
                    <button
                      className={classNames(buttonStyles["button"], buttonStyles["button-ghost"])}
                      onClick={() => {
                        window.location.hash = "/sources";
                      }}
                    >
                      <ExternalLink size={15} /> Проверить снимок
                    </button>
                  )}
                  {item.integration_mode === "external_push" &&
                    item.status !== "archived" && (
                      <button
                        className={classNames(buttonStyles["button"], buttonStyles["button-ghost"])}
                        disabled={busy === item.id}
                        onClick={() => void rotateKey(item.id)}
                      >
                        <KeyRound size={15} /> Сменить ключ
                      </button>
                    )}
                  <button
                    className={classNames(buttonStyles["button"], buttonStyles["button-ghost"])}
                    disabled={busy === item.id}
                    onClick={() => setPolicyTarget(item)}
                  >
                    <SlidersHorizontal size={15} /> Правила качества
                  </button>
                  <ConnectorActions
                    item={item}
                    busy={busy === item.id}
                    onTransition={(status) => void transition(item, status)}
                  />
                </footer>
                {expanded === item.id &&
                  item.integration_mode === "external_push" && (
                    <RunsList items={runs[item.id]} loading={!runs[item.id]} />
                  )}
              </article>
            ))}
          </div>
        )}
      </section>

      {activation && (
        <DialogPortal>
          <div className={dialogStyles["dialog-backdrop"]}>
            <section
              className={styles["dialog"]}
              role="dialog"
              aria-modal="true"
              aria-labelledby="activation-title"
            >
              <header className={dialogStyles["dialog-header"]}>
                <h2 id="activation-title">Активировать и опубликовать</h2>
                <button
                  data-dialog-dismiss
                  aria-label="Закрыть"
                  disabled={!!busy}
                  onClick={() => setActivation(null)}
                >
                  <X size={20} />
                </button>
              </header>
              <div className={dialogStyles["dialog-body"]}>
                <p>
                  После подтверждения расписание сразу станет видно в боте.
                  Одобрение снимка само по себе не включает источник.
                </p>
                <p>
                  Заменяемый источник:{" "}
                  {activation.preview.current_data_source_id ||
                    "первая публикация"}
                </p>
                <table>
                  <thead>
                    <tr>
                      <th>Настройка</th>
                      <th>Сейчас</th>
                      <th>После публикации</th>
                    </tr>
                  </thead>
                  <tbody>
                    {(
                      [
                        ["name", "Название"],
                        ["timezone", "Часовой пояс"],
                        ["schedule_url", "Сайт расписания"],
                        ["locale", "Язык"],
                      ] as const
                    ).map(([key, label]) => (
                      <tr key={key}>
                        <th>{label}</th>
                        <td>
                          {activation.preview.current_institution[key] || "—"}
                        </td>
                        <td>
                          {activation.preview.candidate_institution[key] || "—"}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
                <p>
                  Группы: +{activation.preview.summary.added_groups}, −
                  {activation.preview.summary.removed_groups}, изменено{" "}
                  {activation.preview.summary.changed_groups}. Занятий:{" "}
                  {activation.preview.candidate_lesson_count}.
                </p>
                <p>
                  Снимок: {activation.preview.snapshot_id}. Если действующее
                  расписание изменится, потребуется новое сравнение.
                </p>
              </div>
              <footer className={dialogStyles["dialog-footer"]}>
                <button
                  className={classNames(buttonStyles["button"], buttonStyles["button-secondary"])}
                  data-dialog-dismiss
                  disabled={!!busy}
                  onClick={() => setActivation(null)}
                >
                  Отмена
                </button>
                <button
                  className={classNames(buttonStyles["button"], buttonStyles["button-primary"])}
                  disabled={!!busy}
                  onClick={() =>
                    void transition(
                      activation.item,
                      "active",
                      activation.preview,
                    )
                  }
                >
                  Активировать и опубликовать
                </button>
              </footer>
            </section>
          </div>
        </DialogPortal>
      )}
      {wizardOpen && (
        <IntegrationWizard
          catalog={catalog.data ?? []}
          catalogLoading={catalog.loading}
          notify={notify}
          onClose={() => setWizardOpen(false)}
          onCreated={async (value) => {
            setWizardOpen(false);
            if (value) setCredentials(value);
            await reloadAll();
          }}
        />
      )}
      {credentials && (
        <CredentialsDialog
          value={credentials}
          onClose={() => setCredentials(null)}
          notify={notify}
        />
      )}
      {policyTarget && (
        <QualityPolicyDialog
          connector={policyTarget}
          onClose={() => setPolicyTarget(null)}
          onSaved={async () => {
            setPolicyTarget(null);
            await reloadAll();
          }}
          notify={notify}
        />
      )}
    </div>
  );
}

function ConnectorActions({
  item,
  busy,
  onTransition,
}: {
  item: ConnectorClient;
  busy: boolean;
  onTransition: (status: ConnectorStatus) => void;
}) {
  if (item.status === "draft")
    return (
      <>
        <button
          className={classNames(buttonStyles["button"], buttonStyles["button-primary"])}
          disabled={busy}
          onClick={() => onTransition("testing")}
        >
          <TestTube2 size={15} /> Начать тестирование
        </button>
        <button
          className={classNames(buttonStyles["button"], buttonStyles["button-danger-soft"])}
          disabled={busy}
          onClick={() => onTransition("archived")}
        >
          <Pause size={15} /> В архив
        </button>
      </>
    );
  if (item.status === "testing")
    return (
      <>
        <button
          className={classNames(buttonStyles["button"], buttonStyles["button-primary"])}
          disabled={busy}
          onClick={() => onTransition("pending_review")}
        >
          <Send size={15} /> Передать на проверку
        </button>
        <button
          className={classNames(buttonStyles["button"], buttonStyles["button-ghost"])}
          disabled={busy}
          onClick={() => onTransition("draft")}
        >
          <ArrowLeft size={15} /> В черновик
        </button>
      </>
    );
  if (item.status === "pending_review")
    return (
      <>
        <button
          className={classNames(buttonStyles["button"], buttonStyles["button-primary"])}
          disabled={busy}
          onClick={() => onTransition("active")}
        >
          <ShieldCheck size={15} /> Активировать проверенный снимок
        </button>
        <button
          className={classNames(buttonStyles["button"], buttonStyles["button-ghost"])}
          disabled={busy}
          onClick={() => onTransition("testing")}
        >
          <ArrowLeft size={15} /> На доработку
        </button>
      </>
    );
  if (item.status === "active")
    return (
      <button
        className={classNames(buttonStyles["button"], buttonStyles["button-danger-soft"])}
        disabled={busy}
        onClick={() => onTransition("suspended")}
      >
        <Pause size={15} /> Приостановить
      </button>
    );
  if (item.status === "suspended")
    return (
      <>
        <button
          className={classNames(buttonStyles["button"], buttonStyles["button-primary"])}
          disabled={busy}
          onClick={() => onTransition("testing")}
        >
          <Play size={15} /> Вернуть в тестирование
        </button>
        <button
          className={classNames(buttonStyles["button"], buttonStyles["button-danger-soft"])}
          disabled={busy}
          onClick={() => onTransition("archived")}
        >
          <Pause size={15} /> В архив
        </button>
      </>
    );
  if (item.status === "archived")
    return (
      <button
        className={classNames(buttonStyles["button"], buttonStyles["button-primary"])}
        disabled={busy}
        onClick={() => onTransition("draft")}
      >
        <ArrowLeft size={15} /> Вернуть в черновики
      </button>
    );
  return null;
}

function RunsList({
  items,
  loading,
}: {
  items?: ConnectorRun[];
  loading: boolean;
}) {
  if (loading) return <LoadingBlock rows={2} />;
  if (!items?.length)
    return (
      <div className={styles["connector-empty-run"]}>Снимки ещё не отправлялись.</div>
    );
  return (
    <div className={styles["connector-runs"]}>
      {items.slice(0, 10).map((run) => (
        <div key={run.run_id}>
          <span
            className={classNames(runStatusStyles["run-dot"], runStatusStyles[`run-${run.status === "superseded" ? "Заменён новой публикацией" : run.status}`])}
          />
          <div>
            <strong>{run.external_snapshot_id}</strong>
            <span>
              {formatDateTime(run.received_at)} · {run.group_count} групп ·{" "}
              {run.lesson_count} занятий
            </span>
            {run.error && <p>{run.error}</p>}
          </div>
          <b>
            {run.status === "superseded"
              ? "Заменён новой публикацией"
              : run.status}
          </b>
        </div>
      ))}
    </div>
  );
}

function IntegrationWizard({
  catalog,
  catalogLoading,
  onClose,
  onCreated,
  notify,
}: {
  catalog: ManagedParserCatalogItem[];
  catalogLoading: boolean;
  onClose: () => void;
  onCreated: (credentials?: ConnectorCredentials) => Promise<void>;
  notify: (text: string, tone?: ToastMessage["tone"]) => void;
}) {
  const [step, setStep] = useState(1);
  const [draft, setDraft] = useState<Draft>(emptyDraft);
  const confirmClose = useUnsavedChanges(
    JSON.stringify(draft) !== JSON.stringify(emptyDraft),
  );
  const close = () => {
    if (confirmClose()) onClose();
  };
  const [busy, setBusy] = useState(false);
  const selected = catalog.find(
    (item) => item.manifest.parser_id === draft.parser_id,
  );
  const validInstitution = Boolean(
    draft.university_id && draft.university_name && draft.display_name,
  );
  const canContinue =
    step === 1
      ? true
      : step === 2
        ? draft.integration_mode !== "managed_parser" ||
          Boolean(selected && !selected.connected)
        : step === 3
          ? draft.integration_mode === "managed_parser"
            ? Boolean(selected)
            : validInstitution &&
              (draft.integration_mode !== "declarative_pull" ||
                Boolean(draft.declarative_url))
          : true;
  const rows = useMemo(
    () => [
      ["Способ", modeLabels[draft.integration_mode]],
      [
        "Учебное заведение",
        selected?.manifest.institution.name ??
          `${draft.university_name} (${draft.university_id})`,
      ],
      [
        "Обновление",
        `каждые ${Math.round((selected?.manifest.update_interval ?? draft.update_interval) / 60)} мин.`,
      ],
      [
        "Исполнение",
        draft.integration_mode === "external_push"
          ? "на стороне владельца"
          : "в Scheduler",
      ],
    ],
    [draft, selected],
  );

  function field(name: keyof Draft, label: string, placeholder = "") {
    return (
      <label>
        <span>{label}</span>
        <input
          value={String(draft[name])}
          placeholder={placeholder}
          onChange={(event) =>
            setDraft((current) => ({
              ...current,
              [name]:
                name === "update_interval"
                  ? Number(event.target.value)
                  : event.target.value,
            }))
          }
        />
      </label>
    );
  }

  async function create() {
    setBusy(true);
    try {
      const result = await api.createConnector(draft);
      notify(
        draft.integration_mode === "external_push"
          ? "Интеграция создана. Сохраните закрытый ключ"
          : "Интеграция создана в черновиках",
      );
      await onCreated(result.credentials);
    } catch (error) {
      notify(
        error instanceof Error
          ? error.message
          : "Не удалось создать интеграцию",
        "error",
      );
    } finally {
      setBusy(false);
    }
  }

  return (
    <DialogPortal>
      <div className={dialogStyles["dialog-backdrop"]} role="presentation">
        <section
          className={styles["connector-wizard"]}
          role="dialog"
          aria-modal="true"
          aria-labelledby="connector-wizard-title"
        >
          <header>
            <div>
              <span>Шаг {step} из 4</span>
              <h2 id="connector-wizard-title">Подключение расписания</h2>
            </div>
            <button data-dialog-dismiss onClick={close} aria-label="Закрыть">
              <X size={19} />
            </button>
          </header>
          <div className={classNames(styles["wizard-progress"], styles["wizard-progress-four"])}>
            <i className={step >= 1 ? stateStyles["active"] : undefined} />
            <i className={step >= 2 ? stateStyles["active"] : undefined} />
            <i className={step >= 3 ? stateStyles["active"] : undefined} />
            <i className={step >= 4 ? stateStyles["active"] : undefined} />
          </div>
          {step === 1 && (
            <div className={styles["wizard-fields"]}>
              <p>Выберите, кто будет запускать получение расписания.</p>
              <div className={styles["integration-mode-picker"]}>
                <button
                  className={draft.integration_mode === "managed_parser" ? stateStyles["is-selected"] : undefined}
                  onClick={() =>
                    setDraft((current) => ({
                      ...current,
                      integration_mode: "managed_parser",
                    }))
                  }
                >
                  <ServerCog size={22} />
                  <strong>Управляемый парсер</strong>
                  <span>
                    Рекомендуется. Автор передаёт код, Scheduler запускает его
                    сам.
                  </span>
                </button>
                <button
                  className={draft.integration_mode === "declarative_pull" ? stateStyles["is-selected"] : undefined}
                  onClick={() =>
                    setDraft((current) => ({
                      ...current,
                      integration_mode: "declarative_pull",
                    }))
                  }
                >
                  <FileJson size={22} />
                  <strong>JSON по HTTPS</strong>
                  <span>Источник уже отдаёт полный Schedule Snapshot v1.</span>
                </button>
                <button
                  className={draft.integration_mode === "external_push" ? stateStyles["is-selected"] : undefined}
                  onClick={() =>
                    setDraft((current) => ({
                      ...current,
                      integration_mode: "external_push",
                    }))
                  }
                >
                  <CloudDownload size={22} />
                  <strong>Внешний сервер</strong>
                  <span>
                    Владелец сам запускает парсер и отправляет снимки.
                  </span>
                </button>
              </div>
            </div>
          )}
          {step === 2 && (
            <div className={styles["wizard-fields"]}>
              {draft.integration_mode === "managed_parser" ? (
                <>
                  <p>
                    Выберите парсер, уже прошедший review и включённый в эту
                    сборку Scheduler.
                  </p>
                  {catalogLoading ? (
                    <LoadingBlock rows={2} />
                  ) : (
                    <div className={styles["managed-parser-picker"]}>
                      {catalog.map((item) => (
                        <button
                          disabled={item.connected}
                          className={draft.parser_id === item.manifest.parser_id ? stateStyles["is-selected"] : undefined}
                          key={item.manifest.parser_id}
                          onClick={() =>
                            setDraft((current) => ({
                              ...current,
                              parser_id: item.manifest.parser_id,
                            }))
                          }
                        >
                          <Braces size={19} />
                          <span>
                            <strong>{item.manifest.display_name}</strong>
                            <small>{item.manifest.description}</small>
                            <em>
                              v{item.manifest.version} ·{" "}
                              {item.connected ? "уже подключён" : "доступен"}
                            </em>
                          </span>
                        </button>
                      ))}
                    </div>
                  )}
                </>
              ) : (
                <>
                  <p>Создайте стабильную карточку учебного заведения.</p>
                  <div className={formStyles["form-grid"]}>
                    {field("university_id", "Slug", "university")}
                    {field("university_name", "Короткое название")}
                    {field("university_full_name", "Полное название")}
                    {field(
                      "schedule_url",
                      "Официальная страница расписания",
                      "https://…",
                    )}
                    {field("timezone", "Часовой пояс IANA")}
                    {field("locale", "Локаль")}
                  </div>
                </>
              )}
            </div>
          )}
          {step === 3 && (
            <div className={styles["wizard-fields"]}>
              {draft.integration_mode === "managed_parser" && selected ? (
                <div className={styles["wizard-summary"]}>
                  <p>
                    Код хранится в репозитории проекта. После создания
                    переведите интеграцию в тестирование и запустите первый
                    снимок.
                  </p>
                  <div>
                    <span>Парсер</span>
                    <strong>{selected.manifest.display_name}</strong>
                  </div>
                  <div>
                    <span>Контракт</span>
                    <strong>{selected.manifest.contract_version}</strong>
                  </div>
                  <div>
                    <span>Ответственный</span>
                    <strong>
                      {selected.manifest.maintainer_name ||
                        "сообщество проекта"}
                    </strong>
                  </div>
                </div>
              ) : (
                <>
                  <p>
                    {draft.integration_mode === "declarative_pull"
                      ? "Укажите HTTPS endpoint с готовым Schedule Snapshot v1. Внутренние и локальные адреса заблокированы."
                      : "Укажите владельца внешнего коннектора. После создания будет выпущен одноразово показываемый ключ."}
                  </p>
                  <div className={formStyles["form-grid"]}>
                    {draft.integration_mode === "declarative_pull" &&
                      field(
                        "declarative_url",
                        "URL снимка",
                        "https://schedule.example/snapshot.json",
                      )}
                    {field("display_name", "Название интеграции")}
                    {field("maintainer_name", "Ответственный")}
                    {field("maintainer_url", "Ссылка на проект или автора")}
                    {field("description", "Краткое описание")}
                    {field("update_interval", "Интервал, секунд", "3600")}
                  </div>
                </>
              )}
            </div>
          )}
          {step === 4 && (
            <div className={styles["wizard-summary"]}>
              <p>
                {draft.integration_mode === "external_push"
                  ? "Закрытый Ed25519-ключ будет показан один раз."
                  : "Первый запуск создаст тестовый снимок без автоматической публикации."}
              </p>
              {rows.map(([label, value]) => (
                <div key={label}>
                  <span>{label}</span>
                  <strong>{value}</strong>
                </div>
              ))}
              <div className={styles["wizard-safety"]}>
                <ShieldCheck size={18} />
                <span>
                  Любой способ использует одинаковые проверки структуры,
                  сравнение с последней доверенной версией и карантин аномалий.
                </span>
              </div>
            </div>
          )}
          <footer>
            <button
              className={classNames(buttonStyles["button"], buttonStyles["button-ghost"])}
              onClick={() =>
                step === 1 ? close() : setStep((value) => value - 1)
              }
            >
              <ArrowLeft size={15} /> {step === 1 ? "Отмена" : "Назад"}
            </button>
            {step < 4 ? (
              <button
                className={classNames(buttonStyles["button"], buttonStyles["button-primary"])}
                disabled={!canContinue}
                onClick={() => setStep((value) => value + 1)}
              >
                Продолжить <ArrowRight size={15} />
              </button>
            ) : (
              <button
                className={classNames(buttonStyles["button"], buttonStyles["button-primary"])}
                disabled={busy}
                onClick={() => void create()}
              >
                <Plus size={15} /> {busy ? "Создание…" : "Создать интеграцию"}
              </button>
            )}
          </footer>
        </section>
      </div>
    </DialogPortal>
  );
}

function CredentialsDialog({
  value,
  onClose,
  notify,
}: {
  value: ConnectorCredentials;
  onClose: () => void;
  notify: (text: string, tone?: ToastMessage["tone"]) => void;
}) {
  const config = JSON.stringify(
    {
      base_url: window.location.origin,
      connector_id: value.connector_id,
      key_id: value.key_id,
      private_key: value.private_key,
    },
    null,
    2,
  );
  async function copy() {
    await navigator.clipboard.writeText(config);
    notify("Конфигурация скопирована");
  }
  function download() {
    const link = document.createElement("a");
    link.href = URL.createObjectURL(
      new Blob([config], { type: "application/json" }),
    );
    link.download = `scheduler-connector-${value.connector_id}.json`;
    link.click();
    URL.revokeObjectURL(link.href);
  }
  return (
    <DialogPortal>
      <div className={dialogStyles["dialog-backdrop"]} role="presentation">
        <section className={styles["credentials-dialog"]} role="dialog" aria-modal="true">
          <span className={styles["credentials-icon"]}>
            <KeyRound size={20} />
          </span>
          <h2>Сохраните закрытый ключ</h2>
          <p>
            После закрытия это окно восстановить ключ невозможно. При утрате
            выпустите новый — старый будет отозван.
          </p>
          <pre>{config}</pre>
          <div className={dialogStyles["dialog-actions"]}>
            <button className={classNames(buttonStyles["button"], buttonStyles["button-ghost"])} onClick={() => void copy()}>
              <Clipboard size={15} /> Копировать
            </button>
            <button className={classNames(buttonStyles["button"], buttonStyles["button-ghost"])} onClick={download}>
              <ExternalLink size={15} /> Скачать JSON
            </button>
            <button
              className={classNames(buttonStyles["button"], buttonStyles["button-primary"])}
              data-dialog-dismiss
              onClick={onClose}
            >
              <Check size={15} /> Я сохранил ключ
            </button>
          </div>
        </section>
      </div>
    </DialogPortal>
  );
}

function QualityPolicyDialog({
  connector,
  onClose,
  onSaved,
  notify,
}: {
  connector: ConnectorClient;
  onClose: () => void;
  onSaved: () => Promise<void>;
  notify: (text: string, tone?: ToastMessage["tone"]) => void;
}) {
  const [policy, setPolicy] = useState<SourceQualityPolicy>(
    connector.quality_policy,
  );
  const confirmClose = useUnsavedChanges(
    JSON.stringify(policy) !== JSON.stringify(connector.quality_policy),
  );
  const close = () => {
    if (confirmClose()) onClose();
  };
  const [busy, setBusy] = useState(false);
  function numericField(
    name: keyof SourceQualityPolicy,
    label: string,
    step: string,
  ) {
    return (
      <label>
        <span>{label}</span>
        <input
          type="number"
          min="0"
          max={name.includes("ratio") ? "10" : undefined}
          step={step}
          value={String(policy[name])}
          onChange={(event) =>
            setPolicy((current) => ({
              ...current,
              [name]: Number(event.target.value),
            }))
          }
        />
      </label>
    );
  }
  async function save() {
    setBusy(true);
    try {
      await api.updateConnector(connector.id, { quality_policy: policy });
      notify("Правила проверки снимков сохранены");
      await onSaved();
    } catch (error) {
      notify(
        error instanceof Error ? error.message : "Не удалось сохранить правила",
        "error",
      );
    } finally {
      setBusy(false);
    }
  }
  return (
    <DialogPortal>
      <div className={dialogStyles["dialog-backdrop"]} role="presentation">
        <section
          className={classNames(styles["connector-wizard"], styles["quality-policy-dialog"])}
          role="dialog"
          aria-modal="true"
        >
          <header>
            <div>
              <span>{connector.university_name}</span>
              <h2>Правила качества снимка</h2>
            </div>
            <button
              data-dialog-dismiss
              disabled={busy}
              onClick={close}
              aria-label="Закрыть"
            >
              <X size={19} />
            </button>
          </header>
          <p>
            Снимок, выходящий за эти границы, попадёт в карантин и не заменит
            рабочее расписание без проверки.
          </p>
          <div className={formStyles["form-grid"]}>
            {numericField("minimum_groups", "Минимум групп", "1")}
            {numericField("minimum_lessons", "Минимум занятий", "1")}
            {numericField(
              "maximum_group_drop_ratio",
              "Допустимое уменьшение групп (0.3 = 30%)",
              "0.05",
            )}
            {numericField(
              "maximum_group_growth_ratio",
              "Допустимый рост групп",
              "0.05",
            )}
            {numericField(
              "maximum_lesson_drop_ratio",
              "Допустимое уменьшение занятий",
              "0.05",
            )}
            {numericField(
              "maximum_lesson_growth_ratio",
              "Допустимый рост занятий",
              "0.05",
            )}
          </div>
          <label className={styles["quality-policy-toggle"]}>
            <input
              type="checkbox"
              checked={policy.allow_empty}
              onChange={(event) =>
                setPolicy((current) => ({
                  ...current,
                  allow_empty: event.target.checked,
                }))
              }
            />
            <span>Разрешить публикацию полностью пустого расписания</span>
          </label>
          <footer>
            <button
              className={classNames(buttonStyles["button"], buttonStyles["button-ghost"])}
              disabled={busy}
              data-dialog-dismiss
              onClick={close}
            >
              Отмена
            </button>
            <button
              className={classNames(buttonStyles["button"], buttonStyles["button-primary"])}
              disabled={busy}
              onClick={() => void save()}
            >
              <Check size={15} /> {busy ? "Сохранение…" : "Сохранить правила"}
            </button>
          </footer>
        </section>
      </div>
    </DialogPortal>
  );
}
