import { useEffect, useRef, useState } from "react";
import type { JSONContent } from "@tiptap/react";
import {
  ArrowDown,
  ArrowLeft,
  ArrowUp,
  FileText,
  Plus,
  Send,
  Trash2,
} from "lucide-react";
import {
  attachmentURL,
  broadcastAPI,
  type Broadcast,
  type BroadcastAudience,
  type BroadcastPreview,
} from "../api/broadcasts";
import {
  ErrorBlock,
  LoadingBlock,
  formatDateTime,
  type ToastMessage,
} from "../components";
import { MessageEditor } from "../components/MessageEditor";
import { useRemote } from "../hooks";
import { useUnsavedChanges } from "../hooks/useUnsavedChanges";

const labels: Record<string, string> = {
  draft: "Черновик",
  sending: "Отправляется",
  completed: "Завершена",
  cancelled: "Остановлена",
  delivered: "Доставлено",
  pending: "Ожидает",
  partial: "Частично доставлено",
  failed: "Ошибка",
  skipped: "Пропущено",
  ready: "Согласие получено",
  blocked: "Заблокирован",
  no_consent: "Нет согласия",
  unknown: "Не найден",
};
const errorText = (error: unknown) =>
  error instanceof Error ? error.message : "Не удалось выполнить действие";

export function BroadcastsPage({
  notify,
}: {
  notify: (text: string, tone?: ToastMessage["tone"]) => void;
}) {
  const [offset, setOffset] = useState(0);
  const history = useRemote(() => broadcastAPI.list(offset), [offset]);
  const [active, setActive] = useState<Broadcast | null>(null);
  const [loading, setLoading] = useState(false);
  const open = async (id?: string) => {
    setLoading(true);
    try {
      const selected = id ?? (await broadcastAPI.create()).id;
      setActive(await broadcastAPI.get(selected));
    } catch (error) {
      notify(errorText(error), "error");
    } finally {
      setLoading(false);
    }
  };
  if (active)
    return (
      <BroadcastComposer
        key={active.id}
        initial={active}
        onBack={() => {
          setActive(null);
          void history.reload();
        }}
        notify={notify}
      />
    );
  return (
    <div className="page-stack broadcasts-page">
      <div className="page-intro">
        <div>
          <h2>Обновления для подписчиков</h2>
          <p>
            Авторские сообщения получают только пользователи, давшие согласие.
          </p>
        </div>
        <button
          className="button button-primary"
          disabled={loading}
          onClick={() => void open()}
        >
          <Plus size={17} /> Новая рассылка
        </button>
      </div>
      {history.error && (
        <ErrorBlock message={history.error} retry={history.reload} />
      )}
      {history.loading && !history.data ? (
        <LoadingBlock rows={4} />
      ) : (
        <section className="broadcast-history" aria-label="История рассылок">
          {!history.data?.length && (
            <div className="broadcast-empty">
              <Send size={28} />
              <h3>Рассылок пока нет</h3>
              <p>
                Создайте сообщение, выберите получателей и проверьте его перед
                отправкой.
              </p>
            </div>
          )}
          {history.data?.map((item) => (
            <button
              key={item.id}
              className="broadcast-history-row"
              disabled={loading}
              onClick={() => void open(item.id)}
            >
              <span className={`broadcast-status status-${item.status}`}>
                {labels[item.status]}
              </span>
              <span>
                <strong>
                  {extractText(item.document).slice(0, 100) ||
                    "Новое сообщение"}
                </strong>
                <small>
                  {item.audience_mode === "all"
                    ? "Все подписавшиеся"
                    : "По выбранным никам"}
                </small>
              </span>
              <time>{formatDateTime(item.created_at)}</time>
            </button>
          ))}
        </section>
      )}
      <div className="table-pagination">
        <button
          className="button button-ghost"
          disabled={offset === 0}
          onClick={() => setOffset(Math.max(0, offset - 50))}
        >
          Назад
        </button>
        <span>Страница {offset / 50 + 1}</span>
        <button
          className="button button-ghost"
          disabled={(history.data?.length ?? 0) < 50}
          onClick={() => setOffset(offset + 50)}
        >
          Далее
        </button>
      </div>
    </div>
  );
}

function extractText(doc: JSONContent): string {
  return (
    doc.text ??
    doc.content?.map(extractText).join(doc.type === "paragraph" ? "" : " ") ??
    ""
  );
}

function BroadcastComposer({
  initial,
  onBack,
  notify,
}: {
  initial: Broadcast;
  onBack: () => void;
  notify: (text: string, tone?: ToastMessage["tone"]) => void;
}) {
  const [broadcast, setBroadcast] = useState(initial);
  const [document, setDocument] = useState(initial.document);
  const [mode, setMode] = useState(initial.audience_mode);
  const [usernames, setUsernames] = useState(
    initial.recipients
      .map((r) => r.username)
      .filter(Boolean)
      .join("\n"),
  );
  const [userIDs, setUserIDs] = useState(
    initial.recipients.map((r) => r.user_id),
  );
  const [attachments, setAttachments] = useState(initial.attachments);
  const [audience, setAudience] = useState<BroadcastAudience | null>(null);
  const [preview, setPreview] = useState<BroadcastPreview | null>(null);
  const [dirty, setDirty] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [linkType, setLinkType] = useState<"photo" | "document">("photo");
  const sendKey = useRef("");
  const working = useRef(false);
  const canLeave = useUnsavedChanges(dirty);
  const draft = broadcast.status === "draft";
  const change = () => {
    setDirty(true);
    setPreview(null);
  };
  const run = async (action: () => Promise<void>) => {
    if (working.current) return;
    working.current = true;
    setBusy(true);
    setError("");
    try {
      await action();
    } catch (error) {
      setError(errorText(error));
    } finally {
      working.current = false;
      setBusy(false);
    }
  };
  useEffect(() => {
    if (broadcast.status !== "sending") return;
    let alive = true;
    const timer = window.setInterval(() => {
      void broadcastAPI
        .get(broadcast.id)
        .then((b) => {
          if (alive) setBroadcast(b);
        })
        .catch((error) => {
          if (alive) setError(errorText(error));
        });
    }, 3000);
    return () => {
      alive = false;
      window.clearInterval(timer);
    };
  }, [broadcast.id, broadcast.status]);
  const save = async () => {
    const updated = await broadcastAPI.save(broadcast.id, {
      document,
      audience_mode: mode,
      user_ids: userIDs,
      attachment_ids: attachments.map((a) => a.id),
      version: broadcast.version,
    });
    setBroadcast(updated);
    setAttachments(updated.attachments);
    setDirty(false);
    return updated;
  };
  const upload = async (files: File[]) => {
    let current = broadcast;
    for (const file of files) {
      if (file.size > 10_000_000)
        throw new Error(`${file.name}: максимальный размер 10 МБ`);
      const kind =
        linkType === "photo" && ["image/jpeg", "image/png"].includes(file.type)
          ? "photo"
          : "document";
      current = await broadcastAPI.upload(
        current.id,
        current.version,
        file,
        kind,
      );
      setBroadcast(current);
      setAttachments((previous) => [
        ...previous.filter((a) =>
          current.attachments.some((next) => next.id === a.id),
        ),
        ...current.attachments.filter(
          (a) => !previous.some((existing) => existing.id === a.id),
        ),
      ]);
      setPreview(null);
    }
  };
  const move = (index: number, delta: number) => {
    const next = [...attachments];
    [next[index], next[index + delta]] = [next[index + delta], next[index]];
    setAttachments(next);
    change();
  };
  return (
    <div className="page-stack broadcasts-page">
      <div className="page-intro">
        <div>
          <button
            className="text-button"
            disabled={busy}
            onClick={() => {
              if (canLeave()) onBack();
            }}
          >
            <ArrowLeft size={15} /> Все рассылки
          </button>
          <h2>{draft ? "Новое сообщение" : "Результат рассылки"}</h2>
          <p>
            {labels[broadcast.status]} · {formatDateTime(broadcast.created_at)}
            {broadcast.author_id
              ? ` · Автор ${broadcast.author_id}`
              : " · Локальный администратор"}
          </p>
        </div>
        {!draft && broadcast.status === "sending" && (
          <button
            className="button button-danger"
            disabled={busy}
            onClick={() =>
              void run(async () => {
                setBroadcast(await broadcastAPI.stop(broadcast.id));
                notify("Оставшаяся отправка остановлена");
              })
            }
          >
            Остановить отправку
          </button>
        )}
      </div>
      {error && (
        <div role="alert" className="broadcast-error">
          {error}
        </div>
      )}
      <div className="broadcast-workspace">
        <section className="broadcast-compose" aria-label="Редактор рассылки">
          {draft ? (
            <>
              <label className="field-label">Текст сообщения</label>
              <MessageEditor
                initial={initial.document}
                disabled={busy}
                onChange={(doc) => {
                  setDocument(doc);
                  change();
                }}
              />
              <section className="broadcast-attachments">
                <div className="section-heading">
                  <h3>Вложения</h3>
                  <span>{attachments.length} / 10</span>
                </div>
                <p>
                  Текст придёт первым, затем фото и документы в этом порядке. До
                  10 МБ на файл, до 50 МБ всего.
                </p>
                <div className="attachment-upload">
                  <select
                    aria-label="Как отправлять изображения"
                    value={linkType}
                    disabled={busy}
                    onChange={(e) =>
                      setLinkType(e.target.value as "photo" | "document")
                    }
                  >
                    <option value="photo">Изображения как фото</option>
                    <option value="document">Все вложения как документы</option>
                  </select>
                  <label className="button button-ghost">
                    <Plus size={16} /> Прикрепить файлы
                    <input
                      type="file"
                      multiple
                      disabled={busy || attachments.length >= 10}
                      onChange={(e) => {
                        const files = Array.from(e.target.files ?? []);
                        e.target.value = "";
                        void run(() => upload(files));
                      }}
                    />
                  </label>
                </div>
                <ol className="attachment-list">
                  {attachments.map((a, i) => (
                    <li key={a.id}>
                      <FileText size={18} />
                      <a
                        href={attachmentURL(broadcast.id, a.id)}
                        target="_blank"
                        rel="noreferrer"
                      >
                        {a.filename}
                      </a>
                      <small>{(a.size / 1_000_000).toFixed(2)} МБ</small>
                      <button
                        type="button"
                        aria-label={`Выше: ${a.filename}`}
                        disabled={busy || i === 0}
                        onClick={() => move(i, -1)}
                      >
                        <ArrowUp size={15} />
                      </button>
                      <button
                        type="button"
                        aria-label={`Ниже: ${a.filename}`}
                        disabled={busy || i === attachments.length - 1}
                        onClick={() => move(i, 1)}
                      >
                        <ArrowDown size={15} />
                      </button>
                      <button
                        type="button"
                        aria-label={`Удалить: ${a.filename}`}
                        disabled={busy}
                        onClick={() =>
                          void run(async () => {
                            const updated = await broadcastAPI.removeAttachment(
                              broadcast.id,
                              a.id,
                              broadcast.version,
                            );
                            setBroadcast(updated);
                            setAttachments(
                              attachments.filter((item) => item.id !== a.id),
                            );
                            setPreview(null);
                          })
                        }
                      >
                        <Trash2 size={15} />
                      </button>
                    </li>
                  ))}
                </ol>
              </section>
              <section className="broadcast-audience">
                <h3>Получатели</h3>
                <div className="audience-mode">
                  <label>
                    <input
                      type="radio"
                      name="audience"
                      checked={mode === "all"}
                      disabled={busy}
                      onChange={() => {
                        setMode("all");
                        setAudience(null);
                        change();
                      }}
                    />{" "}
                    Все подписавшиеся
                  </label>
                  <label>
                    <input
                      type="radio"
                      name="audience"
                      checked={mode === "selected"}
                      disabled={busy}
                      onChange={() => {
                        setMode("selected");
                        setAudience(null);
                        change();
                      }}
                    />{" "}
                    По никам
                  </label>
                </div>
                {mode === "selected" && (
                  <>
                    <textarea
                      aria-label="Ники получателей"
                      placeholder="@username, @another_user"
                      value={usernames}
                      disabled={busy}
                      onChange={(e) => {
                        setUsernames(e.target.value);
                        setAudience(null);
                        setUserIDs([]);
                        change();
                      }}
                    />
                    <button
                      className="button button-ghost"
                      disabled={busy || !usernames.trim()}
                      onClick={() =>
                        void run(async () => {
                          const result = await broadcastAPI.audience(
                            mode,
                            usernames,
                          );
                          setAudience(result);
                          setUserIDs(
                            result.recipients
                              .filter(
                                (r) =>
                                  r.status === "ready" &&
                                  r.reason !== "ambiguous",
                              )
                              .map((r) => r.user_id),
                          );
                          change();
                        })
                      }
                    >
                      Проверить ники
                    </button>
                  </>
                )}
                <p>
                  Без согласия сообщения не отправляются — в том числе при
                  выборе по нику.
                </p>
                {audience && (
                  <div className="audience-results">
                    {audience.recipients.map((r, i) => (
                      <label key={`${r.user_id}-${i}`}>
                        <input
                          type="checkbox"
                          disabled={busy || r.status !== "ready"}
                          checked={userIDs.includes(r.user_id)}
                          onChange={(e) => {
                            setUserIDs(
                              e.target.checked
                                ? [...userIDs, r.user_id]
                                : userIDs.filter((id) => id !== r.user_id),
                            );
                            change();
                          }}
                        />
                        <span>
                          @{r.username}
                          {r.reason === "ambiguous"
                            ? ` · ID ${r.user_id} — выберите явно`
                            : ""}
                        </span>
                        <small>{labels[r.status]}</small>
                      </label>
                    ))}
                  </div>
                )}
                {mode === "selected" && (
                  <strong>Выбрано: {userIDs.length}</strong>
                )}
              </section>
              <div className="broadcast-actions">
                <button
                  className="button button-ghost"
                  disabled={busy}
                  onClick={() =>
                    void run(async () => {
                      await save();
                      notify("Черновик сохранён");
                    })
                  }
                >
                  Сохранить черновик
                </button>
                <button
                  className="button button-primary"
                  disabled={busy || (mode === "selected" && !userIDs.length)}
                  onClick={() =>
                    void run(async () => {
                      const saved = await save();
                      setPreview(await broadcastAPI.preview(saved.id));
                      sendKey.current = crypto.randomUUID();
                    })
                  }
                >
                  Проверить и отправить
                </button>
                <button
                  className="text-button"
                  disabled={busy}
                  onClick={() =>
                    void run(async () => {
                      if (window.confirm("Удалить этот черновик?")) {
                        await broadcastAPI.delete(broadcast.id);
                        setDirty(false);
                        onBack();
                      }
                    })
                  }
                >
                  Удалить черновик
                </button>
              </div>
            </>
          ) : (
            <>
              <div className="broadcast-result-counts">
                {Object.entries(broadcast.counts).map(([status, count]) => (
                  <div key={status}>
                    <strong>{count}</strong>
                    <span>{labels[status] ?? status}</span>
                  </div>
                ))}
              </div>
              <div className="table-wrap">
                <table>
                  <thead>
                    <tr>
                      <th>Получатель</th>
                      <th>Результат</th>
                      <th>Частей</th>
                    </tr>
                  </thead>
                  <tbody>
                    {broadcast.recipients.map((r) => (
                      <tr key={r.user_id}>
                        <td>{r.username ? `@${r.username}` : r.user_id}</td>
                        <td>
                          {labels[r.status] ?? r.status}
                          {r.reason && <small>{r.reason}</small>}
                        </td>
                        <td>{r.parts}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </>
          )}
        </section>
        <aside
          className="broadcast-preview"
          aria-label="Предпросмотр сообщения"
        >
          <div className="section-heading">
            <h3>
              {preview
                ? "Готово к отправке"
                : draft
                  ? "Перед отправкой"
                  : "Отправленное сообщение"}
            </h3>
          </div>
          {draft && !preview ? (
            <div className="broadcast-preview-empty">
              <Send size={25} />
              <p>Заполните сообщение и нажмите «Проверить и отправить».</p>
              <small>
                Здесь появятся точный текст, порядок вложений и число
                получателей.
              </small>
            </div>
          ) : (
            <>
              <div className="telegram-message">
                <strong className="telegram-message-author">Scheduler</strong>
                <div
                  className="broadcast-body"
                  dangerouslySetInnerHTML={{
                    __html: preview?.body ?? broadcast.body,
                  }}
                />
                <div className="telegram-message-unsubscribe">
                  Отключить обновления сервиса
                </div>
              </div>
              {(preview?.broadcast.attachments ?? broadcast.attachments).map(
                (a) => (
                  <div className="telegram-attachment" key={a.id}>
                    {a.media_type === "photo" ? (
                      <img
                        src={attachmentURL(broadcast.id, a.id)}
                        alt={a.filename}
                      />
                    ) : (
                      <>
                        <FileText size={22} />
                        <a href={attachmentURL(broadcast.id, a.id)}>
                          {a.filename}
                        </a>
                      </>
                    )}
                  </div>
                ),
              )}
              {preview && draft && (
                <div className="broadcast-confirm">
                  <strong>Получат: {preview.eligible}</strong>
                  <p>
                    Отправка начнётся сразу. После запуска изменить сообщение
                    нельзя.
                  </p>
                  <button
                    className="button button-primary"
                    disabled={busy || !preview.eligible}
                    onClick={() =>
                      void run(async () => {
                        const sent = await broadcastAPI.send(
                          broadcast.id,
                          preview.broadcast.version,
                          sendKey.current,
                        );
                        setBroadcast(sent);
                        setDirty(false);
                        setPreview(null);
                        notify("Рассылка запущена");
                      })
                    }
                  >
                    <Send size={16} /> Отправить {preview.eligible}{" "}
                    пользователям
                  </button>
                </div>
              )}
            </>
          )}
        </aside>
      </div>
    </div>
  );
}
