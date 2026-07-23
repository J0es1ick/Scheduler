import {
  KeyRound,
  RefreshCw,
  CalendarPlus,
  Pencil,
  RotateCcw,
  Trash2,
  Settings2,
  ShieldCheck,
  UserCog,
  MessagesSquare,
} from "lucide-react";
import { api } from "../api";
import {
  EmptyBlock,
  ErrorBlock,
  formatDateTime,
  LoadingBlock,
  SectionTitle,
} from "../components";
import { useRemote } from "../hooks";

const actionCopy: Record<string, { title: string; icon: typeof ShieldCheck }> =
  {
    login: { title: "Вход в админку", icon: KeyRound },
    logout: { title: "Выход из админки", icon: KeyRound },
    sync_requested: { title: "Запрошено обновление", icon: RefreshCw },
    sync_completed: { title: "Обновление завершено", icon: RefreshCw },
    sync_failed: { title: "Ошибка обновления", icon: RefreshCw },
    update_interval: { title: "Изменён интервал", icon: Settings2 },
    update_admin_role: { title: "Изменена роль", icon: UserCog },
    create_lesson: { title: "Добавлено занятие", icon: CalendarPlus },
    update_lesson: { title: "Изменено занятие", icon: Pencil },
    delete_lesson: { title: "Удалено занятие", icon: Trash2 },
    restore_lesson: { title: "Восстановлено занятие", icon: RotateCcw },
    resolve_support_request: {
      title: "Рассмотрено обращение",
      icon: MessagesSquare,
    },
  };

export function AuditPage() {
  const audit = useRemote(() => api.audit(), []);
  return (
    <div className="page-stack audit-page">
      <section className="card-surface audit-card">
        <SectionTitle title="Административные действия" />
        {audit.loading && !audit.data ? (
          <LoadingBlock rows={6} />
        ) : audit.error ? (
          <ErrorBlock message={audit.error} retry={audit.reload} />
        ) : !audit.data?.length ? (
          <EmptyBlock
            title="Журнал пока пуст"
            text="Первое действие появится после входа или ручного запуска."
          />
        ) : (
          <div className="audit-timeline">
            {audit.data.map((item) => {
              const copy = actionCopy[item.action] ?? {
                title: item.action,
                icon: ShieldCheck,
              };
              const Icon = copy.icon;
              return (
                <article key={item.id}>
                  <div className="audit-icon">
                    <Icon size={17} />
                  </div>
                  <div className="audit-copy">
                    <div>
                      <strong>{copy.title}</strong>
                      <span>{item.actor_name || item.actor_id}</span>
                    </div>
                    <p>{auditObjectLabel(item)}</p>
                  </div>
                  <div className="audit-meta">
                    <strong>{formatDateTime(item.created_at)}</strong>
                    <span>{item.ip_address || "локально"}</span>
                  </div>
                </article>
              );
            })}
          </div>
        )}
      </section>
    </div>
  );
}

function auditObjectLabel(item: {
  action: string;
  object_type: string;
  object_id: string;
  details: Record<string, unknown>;
}) {
  if (item.action === "login") {
    return item.details.method === "telegram"
      ? "Telegram Mini App"
      : "Локальный ключ доступа";
  }
  if (item.action === "resolve_support_request") {
    const status =
      item.details.status === "approved" ? "принято в работу" : "отклонено";
    return `Обращение ${status}${item.object_id ? ` · ${item.object_id}` : ""}`;
  }
  return `${item.object_type}${item.object_id ? ` · ${item.object_id}` : ""}`;
}
