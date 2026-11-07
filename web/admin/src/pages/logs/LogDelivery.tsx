import { formatDateTime } from "../../shared/lib/format";
import { classNames } from "../../../../shared/classNames";
import layoutStyles from "../../ui/layout/Layout.module.css";
import tableStyles from "../../ui/table/Table.module.css";
import buttonStyles from "../../ui/button/Button.module.css";
import calendarPreviewStyles from "../../features/schedule-editor/calendar-preview/CalendarPreview.module.css";
import { api } from "../../shared/api";
import { useRemote } from "../../shared/hooks";
import { ErrorBlock, LoadingBlock, SectionTitle } from "../../ui";

export function LogDelivery() {
  const health = useRemote(() => api.dashboard(), []);
  const delivery = health.data?.operations;
  return (
    <section
      className={classNames(layoutStyles["card-surface"], tableStyles["table-card"])}
      aria-label="Диагностика доставки"
    >
      <SectionTitle
        title="Доставка сообщений"
        action={
          <button className={buttonStyles["text-button"]} onClick={() => void health.reload()}>
            Обновить
          </button>
        }
      />
      {delivery ? (
        <div className={calendarPreviewStyles["calendar-preview"]}>
          <p>
            Ожидают: {delivery.pending_notifications + delivery.pending_outbox}.
            С ошибкой: {delivery.failed_notifications + delivery.failed_outbox}.
          </p>
          <p>
            Возраст старейшего сообщения:{" "}
            {Math.round(delivery.oldest_pending_seconds)} сек. Просроченных
            напоминаний в очереди: {delivery.expired_pending_reminders ?? 0}.
          </p>
          <p>
            Проверено: {formatDateTime(delivery.checked_at)}. При устойчивом
            росте очереди проверьте доступность Telegram и состояние процесса
            бота по инструкции эксплуатации.
          </p>
        </div>
      ) : health.error ? (
        <ErrorBlock message={health.error} retry={health.reload} />
      ) : (
        <LoadingBlock rows={2} />
      )}
    </section>
  );
}
