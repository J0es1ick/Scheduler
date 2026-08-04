const formatNumber = new Intl.NumberFormat("ru-RU");

interface MetricProps {
  value?: number;
  label: string;
  loading: boolean;
}

export function Metric({ value, label, loading }: MetricProps) {
  return (
    <div className="public-metric">
      <strong className={loading ? "is-loading" : ""}>
        {loading || value === undefined ? "—" : formatNumber.format(value)}
      </strong>
      <span>{label}</span>
    </div>
  );
}
