import dialogStyles from "./Dialog.module.css";
import { useEffect, useRef, type ReactNode } from "react";
import { createPortal } from "react-dom";

export function DialogPortal({ children }: { children: ReactNode }) {
  const container = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const previousOverflow = document.body.style.overflow;
    const previousFocus = document.activeElement as HTMLElement | null;
    const root = document.getElementById("root");
    const previousInert = root?.inert ?? false;
    if (root) root.inert = true;
    document.body.style.overflow = "hidden";
    const dialog =
      container.current?.querySelector<HTMLElement>('[role="dialog"]');
    dialog?.setAttribute("tabindex", "-1");
    const focusable = () =>
      Array.from(
        dialog?.querySelectorAll<HTMLElement>(
          'button:not(:disabled), a[href], input:not(:disabled), select:not(:disabled), textarea:not(:disabled), [tabindex="0"]',
        ) ?? [],
      ).filter((element) => element.getClientRects().length > 0);
    (
      dialog?.querySelector<HTMLElement>("input, select, textarea") ??
      focusable()[0] ??
      dialog
    )?.focus();
    const keydown = (event: KeyboardEvent) => {
      const dialogs = document.querySelectorAll('[role="dialog"]');
      if (dialogs[dialogs.length - 1] !== dialog) return;
      if (event.key === "Escape") {
        event.preventDefault();
        event.stopImmediatePropagation();
        dialog
          ?.querySelector<HTMLButtonElement>(
            `button[data-dialog-dismiss], button[aria-label="Закрыть"], .${dialogStyles["dialog-close"]}`,
          )
          ?.click();
      }
      if (event.key === "Tab") {
        const items = focusable();
        const first = items[0],
          last = items[items.length - 1];
        if (!first) {
          event.preventDefault();
          dialog?.focus();
        } else if (
          event.shiftKey &&
          (document.activeElement === first ||
            document.activeElement === dialog)
        ) {
          event.preventDefault();
          last.focus();
        } else if (!event.shiftKey && document.activeElement === last) {
          event.preventDefault();
          first.focus();
        }
      }
    };
    document.addEventListener("keydown", keydown, true);
    window.dispatchEvent(new Event("scheduler:dialog-state"));
    return () => {
      document.removeEventListener("keydown", keydown, true);
      document.body.style.overflow = previousOverflow;
      if (root) root.inert = previousInert;
      previousFocus?.focus();
      queueMicrotask(() =>
        window.dispatchEvent(new Event("scheduler:dialog-state")),
      );
    };
  }, []);
  return createPortal(
    <div ref={container} style={{ display: "contents" }}>
      {children}
    </div>,
    document.body,
  );
}
