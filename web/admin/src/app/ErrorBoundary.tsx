import { Component, type ErrorInfo, type ReactNode } from "react";
import { getCSRFToken } from "../api";

interface Props {
  children: ReactNode;
}

interface State {
  failed: boolean;
}

export class ErrorBoundary extends Component<Props, State> {
  state: State = { failed: false };

  static getDerivedStateFromError(): State {
    return { failed: true };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    void fetch("/api/client-errors", {
      method: "POST",
      credentials: "include",
      headers: {
        "Content-Type": "application/json",
        "X-CSRF-Token": getCSRFToken(),
      },
      body: JSON.stringify({
        message: error.message,
        stack: `${error.stack ?? ""}\n${info.componentStack ?? ""}`,
        url: window.location.href,
      }),
    }).catch(() => undefined);
  }

  render() {
    if (this.state.failed) {
      return (
        <main className="fatal-error">
          <section>
            <p className="eyebrow">Ошибка интерфейса</p>
            <h1>Страница не смогла продолжить работу</h1>
            <p>
              Данные не изменены. Перезагрузите страницу; техническая информация
              уже записана в журнал сервера.
            </p>
            <button type="button" onClick={() => window.location.reload()}>
              Перезагрузить
            </button>
          </section>
        </main>
      );
    }
    return this.props.children;
  }
}
