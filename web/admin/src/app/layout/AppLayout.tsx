import type { ReactNode } from "react";
import {
  Activity,
  CalendarDays,
  CalendarRange,
  Database,
  History,
  LayoutDashboard,
  LogOut,
  MessagesSquare,
  RadioTower,
  ShieldCheck,
  Users,
} from "lucide-react";
import { LogoMark } from "../../components";
import type { AdminIdentity } from "../../types";

export type ViewName =
  | "overview"
  | "editor"
  | "sources"
  | "logs"
  | "data"
  | "support"
  | "users"
  | "audit";

const navigation: Array<{
  id: ViewName;
  label: string;
  icon: typeof LayoutDashboard;
}> = [
  { id: "overview", label: "Обзор", icon: LayoutDashboard },
  { id: "editor", label: "Редактор", icon: CalendarDays },
  { id: "sources", label: "Источники", icon: RadioTower },
  { id: "logs", label: "Запуски", icon: History },
  { id: "data", label: "Расписание", icon: Database },
  { id: "support", label: "Обращения", icon: MessagesSquare },
  { id: "users", label: "Пользователи", icon: Users },
  { id: "audit", label: "Аудит", icon: ShieldCheck },
];

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
          {navigation.map((item) => {
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
                ? "Telegram Admin"
                : "Local Admin"}
            </span>
          </div>
          <button onClick={onLogout} title="Выйти">
            <LogOut size={17} />
          </button>
        </div>
      </aside>

      <main className="main-area">
        <header className="topbar">
          <div>
            <span className="mobile-brand">
              <LogoMark compact />
            </span>
            <p>{date}</p>
            <h1>{current.title}</h1>
            <span>{current.subtitle}</span>
          </div>
          <button
            className="topbar-audit"
            onClick={() => onNavigate("audit")}
            aria-label="Открыть аудит"
          >
            <ShieldCheck size={20} />
          </button>
        </header>
        <div className="content-area">{children}</div>
      </main>

      <nav className="mobile-nav" aria-label="Мобильная навигация">
        <button
          className={view === "overview" ? "is-active" : ""}
          onClick={() => onNavigate("overview")}
        >
          <LayoutDashboard size={20} />
          <span>Обзор</span>
        </button>
        <button
          className={view === "sources" ? "is-active" : ""}
          onClick={() => onNavigate("sources")}
        >
          <RadioTower size={20} />
          <span>Источники</span>
        </button>
        <button
          className={`mobile-primary ${view === "editor" ? "is-active" : ""}`}
          onClick={() => onNavigate("editor")}
          aria-label="Редактор"
        >
          <CalendarDays size={24} />
        </button>
        <button
          className={view === "support" ? "is-active" : ""}
          onClick={() => onNavigate("support")}
        >
          <MessagesSquare size={20} />
          <span>Заявки</span>
        </button>
        <button
          className={view === "users" ? "is-active" : ""}
          onClick={() => onNavigate("users")}
        >
          <Users size={20} />
          <span>Люди</span>
        </button>
      </nav>
    </div>
  );
}
