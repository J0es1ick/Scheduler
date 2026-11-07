import { formatDateTime } from "../../shared/lib/format";
import { classNames } from "../../../../shared/classNames";
import layoutStyles from "../../ui/layout/Layout.module.css";
import styles from "./SupportPage.module.css";
import formStyles from "../../ui/form/Form.module.css";
import stateStyles from "../../ui/state/State.module.css";
import lessonTypeStyles from "../../ui/lesson-type/LessonType.module.css";
import statusPillStyles from "../../ui/status/StatusPill.module.css";
import buttonStyles from "../../ui/button/Button.module.css";
import { useViewState } from "../../shared/hooks/useViewState";
import { useUnsavedChanges } from "../../shared/hooks/useUnsavedChanges";
import { useState } from "react";
import {
  Building2,
  Check,
  Clock3,
  ExternalLink,
  MessageSquareText,
  X,
} from "lucide-react";
import { api } from "../../shared/api";
import { EmptyBlock, ErrorBlock, LoadingBlock, SearchField, type ToastMessage } from "../../ui";
import { useDebounced, useRemote } from "../../shared/hooks";
import type { SupportRequestView } from "../../shared/model";

const statusLabels: Record<SupportRequestView["status"], string> = {
  pending: "Ожидает решения",
  approved: "Принято в работу",
  rejected: "Отклонено",
};

const typeLabels: Record<SupportRequestView["request_type"], string> = {
  update_existing: "Обновление расписания",
  new_institution: "Новое учебное заведение",
  feedback: "Пожелания и обратная связь",
};

export function SupportPage({
  notify,
}: {
  notify: (text: string, tone?: ToastMessage["tone"]) => void;
}) {
  const [status, setStatus] = useViewState("support:status", "pending");
  const [requestType, setRequestType] = useViewState("support:requestType", "");
  const [query, setQuery] = useViewState("support:query", "");
  const [notes, setNotes] = useState<Record<string, string>>({});
  useUnsavedChanges(
    Object.values(notes).some((note) => note.trim().length > 0),
  );
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
      setNotes((current) => {
        const remaining = { ...current };
        delete remaining[item.id];
        return remaining;
      });
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
    <div className={classNames(layoutStyles["page-stack"], styles["support-page"])}>
      <div className={classNames(layoutStyles["page-intro"], layoutStyles["compact-intro"])}>
        <div>
          <h2>Обращения пользователей</h2>
          <p>
            Ошибки расписания, новые учебные заведения и пожелания о работе
            бота.
          </p>
        </div>
        <SearchField
          value={query}
          onChange={setQuery}
          placeholder="Текст обращения, заведение, Telegram ID"
        />
      </div>

      <div className={styles["support-toolbar"]}>
        <div className={formStyles["segmented-control"]}>
          {[
            ["pending", "Новые"],
            ["", "Все"],
            ["approved", "Принятые"],
            ["rejected", "Отклонённые"],
          ].map(([value, label]) => (
            <button
              key={label}
              className={status === value ? stateStyles["is-active"] : undefined}
              onClick={() => setStatus(value)}
            >
              {label}
            </button>
          ))}
        </div>
        <select
          className={formStyles["select-control"]}
          aria-label="Тип обращения"
          value={requestType}
          onChange={(event) => setRequestType(event.target.value)}
        >
          <option value="">Все типы</option>
          <option value="update_existing">Обновление расписания</option>
          <option value="new_institution">Новое учебное заведение</option>
          <option value="feedback">Пожелания и обратная связь</option>
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
        <section className={styles["support-list"]}>
          {requests.data.map((item) => (
            <article className={classNames(styles["support-card"], layoutStyles["card-surface"])} key={item.id}>
              <header>
                <div className={classNames(styles["support-type"], lessonTypeStyles[`type-${item.request_type}`])}>
                  {item.request_type === "new_institution" ? (
                    <Building2 size={18} />
                  ) : item.request_type === "feedback" ? (
                    <MessageSquareText size={18} />
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
                <div className={classNames(styles["support-status"], statusPillStyles[`status-${item.status}`])}>
                  {item.status === "pending" && <Clock3 size={13} />}
                  {item.status === "approved" && <Check size={13} />}
                  {item.status === "rejected" && <X size={13} />}
                  {statusLabels[item.status]}
                </div>
              </header>

              <div className={styles["support-meta"]}>
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
                      className={classNames(buttonStyles["button"], buttonStyles["button-danger-soft"])}
                      disabled={busy === item.id}
                      onClick={() => void resolve(item, "rejected")}
                    >
                      <X size={16} /> Отклонить
                    </button>
                    <button
                      className={classNames(buttonStyles["button"], buttonStyles["button-primary"])}
                      disabled={busy === item.id}
                      onClick={() => void resolve(item, "approved")}
                    >
                      <Check size={16} /> Принять в работу
                    </button>
                  </div>
                </footer>
              ) : (
                <div className={styles["support-resolution"]}>
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
