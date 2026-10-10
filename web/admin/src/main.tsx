import { lazy, StrictMode, Suspense } from "react";
import { createRoot } from "react-dom/client";
import App from "./app/App";
import { ErrorBoundary } from "./app/ErrorBoundary";
import "./styles.css";

const PersonalApp = lazy(() => import("./personal/PersonalApp"));

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <ErrorBoundary>
      {window.location.pathname.replace(/\/$/, "").endsWith("/app") ? (
        <Suspense fallback={<p>Загрузка…</p>}>
          <PersonalApp />
        </Suspense>
      ) : (
        <App />
      )}
    </ErrorBoundary>
  </StrictMode>,
);
