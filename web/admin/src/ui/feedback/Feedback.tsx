import { classNames } from "../../../../shared/classNames";
import styles from "./Feedback.module.css";
import buttonStyles from "../button/Button.module.css";

import { AlertCircle, Clock3 } from "lucide-react";

export function LoadingBlock({ rows = 3 }: { rows?: number }) {
  return (
    <div className={styles["loading-stack"]} role="status" aria-label="Загрузка">
      {Array.from({ length: rows }).map((_, index) => (
        <div
          className={styles["skeleton"]}
          key={index}
          style={{ animationDelay: `${index * 80}ms` }}
        />
      ))}
    </div>
  );
}

export function ErrorBlock({
  message,
  retry,
}: {
  message: string;
  retry?: () => void;
}) {
  return (
    <div className={classNames(styles["state-block"], styles["state-error"])}>
      <AlertCircle size={24} />
      <div>
        <strong>Не удалось загрузить данные</strong>
        <p>{message}</p>
      </div>
      {retry && (
        <button className={classNames(buttonStyles["button"], buttonStyles["button-ghost"])} onClick={retry}>
          Повторить
        </button>
      )}
    </div>
  );
}

export function EmptyBlock({ title, text }: { title: string; text: string }) {
  return (
    <div className={styles["state-block"]}>
      <Clock3 size={24} />
      <div>
        <strong>{title}</strong>
        <p>{text}</p>
      </div>
    </div>
  );
}
