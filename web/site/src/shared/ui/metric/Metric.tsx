import styles from "./Metric.module.css";
import statusStyles from "../status/Status.module.css";
const formatNumber = new Intl.NumberFormat("ru-RU");

interface MetricProps {
  value?: number;
  label: string;
  loading: boolean;
}

export function Metric({ value, label, loading }: MetricProps) {
  return (
    <div className={styles["public-metric"]}>
      <strong className={loading ? statusStyles["is-loading"] : undefined}>
        {loading || value === undefined ? "—" : formatNumber.format(value)}
      </strong>
      <span>{label}</span>
    </div>
  );
}
