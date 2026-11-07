import { classNames } from "../../../../shared/classNames";
import styles from "./Toasts.module.css";

import { AlertCircle, Check } from "lucide-react";

export type ToastMessage = {
  id: number;
  text: string;
  tone: "success" | "error" | "info";
};

export function Toasts({ items }: { items: ToastMessage[] }) {
  return (
    <div className={styles["toast-stack"]} aria-live="polite">
      {items.map((item) => (
        <div key={item.id} className={classNames(styles["toast"], styles[`toast-${item.tone}`])}>
          {item.tone === "success" ? <Check size={18} aria-hidden="true" /> : <AlertCircle size={18} aria-hidden="true" />}
          <span>{item.text}</span>
        </div>
      ))}
    </div>
  );
}
