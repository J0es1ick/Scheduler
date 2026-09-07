import { ArrowRight, KeyRound, LockKeyhole } from "lucide-react";
import { FormEvent, useState, type ReactNode } from "react";
import { LogoMark } from "../components";

export function LoginPage({
  onLogin,
  onTelegramLogin,
  loading,
  telegramDetected,
  accessKeyEnabled,
  error,
  themeControl,
}: {
  onTelegramLogin?: () => Promise<void>;
  onLogin: (accessKey: string) => Promise<void>;
  loading: boolean;
  telegramDetected: boolean;
  accessKeyEnabled: boolean;
  error: string;
  themeControl: ReactNode;
}) {
  const [accessKey, setAccessKey] = useState("");

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!accessKey.trim() || loading) return;
    await onLogin(accessKey.trim());
  }

  return (
    <main className="login-page">
      <section className="login-sheet">
        <header>
          <LogoMark />
          <span>служебный доступ</span>
        </header>
        <div className="login-sheet-copy">
          <p>Администрирование расписаний</p>
          <h1>Scheduler</h1>
          <span>Источники, ручные правки, пользователи и журнал действий.</span>
        </div>
        <footer>Версия {new Date().getFullYear()}</footer>
      </section>

      <section className="login-panel-wrap">
        <div className="login-theme-control">{themeControl}</div>
        <div className="login-panel">
          <span className="login-panel-icon">
            <LockKeyhole size={20} />
          </span>
          <h2>Вход в админку</h2>
          <p>
            {telegramDetected
              ? loading
                ? "Проверяем вашу учётную запись Telegram…"
                : "Нажмите «Войти через Telegram», чтобы продолжить."
              : accessKeyEnabled
                ? "Введите аварийный ключ доступа из конфигурации сервиса."
                : "Откройте админку из меню бота. Доступ проверяется по вашей роли в Telegram."}
          </p>

          {onTelegramLogin && (
            <button
              type="button"
              className="login-submit"
              disabled={loading}
              onClick={() => void onTelegramLogin()}
            >
              {loading ? "Проверяем…" : "Войти через Telegram"}
            </button>
          )}
          {accessKeyEnabled && (
            <form onSubmit={submit}>
              <label htmlFor="access-key">Ключ доступа</label>
              <div className="login-input">
                <KeyRound size={17} />
                <input
                  id="access-key"
                  type="password"
                  autoComplete="current-password"
                  value={accessKey}
                  onChange={(event) => setAccessKey(event.target.value)}
                  placeholder="Введите ключ"
                  autoFocus={!telegramDetected}
                />
              </div>
              <button
                className="login-submit"
                disabled={loading || !accessKey.trim()}
              >
                <span>{loading ? "Проверяем…" : "Войти"}</span>
                <ArrowRight size={18} />
              </button>
            </form>
          )}

          {error && (
            <p className="login-error" role="alert">
              {error}
            </p>
          )}

          <div className="login-help">
            В Mini App вход выполняется автоматически для пользователей с ролью
            администратора.
          </div>
        </div>
      </section>
    </main>
  );
}
