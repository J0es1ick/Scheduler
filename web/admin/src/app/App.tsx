import { useEffect, useMemo, useState } from "react";
import { APIError, api } from "../api";
import { Toasts, type ToastMessage } from "../components";
import { AuditPage } from "../pages/AuditPage";
import { DataPage } from "../pages/DataPage";
import { EditorPage } from "../pages/EditorPage";
import { LoginPage } from "../pages/LoginPage";
import { LogsPage } from "../pages/LogsPage";
import { OverviewPage } from "../pages/OverviewPage";
import { SourcesPage } from "../pages/SourcesPage";
import { SupportPage } from "../pages/SupportPage";
import { UsersPage } from "../pages/UsersPage";
import type { AdminIdentity } from "../types";
import { AppLayout, type ViewName } from "./layout/AppLayout";

const knownViews: ViewName[] = [
  "overview",
  "editor",
  "sources",
  "logs",
  "data",
  "support",
  "users",
  "audit",
];

function viewFromHash(): ViewName {
  const candidate = window.location.hash.replace("#/", "") as ViewName;
  return knownViews.includes(candidate) ? candidate : "overview";
}

function messageFrom(error: unknown) {
  if (error instanceof APIError && error.status === 403)
    return "Доступ разрешён только администраторам.";
  if (error instanceof APIError && error.status === 401)
    return "Неверный ключ доступа.";
  return error instanceof Error
    ? error.message
    : "Не удалось выполнить запрос.";
}

export default function App() {
  const [user, setUser] = useState<AdminIdentity | null>(null);
  const [booting, setBooting] = useState(true);
  const [authError, setAuthError] = useState("");
  const [view, setView] = useState<ViewName>(viewFromHash);
  const [toasts, setToasts] = useState<ToastMessage[]>([]);
  const telegram = window.Telegram?.WebApp;
  const telegramDetected = Boolean(telegram?.initData);

  useEffect(() => {
    if (telegram?.initData) {
      telegram.ready();
      telegram.expand();
      telegram.setHeaderColor?.("#e9e2d5");
      telegram.setBackgroundColor?.("#f3efe5");
    }

    let active = true;
    async function bootstrap() {
      try {
        const identity = await api.me();
        if (active) setUser(identity);
      } catch (error) {
        if (
          error instanceof APIError &&
          error.status === 401 &&
          telegram?.initData
        ) {
          try {
            const identity = await api.loginWithTelegram(telegram.initData);
            if (active) setUser(identity);
          } catch (telegramError) {
            if (active) setAuthError(messageFrom(telegramError));
          }
        } else if (
          !(error instanceof APIError && error.status === 401) &&
          active
        ) {
          setAuthError(messageFrom(error));
        }
      } finally {
        if (active) setBooting(false);
      }
    }
    void bootstrap();
    return () => {
      active = false;
    };
  }, [telegram]);

  useEffect(() => {
    const syncHash = () => setView(viewFromHash());
    window.addEventListener("hashchange", syncHash);
    return () => window.removeEventListener("hashchange", syncHash);
  }, []);

  function navigate(next: ViewName) {
    setView(next);
    window.history.replaceState(null, "", `#/${next}`);
    window.scrollTo({ top: 0, behavior: "smooth" });
  }

  function notify(text: string, tone: ToastMessage["tone"] = "success") {
    const id = Date.now() + Math.random();
    setToasts((current) => [...current, { id, text, tone }]);
    window.setTimeout(
      () => setToasts((current) => current.filter((item) => item.id !== id)),
      4200,
    );
  }

  async function login(accessKey: string) {
    setBooting(true);
    setAuthError("");
    try {
      setUser(await api.loginWithAccessKey(accessKey));
    } catch (error) {
      setAuthError(messageFrom(error));
    } finally {
      setBooting(false);
    }
  }

  async function logout() {
    try {
      await api.logout();
    } finally {
      setUser(null);
      setAuthError("");
    }
  }

  const page = useMemo(() => {
    switch (view) {
      case "editor":
        return <EditorPage notify={notify} />;
      case "sources":
        return <SourcesPage notify={notify} />;
      case "logs":
        return <LogsPage />;
      case "data":
        return <DataPage />;
      case "support":
        return <SupportPage notify={notify} />;
      case "users":
        return user ? <UsersPage user={user} notify={notify} /> : null;
      case "audit":
        return <AuditPage />;
      default:
        return <OverviewPage onNavigate={navigate} />;
    }
  }, [view, user]);

  if (!user) {
    return (
      <LoginPage
        onLogin={login}
        loading={booting}
        telegramDetected={telegramDetected && booting}
        error={authError}
      />
    );
  }

  return (
    <>
      <AppLayout
        user={user}
        view={view}
        onNavigate={navigate}
        onLogout={() => void logout()}
      >
        {page}
      </AppLayout>
      <Toasts items={toasts} />
    </>
  );
}
