import { useEffect } from "react";

export function useMiniApp(view: string) {
  useEffect(() => {
    const app = window.Telegram?.WebApp;
    const apply = () => {
      for (const side of ["top", "bottom", "left", "right"] as const) {
        document.documentElement.style.setProperty(
          `--tg-safe-area-inset-${side}`,
          `${app?.safeAreaInset?.[side] ?? 0}px`,
        );
        document.documentElement.style.setProperty(
          `--tg-content-safe-area-inset-${side}`,
          `${app?.contentSafeAreaInset?.[side] ?? 0}px`,
        );
      }
      const height =
        window.visualViewport?.height ??
        app?.viewportHeight ??
        window.innerHeight;
      document.documentElement.style.setProperty(
        "--app-viewport-height",
        `${height}px`,
      );
    };
    apply();
    window.visualViewport?.addEventListener("resize", apply);
    app?.onEvent?.("viewportChanged", apply);
    app?.onEvent?.("safeAreaChanged", apply);
    app?.onEvent?.("contentSafeAreaChanged", apply);
    return () => {
      window.visualViewport?.removeEventListener("resize", apply);
      app?.offEvent?.("viewportChanged", apply);
      app?.offEvent?.("safeAreaChanged", apply);
      app?.offEvent?.("contentSafeAreaChanged", apply);
    };
  }, []);

  useEffect(() => {
    const back = window.Telegram?.WebApp?.BackButton;
    const update = () => {
      if (view !== "overview" || document.querySelector('[role="dialog"]'))
        back?.show();
      else back?.hide();
    };
    const onBack = () => {
      if (document.querySelector('[role="dialog"]')) {
        document.dispatchEvent(
          new KeyboardEvent("keydown", { key: "Escape", bubbles: true }),
        );
      } else if (view !== "overview") {
        if (window.history.state?.scheduler) window.history.back();
        else window.location.hash = "#/overview";
      }
    };
    update();
    back?.onClick(onBack);
    window.addEventListener("scheduler:dialog-state", update);
    return () => {
      back?.offClick(onBack);
      window.removeEventListener("scheduler:dialog-state", update);
    };
  }, [view]);
}
