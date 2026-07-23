import { ArrowRight, KeyRound, LockKeyhole } from "lucide-react";
import { FormEvent, useState } from "react";
import { LogoMark } from "../components";

export function LoginPage({
  onLogin,
  loading,
  telegramDetected,
  error,
}: {
  onLogin: (accessKey: string) => Promise<void>;
  loading: boolean;
  telegramDetected: boolean;
  error: string;
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
        <div className="login-panel">
          <span className="login-panel-icon">
            <LockKeyhole size={20} />
          </span>
          <h2>Вход в админку</h2>
          <p>
            {telegramDetected
              ? "Проверяем вашу учётную запись Telegram…"
              : "Введите ключ доступа из конфигурации сервиса."}
          </p>

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
            {error && (
              <p className="login-error" role="alert">
                {error}
              </p>
            )}
            <button
              className="login-submit"
              disabled={loading || !accessKey.trim()}
            >
              <span>{loading ? "Проверяем…" : "Войти"}</span>
              <ArrowRight size={18} />
            </button>
          </form>

          <div className="login-help">
            В Mini App вход выполняется автоматически для пользователей с ролью
            администратора.
          </div>
        </div>
      </section>
    </main>
  );
}
