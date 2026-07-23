import { useState } from "react";
import { BellRing, Shield, ShieldOff, UserRoundCheck } from "lucide-react";
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
import type { AdminIdentity } from "../types";

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

  const changeRole = async (id: string, isAdmin: boolean) => {
    setBusy(id);
    try {
      await api.updateUser(id, isAdmin);
      notify(
        isAdmin
          ? "Пользователю выданы права администратора"
          : "Права администратора сняты",
      );
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
                <button
                  className={`button ${item.is_admin ? "button-danger-soft" : "button-ghost"}`}
                  disabled={
                    busy === item.id || (item.id === user.id && item.is_admin)
                  }
                  onClick={() => void changeRole(item.id, !item.is_admin)}
                >
                  {item.is_admin ? (
                    <>
                      <ShieldOff size={16} /> Снять роль
                    </>
                  ) : (
                    <>
                      <Shield size={16} /> Сделать admin
                    </>
                  )}
                </button>
              </article>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}
