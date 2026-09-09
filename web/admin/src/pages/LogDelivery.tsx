import { api } from "../api";
import { useRemote } from "../hooks";
import {
  ErrorBlock,
  LoadingBlock,
  SectionTitle,
  formatDateTime,
} from "../components";

export function LogDelivery() {
  const health = useRemote(() => api.dashboard(), []);
  const delivery = health.data?.operations;
  return (
    <section
      className="card-surface table-card"
      aria-label="Диагностика доставки"
    >
      <SectionTitle
        title="Доставка сообщений"
        action={
          <button className="text-button" onClick={() => void health.reload()}>
            Обновить
          </button>
        }
      />
      {delivery ? (
        <div className="calendar-preview">
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
