import type { ReactNode } from "react";
import {
  Activity,
  CalendarDays,
  Database,
  History,
  LayoutDashboard,
  LogOut,
  MessagesSquare,
  RadioTower,
  PlugZap,
  ShieldCheck,
  Users,
} from "lucide-react";
import { LogoMark } from "../../components";
import type { AdminIdentity, AdminRole } from "../../types";

export type ViewName =
  | "overview"
  | "editor"
  | "sources"
  | "connectors"
  | "logs"
  | "data"
  | "support"
  | "users"
  | "audit";

const navigation: Array<{
  id: ViewName;
  label: string;
  icon: typeof LayoutDashboard;
  minimumRole: AdminRole;
}> = [
  {
    id: "overview",
    label: "Обзор",
    icon: LayoutDashboard,
    minimumRole: "read_only",
  },
  {
    id: "editor",
    label: "Редактор",
    icon: CalendarDays,
    minimumRole: "editor",
  },
  {
    id: "sources",
    label: "Источники",
    icon: RadioTower,
    minimumRole: "operator",
  },
  {
    id: "connectors",
    label: "Интеграции",
    icon: PlugZap,
    minimumRole: "operator",
  },
  { id: "logs", label: "Запуски", icon: History, minimumRole: "read_only" },
  { id: "data", label: "Расписание", icon: Database, minimumRole: "read_only" },
  {
    id: "support",
    label: "Обращения",
    icon: MessagesSquare,
    minimumRole: "support",
  },
  { id: "users", label: "Пользователи", icon: Users, minimumRole: "owner" },
  { id: "audit", label: "Аудит", icon: ShieldCheck, minimumRole: "operator" },
];

const roleLabels: Record<AdminRole, string> = {
  none: "Нет доступа",
  read_only: "Наблюдатель",
  support: "Поддержка",
  editor: "Редактор",
  reviewer: "Ревьюер",
  operator: "Оператор",
  owner: "Владелец",
};

export function canAccessView(view: ViewName, role: AdminRole): boolean {
  const item = navigation.find((entry) => entry.id === view);
  if (!item) return false;
  if (role === "owner") return true;
  if (item.minimumRole === "read_only") return role !== "none";
  const permissions: Partial<Record<AdminRole, AdminRole[]>> = {
    support: ["support"],
    editor: ["editor"],
    reviewer: ["reviewer", "editor"],
    operator: ["operator", "reviewer", "editor", "support"],
  };
  return permissions[role]?.includes(item.minimumRole) ?? false;
}

const pageCopy: Record<ViewName, { title: string; subtitle: string }> = {
  overview: {
    title: "Обзор",
    subtitle: "Состояние сервиса и последние обновления",
  },
  editor: {
    title: "Редактор расписания",
    subtitle: "Ручные изменения поверх данных источников",
  },
  sources: { title: "Источники", subtitle: "Парсеры и интервалы обновления" },
  connectors: {
    title: "Интеграции",
    subtitle: "Управляемые парсеры, JSON pull и внешние поставщики",
  },
  logs: { title: "Запуски", subtitle: "История работы парсеров" },
  data: { title: "Справочники", subtitle: "Группы и занятия в базе" },
  support: { title: "Обращения", subtitle: "Горячая линия расписаний" },
  users: { title: "Пользователи", subtitle: "Подписки и права доступа" },
  audit: { title: "Аудит", subtitle: "Административные действия" },
};

export function AppLayout({
  user,
  view,
  onNavigate,
  onLogout,
  children,
}: {
  user: AdminIdentity;
  view: ViewName;
  onNavigate: (view: ViewName) => void;
  onLogout: () => void;
  children: ReactNode;
}) {
  const current = pageCopy[view];
  const date = new Intl.DateTimeFormat("ru-RU", {
    weekday: "long",
    day: "numeric",
    month: "long",
  }).format(new Date());

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <LogoMark />
        <nav className="desktop-nav" aria-label="Основная навигация">
          <span className="nav-caption">Управление</span>
          {navigation
            .filter((item) => canAccessView(item.id, user.role))
            .map((item) => {
              const Icon = item.icon;
              return (
                <button
                  key={item.id}
                  className={view === item.id ? "is-active" : ""}
                  onClick={() => onNavigate(item.id)}
                >
                  <Icon size={19} />
                  <span>{item.label}</span>
                </button>
              );
            })}
        </nav>
        <div className="sidebar-status">
          <Activity size={15} />
          <span>Сервис работает</span>
        </div>
        <div className="sidebar-user">
          <div className="avatar">
            {user.name.replace("@", "").slice(0, 1).toUpperCase()}
          </div>
          <div>
            <strong>{user.name}</strong>
            <span>
              {user.auth_method === "telegram"
                ? roleLabels[user.role]
                : `Локальный · ${roleLabels[user.role]}`}
            </span>
          </div>
          <button onClick={onLogout} title="Выйти">
            <LogOut size={17} />
          </button>
        </div>
      </aside>

      <main className="main-area">
        <header className="topbar">
          <div className="topbar-heading" key={view}>
            <span className="mobile-brand">
              <LogoMark compact />
            </span>
            <p>{date}</p>
            <h1>{current.title}</h1>
            <span>{current.subtitle}</span>
          </div>
          {canAccessView("audit", user.role) && (
            <button
              className="topbar-audit"
              onClick={() => onNavigate("audit")}
              aria-label="Открыть аудит"
            >
              <ShieldCheck size={20} />
            </button>
          )}
        </header>
        <div className="content-area">{children}</div>
      </main>

      <nav className="mobile-nav" aria-label="Мобильная навигация">
        {canAccessView("overview", user.role) && (
          <button
            className={view === "overview" ? "is-active" : ""}
            onClick={() => onNavigate("overview")}
          >
            <LayoutDashboard size={20} />
            <span>Обзор</span>
          </button>
        )}
        {canAccessView("sources", user.role) && (
          <button
            className={view === "sources" ? "is-active" : ""}
            onClick={() => onNavigate("sources")}
          >
            <RadioTower size={20} />
            <span>Источники</span>
          </button>
        )}
        {canAccessView("editor", user.role) && (
          <button
            className={`mobile-primary ${view === "editor" ? "is-active" : ""}`}
            onClick={() => onNavigate("editor")}
            aria-label="Редактор"
          >
            <CalendarDays size={24} />
          </button>
        )}
        {canAccessView("support", user.role) && (
          <button
            className={view === "support" ? "is-active" : ""}
            onClick={() => onNavigate("support")}
          >
            <MessagesSquare size={20} />
            <span>Заявки</span>
          </button>
        )}
        {canAccessView("users", user.role) && (
          <button
            className={view === "users" ? "is-active" : ""}
            onClick={() => onNavigate("users")}
          >
            <Users size={20} />
            <span>Люди</span>
          </button>
        )}
      </nav>
    </div>
  );
}
