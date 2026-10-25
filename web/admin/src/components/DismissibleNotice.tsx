import { useEffect, useId, type ReactNode } from "react";
import { CircleGauge, X, type LucideIcon } from "lucide-react";
import { useViewState } from "../hooks/useViewState";

export function DismissibleNotice({
  id,
  signature,
  title,
  children,
  action,
  tone = "warning",
  icon: Icon = CircleGauge,
  className = "",
  role,
}: {
  id: string;
  signature: string | null;
  title: string;
  children: ReactNode;
  action?: ReactNode;
  tone?: "warning" | "neutral";
  icon?: LucideIcon;
  className?: string;
  role?: "alert";
}) {
  const headingID = useId();
  const [dismissed, setDismissed] = useViewState<string | null>(`notice:${id}`, null);
  useEffect(() => {
    if (dismissed !== null && dismissed !== signature) setDismissed(null);
  }, [dismissed, signature, setDismissed]);

  if (signature === null || dismissed === signature) return null;

  return (
    <section
      className={`operations-warning dismissible-notice notice-${tone} ${className}`.trim()}
      aria-labelledby={headingID}
      role={role}
    >
      <Icon size={20} aria-hidden="true" />
      <div className="notice-copy">
        <strong id={headingID}>{title}</strong>
        {children}
      </div>
      {action && <div className="notice-action">{action}</div>}
      <button
        type="button"
        className="notice-dismiss"
        aria-label={`Скрыть уведомление «${title}»`}
        title="Скрыть уведомление"
        onClick={() => setDismissed(signature)}
      >
        <X size={18} aria-hidden="true" />
      </button>
    </section>
  );
}
