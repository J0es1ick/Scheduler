import { formatDateTime } from "../../shared/lib/format";
import { classNames } from "../../../../shared/classNames";
import styles from "./LogConsole.module.css";
import buttonStyles from "../../ui/button/Button.module.css";
import { useLayoutEffect, useRef } from "react";

import type { ServiceLogEntry } from "../../shared/model";

export function LogConsole({
  entries,
  label,
}: {
  entries: ServiceLogEntry[];
  label: string;
}) {
  const viewport = useRef<HTMLDivElement>(null);
  const following = useRef(true);
  useLayoutEffect(() => {
    if (viewport.current && following.current)
      viewport.current.scrollTop = viewport.current.scrollHeight;
  }, [entries]);

  return (
    <div className={styles["log-console-shell"]}>
      <div className={styles["log-console-caption"]}>
        <strong>{label}</strong>
        <span>Новые записи внизу</span>
      </div>
      <div
        className={styles["log-console"]}
        ref={viewport}
        role="region"
        aria-label={`Консоль: ${label}`}
        tabIndex={0}
        onScroll={(event) => {
          const node = event.currentTarget;
          following.current =
            node.scrollHeight - node.scrollTop - node.clientHeight < 24;
        }}
      >
        {entries.length === 0 ? (
          <p className={styles["log-console-empty"]}>
            В этом потоке нет записей за выбранный период и с заданными
            фильтрами.
          </p>
        ) : (
          [...entries].reverse().map((entry) => (
            <div
              className={classNames(styles["log-console-record"], styles[`log-console-${entry.level.toLowerCase()}`])}
              key={entry.id}
            >
              <time dateTime={entry.time}>{formatDateTime(entry.time)}</time>{" "}
              <strong className={styles["log-console-level"]}>{entry.level}</strong>{" "}
              <span className={styles["log-console-module"]}>
                [{entry.component}
                {entry.module !== entry.component ? `/${entry.module}` : ""}]
              </span>{" "}
              <span>{entry.message}</span>
              {entry.source && (
                <span className={styles["log-console-fields"]}>{` source=${entry.source}`}</span>
              )}
              {Object.keys(entry.fields).length > 0 && (
                <span className={styles["log-console-fields"]}>
                  {" "}
                  {JSON.stringify(entry.fields)}
                </span>
              )}
            </div>
          ))
        )}
      </div>
      <button
        type="button"
        className={classNames(buttonStyles["text-button"], styles["log-console-bottom"])}
        onClick={() => {
          following.current = true;
          if (viewport.current)
            viewport.current.scrollTop = viewport.current.scrollHeight;
        }}
      >
        К последним записям на странице ↓
      </button>
    </div>
  );
}
