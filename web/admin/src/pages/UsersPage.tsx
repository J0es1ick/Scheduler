import { useState } from "react";
import { BellRing, Shield, UserRoundCheck } from "lucide-react";
import { api } from "../api";
import {
  EmptyBlock,
  ErrorBlock,
  formatDateTime,
  LoadingBlock,
  SearchField,
  SectionTitle,
  type ToastMessage,
} from "../components";
import { useDebounced, useRemote } from "../hooks";
import type { AdminIdentity, AdminRole } from "../types";

const roleLabels: Record<AdminRole, string> = {
  none: "Нет доступа",
  read_only: "Только просмотр",
  support: "Поддержка",
  editor: "Редактор",
  reviewer: "Проверяющий",
  operator: "Оператор",
  owner: "Владелец",
};

export function UsersPage({
  user,
  notify,
}: {
  user: AdminIdentity;
  notify: (text: string, tone?: ToastMessage["tone"]) => void;
}) {
  const [query, setQuery] = useState("");
  const [busy, setBusy] = useState("");
  const debounced = useDebounced(query);
  const users = useRemote(() => api.users(debounced), [debounced]);

  const changeRole = async (id: string, role: AdminRole) => {
    setBusy(id);
    try {
      await api.updateUser(id, role);
      notify(`Назначена роль: ${roleLabels[role]}`);
      await users.reload();
    } catch (caught) {
      notify(
        caught instanceof Error ? caught.message : "Не удалось изменить роль",
        "error",
      );
    } finally {
      setBusy("");
    }
  };

  return (
    <div className="page-stack users-page">
      <div className="page-intro compact-intro">
        <div>
          <h2>Пользователи и права</h2>
          <p>
            Роль администратора разрешает вход только через подтверждённую
            учётную запись Telegram.
          </p>
        </div>
        <SearchField
          value={query}
          onChange={setQuery}
          placeholder="ID или username"
        />
      </div>
      <section className="card-surface table-card">
        <SectionTitle title="Пользователи" />
        {users.loading && !users.data ? (
          <LoadingBlock rows={6} />
        ) : users.error ? (
          <ErrorBlock message={users.error} retry={users.reload} />
        ) : !users.data?.length ? (
          <EmptyBlock
            title="Пользователей нет"
            text="Они появятся после первого запуска бота."
          />
        ) : (
          <div className="user-grid">
            {users.data.map((item) => (
              <article className="user-card" key={item.id}>
                <div
                  className={`user-avatar ${item.is_admin ? "is-admin" : ""}`}
                >
                  {(item.username || item.id).slice(0, 1).toUpperCase()}
                  <i />
                </div>
                <div className="user-copy">
                  <div>
                    <h3>
                      {item.username
                        ? `@${item.username.replace("@", "")}`
                        : `ID ${item.id}`}
                    </h3>
                    {item.is_admin && (
                      <span className="admin-label">
                        <Shield size={12} /> admin
                      </span>
                    )}
                  </div>
                  <p>Telegram ID: {item.id}</p>
                  <div>
                    <span>
                      <BellRing size={14} /> {item.subscriptions} подписок на
                      группы
                    </span>
                    <span>
                      Основная: {item.default_group_name || "не выбрана"}
                    </span>
                    <span>
                      Уведомления:{" "}
                      {item.notifications_enabled ? "включены" : "выключены"}
                    </span>
                    <span>
                      <UserRoundCheck size={14} />{" "}
                      {formatDateTime(item.created_at)}
                    </span>
                  </div>
                </div>
                <label className="role-select">
                  <Shield size={15} />
                  <select
                    value={item.admin_role}
                    disabled={busy === item.id || item.id === user.id}
                    onChange={(event) =>
                      void changeRole(item.id, event.target.value as AdminRole)
                    }
                  >
                    {Object.entries(roleLabels).map(([role, label]) => (
                      <option key={role} value={role}>
                        {label}
                      </option>
                    ))}
                  </select>
                </label>
              </article>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}
