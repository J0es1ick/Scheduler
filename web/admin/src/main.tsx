import "./app/styles/global.css";
import { lazy, StrictMode, Suspense } from "react";
import { createRoot } from "react-dom/client";
import App from "./app/App";
import { ErrorBoundary } from "./app/ErrorBoundary";

const PersonalApp = lazy(() => import("./pages/personal/PersonalApp"));

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
