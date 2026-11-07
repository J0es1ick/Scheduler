import styles from "./SectionTitle.module.css";
import layoutStyles from "../layout/Layout.module.css";
import { type ReactNode } from "react";

export function SectionTitle({
  eyebrow,
  title,
  action,
}: {
  eyebrow?: string;
  title: string;
  action?: ReactNode;
}) {
  return (
    <div className={styles["section-title"]}>
      <div>
        {eyebrow && <span className={layoutStyles["eyebrow"]}>{eyebrow}</span>}
        <h2>{title}</h2>
      </div>
      {action}
    </div>
  );
}
