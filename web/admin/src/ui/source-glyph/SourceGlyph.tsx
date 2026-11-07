import { classNames } from "../../../../shared/classNames";
import styles from "./SourceGlyph.module.css";
import stateStyles from "../state/State.module.css";

export function SourceGlyph({
  name,
  small = false,
}: {
  name: string;
  small?: boolean;
}) {
  const letters = name
    .replace(/[^А-ЯA-Z]/gi, "")
    .slice(0, 2)
    .toUpperCase();
  return (
    <div
      className={classNames(styles["source-glyph"], small ? stateStyles["is-small"] : undefined)}
      aria-hidden="true"
    >
      <span>{letters}</span>
      <i />
    </div>
  );
}
