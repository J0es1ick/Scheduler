import { explicitlyLoggedOut, setExplicitLogout } from "./logoutState";
import { RefreshStatus } from "./RefreshStatus";
import { useMiniApp } from "./useMiniApp";
import { useTheme } from "./useTheme";
import { ThemeSwitch } from "./ThemeSwitch";
import { clearViewState } from "../hooks/useViewState";
import { useEffect, useState } from "react";
import { APIError, api } from "../api";
import { Toasts, type ToastMessage } from "../components";
import { AuditPage } from "../pages/AuditPage";
import { DataPage } from "../pages/DataPage";
import { ConnectorsPage } from "../pages/ConnectorsPage";
import { EditorPage } from "../pages/EditorPage";
import { LoginPage } from "../pages/LoginPage";
import { LogsPage } from "../pages/LogsPage";
import { OverviewPage } from "../pages/OverviewPage";
import { SourcesPage } from "../pages/SourcesPage";
import { SupportPage } from "../pages/SupportPage";
import { UsersPage } from "../pages/UsersPage";
import type { AdminIdentity } from "../types";
import { AppLayout, canAccessView, type ViewName } from "./layout/AppLayout";

const knownViews: ViewName[] = [
  "overview",
  "editor",
  "sources",
  "connectors",
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
    return "Сессия недействительна. Откройте вход заново.";
  return error instanceof Error
    ? error.message
    : "Не удалось выполнить запрос.";
}

export default function App() {
  const [user, setUser] = useState<AdminIdentity | null>(null);
  const [booting, setBooting] = useState(true);
  const [authError, setAuthError] = useState("");
  const [accessKeyEnabled, setAccessKeyEnabled] = useState(false);
  const [view, setView] = useState<ViewName>(viewFromHash);
  const [toasts, setToasts] = useState<ToastMessage[]>([]);
  useMiniApp(view);
  const theme = useTheme();
  const telegram = window.Telegram?.WebApp;
  const telegramDetected = Boolean(telegram?.initData);

  useEffect(() => {
    if (telegram?.initData) {
      telegram.ready();
      telegram.expand();
    }

    let active = true;
    async function bootstrap() {
      try {
        const authConfig = await api.authConfig();
        if (active) setAccessKeyEnabled(authConfig.access_key_enabled);
      } catch {
        // игнор
      }
      try {
        const identity = await api.me();
        if (active) setUser(identity);
      } catch (error) {
        if (
          error instanceof APIError &&
          error.status === 401 &&
          telegram?.initData &&
          !explicitlyLoggedOut()
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
    let accepted = window.location.hash;
    const syncHash = () => {
      if (window.location.hash === accepted) return;
      if (
        !window.dispatchEvent(
          new Event("scheduler:before-navigate", { cancelable: true }),
        )
      ) {
        window.history.pushState(
          { scheduler: true },
          "",
          accepted || "#/overview",
        );
        return;
      }
      accepted = window.location.hash;
      setView(viewFromHash());
    };
    window.addEventListener("hashchange", syncHash);
    window.addEventListener("popstate", syncHash);
    return () => {
      window.removeEventListener("hashchange", syncHash);
      window.removeEventListener("popstate", syncHash);
    };
  }, [view]);

  useEffect(() => {
    if (user && !canAccessView(view, user.role)) {
      setView("overview");
      window.history.replaceState(null, "", "#/overview");
    }
  }, [user, view]);

  useEffect(() => {
    const sessionExpired = () => {
      clearViewState();
      setUser(null);
      setAuthError(
        "Сессия истекла. Войдите снова через Telegram или аварийный ключ.",
      );
    };
    window.addEventListener("scheduler:session-expired", sessionExpired);
    return () =>
      window.removeEventListener("scheduler:session-expired", sessionExpired);
  }, []);

  function navigate(next: ViewName) {
    if (
      next === view ||
      !window.dispatchEvent(
        new Event("scheduler:before-navigate", { cancelable: true }),
      )
    )
      return;
    setView(next);
    window.history.pushState({ scheduler: true }, "", `#/${next}`);
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
      setExplicitLogout(false);
    } catch (error) {
      setAuthError(messageFrom(error));
    } finally {
      setBooting(false);
    }
  }

  async function loginTelegram() {
    if (!telegram?.initData) return;
    setBooting(true);
    setAuthError("");
    try {
      setUser(await api.loginWithTelegram(telegram.initData));
      setExplicitLogout(false);
    } catch (error) {
      setAuthError(messageFrom(error));
    } finally {
      setBooting(false);
    }
  }

  async function logout() {
    try {
      await api.logout();
      clearViewState();
      setExplicitLogout(true);
      clearViewState();
      setUser(null);
      setAuthError("");
    } catch (error) {
      notify(messageFrom(error), "error");
    }
  }

  const page = (() => {
    switch (view) {
      case "editor":
        return <EditorPage notify={notify} />;
      case "sources":
        return (
          <SourcesPage
            canOperate={user?.role === "owner" || user?.role === "operator"}
            notify={notify}
          />
        );
      case "connectors":
        return <ConnectorsPage notify={notify} />;
      case "logs":
        return <LogsPage role={user?.role ?? "none"} />;
      case "data":
        return (
          <DataPage
            canManage={user?.role === "owner" || user?.role === "operator"}
            notify={notify}
          />
        );
      case "support":
        return <SupportPage notify={notify} />;
      case "users":
        return user ? <UsersPage user={user} notify={notify} /> : null;
      case "audit":
        return <AuditPage />;
      default:
        return (
          <OverviewPage
            canEdit={!!user && canAccessView("editor", user.role)}
            canOperate={!!user && canAccessView("sources", user.role)}
            onNavigate={navigate}
          />
        );
    }
  })();

  if (!user) {
    return (
      <LoginPage
        themeControl={<ThemeSwitch {...theme} />}
        onLogin={login}
        onTelegramLogin={telegramDetected ? loginTelegram : undefined}
        loading={booting}
        telegramDetected={telegramDetected && booting}
        accessKeyEnabled={accessKeyEnabled}
        error={authError}
      />
    );
  }

  return (
    <>
      <AppLayout
        themeControl={<ThemeSwitch {...theme} />}
        user={user}
        view={view}
        onNavigate={navigate}
        onLogout={() => void logout()}
      >
        <RefreshStatus />
        {page}
      </AppLayout>
      <Toasts items={toasts} />
    </>
  );
}
