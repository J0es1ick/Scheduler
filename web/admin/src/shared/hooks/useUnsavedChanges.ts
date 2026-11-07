import { useEffect } from "react";

export function useUnsavedChanges(dirty: boolean) {
  useEffect(() => {
    if (!dirty) return;
    const unload = (event: BeforeUnloadEvent) => {
      event.preventDefault();
    };
    const navigate = (event: Event) => {
      if (!window.confirm("Уйти без сохранения введённых данных?"))
        event.preventDefault();
    };
    window.addEventListener("beforeunload", unload);
    window.addEventListener("scheduler:before-navigate", navigate);
    window.Telegram?.WebApp?.enableClosingConfirmation?.();
    return () => {
      window.removeEventListener("beforeunload", unload);
      window.removeEventListener("scheduler:before-navigate", navigate);
      window.Telegram?.WebApp?.disableClosingConfirmation?.();
    };
  }, [dirty]);
  return () => !dirty || window.confirm("Закрыть форму без сохранения?");
}
