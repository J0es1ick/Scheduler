import { useState } from "react";
import {
  Building2,
  Check,
  Clock3,
  ExternalLink,
  MessageSquareText,
  X,
} from "lucide-react";
import { api } from "../api";
import {
  EmptyBlock,
  ErrorBlock,
  formatDateTime,
  LoadingBlock,
  SearchField,
  type ToastMessage,
} from "../components";
import { useDebounced, useRemote } from "../hooks";
import type { SupportRequestView } from "../types";

const statusLabels: Record<SupportRequestView["status"], string> = {
  pending: "Ожидает решения",
  approved: "Принято в работу",
  rejected: "Отклонено",
};

const typeLabels: Record<SupportRequestView["request_type"], string> = {
  update_existing: "Обновление расписания",
  new_institution: "Новое учебное заведение",
};

export function SupportPage({
  notify,
}: {
  notify: (text: string, tone?: ToastMessage["tone"]) => void;
}) {
  const [status, setStatus] = useState("pending");
  const [requestType, setRequestType] = useState("");
  const [query, setQuery] = useState("");
  const [notes, setNotes] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState("");
  const debounced = useDebounced(query);
  const requests = useRemote(
    () => api.supportRequests({ status, type: requestType, q: debounced }),
    [status, requestType, debounced],
  );

  async function resolve(
    item: SupportRequestView,
    nextStatus: "approved" | "rejected",
  ) {
    const note = (notes[item.id] ?? "").trim();
    if (nextStatus === "rejected" && !note) {
      notify(
        "Укажите причину отклонения — пользователь получит этот комментарий",
        "error",
      );
      return;
    }
    setBusy(item.id);
    try {
      await api.resolveSupportRequest(item.id, nextStatus, note);
      notify(
        nextStatus === "approved"
          ? "Обращение принято в работу"
          : "Обращение отклонено",
      );
      await requests.reload();
    } catch (caught) {
      notify(
        caught instanceof Error
          ? caught.message
          : "Не удалось обработать обращение",
        "error",
      );
    } finally {
      setBusy("");
    }
  }

  return (
    <div className="page-stack support-page">
      <div className="page-intro compact-intro">
        <div>
          <h2>Обращения пользователей</h2>
          <p>
            Запросы на обновление подключённых расписаний и добавление новых
            учебных заведений.
          </p>
        </div>
        <SearchField
          value={query}
          onChange={setQuery}
          placeholder="Заведение, ссылка, Telegram ID"
        />
      </div>

      <div className="support-toolbar">
        <div className="segmented-control">
          {[
            ["pending", "Новые"],
            ["", "Все"],
            ["approved", "Принятые"],
            ["rejected", "Отклонённые"],
          ].map(([value, label]) => (
            <button
              key={label}
              className={status === value ? "is-active" : ""}
              onClick={() => setStatus(value)}
            >
              {label}
            </button>
          ))}
        </div>
        <select
          className="select-control"
          value={requestType}
          onChange={(event) => setRequestType(event.target.value)}
        >
          <option value="">Все типы</option>
          <option value="update_existing">Обновление расписания</option>
          <option value="new_institution">Новое учебное заведение</option>
        </select>
      </div>

      {requests.loading && !requests.data ? (
        <LoadingBlock rows={5} />
      ) : requests.error ? (
        <ErrorBlock message={requests.error} retry={requests.reload} />
      ) : !requests.data?.length ? (
        <EmptyBlock
          title="Обращений нет"
          text="Здесь появятся заявки, отправленные через горячую линию бота."
        />
      ) : (
        <section className="support-list">
          {requests.data.map((item) => (
            <article className="support-card card-surface" key={item.id}>
              <header>
                <div className={`support-type type-${item.request_type}`}>
                  {item.request_type === "new_institution" ? (
                    <Building2 size={18} />
                  ) : (
                    <ExternalLink size={18} />
                  )}
                </div>
                <div>
                  <span>{typeLabels[item.request_type]}</span>
                  <h3>
                    {item.username
                      ? `@${item.username.replace("@", "")}`
                      : `Telegram ID ${item.user_id}`}
                  </h3>
                </div>
                <div className={`support-status status-${item.status}`}>
                  {item.status === "pending" && <Clock3 size={13} />}
                  {item.status === "approved" && <Check size={13} />}
                  {item.status === "rejected" && <X size={13} />}
                  {statusLabels[item.status]}
                </div>
              </header>

              <div className="support-meta">
                <span>Заявка {item.id}</span>
                <span>{formatDateTime(item.created_at)}</span>
              </div>
              <pre>{item.details}</pre>

              {item.status === "pending" ? (
                <footer>
                  <label>
                    <span>Комментарий пользователю</span>
                    <textarea
                      maxLength={1000}
                      value={notes[item.id] ?? ""}
                      onChange={(event) =>
                        setNotes((current) => ({
                          ...current,
                          [item.id]: event.target.value,
                        }))
                      }
                      placeholder="Что будет сделано или почему запрос нельзя принять"
                    />
                  </label>
                  <div>
                    <button
                      className="button button-danger-soft"
                      disabled={busy === item.id}
                      onClick={() => void resolve(item, "rejected")}
                    >
                      <X size={16} /> Отклонить
                    </button>
                    <button
                      className="button button-primary"
                      disabled={busy === item.id}
                      onClick={() => void resolve(item, "approved")}
                    >
                      <Check size={16} /> Принять в работу
                    </button>
                  </div>
                </footer>
              ) : (
                <div className="support-resolution">
                  <MessageSquareText size={17} />
                  <div>
                    <strong>{statusLabels[item.status]}</strong>
                    <p>{item.review_note || "Без комментария"}</p>
                  </div>
                </div>
              )}
            </article>
          ))}
        </section>
      )}
    </div>
  );
}
