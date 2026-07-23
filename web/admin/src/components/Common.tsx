import type { ReactNode } from "react";
import {
  AlertCircle,
  ArrowLeft,
  ArrowRight,
  Check,
  Clock3,
  LoaderCircle,
  Search,
  X,
} from "lucide-react";
import type { Pagination, SourceHealth } from "../types";

export const number = new Intl.NumberFormat("ru-RU");

export function formatDateTime(value?: string | null) {
  if (!value) return "ещё не запускался";
  return new Intl.DateTimeFormat("ru-RU", {
    day: "2-digit",
    month: "short",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(value));
}

export function formatDate(value?: string | null) {
  if (!value) return "—";
  return new Intl.DateTimeFormat("ru-RU", {
    day: "2-digit",
    month: "2-digit",
    year: "numeric",
  }).format(new Date(value));
}

export function formatDuration(milliseconds: number) {
  if (milliseconds < 1000) return `${milliseconds} мс`;
  const seconds = Math.round(milliseconds / 1000);
  if (seconds < 60) return `${seconds} сек`;
  const minutes = Math.floor(seconds / 60);
  return `${minutes} мин ${seconds % 60} сек`;
}

export function relativeTime(value?: string | null) {
  if (!value) return "нет данных";
  const delta = new Date(value).getTime() - Date.now();
  const absolute = Math.abs(delta);
  const formatter = new Intl.RelativeTimeFormat("ru-RU", { numeric: "auto" });
  if (absolute < 60_000)
    return formatter.format(Math.round(delta / 1000), "second");
  if (absolute < 3_600_000)
    return formatter.format(Math.round(delta / 60_000), "minute");
  if (absolute < 86_400_000)
    return formatter.format(Math.round(delta / 3_600_000), "hour");
  return formatter.format(Math.round(delta / 86_400_000), "day");
}

export function intervalLabel(seconds: number) {
  if (seconds % 86400 === 0) return `${seconds / 86400} дн.`;
  if (seconds % 3600 === 0) return `${seconds / 3600} ч.`;
  return `${Math.round(seconds / 60)} мин.`;
}

export function LogoMark({ compact = false }: { compact?: boolean }) {
  return (
    <div
      className={`logo-lockup ${compact ? "is-compact" : ""}`}
      aria-label="Scheduler Admin"
    >
      <div className="logo-mark" aria-hidden="true">
        <span className="logo-mark-line line-a" />
        <span className="logo-mark-line line-b" />
        <span className="logo-mark-dot" />
      </div>
      {!compact && (
        <div>
          <strong>Scheduler</strong>
          <span>админ-панель</span>
        </div>
      )}
    </div>
  );
}

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
      className={`source-glyph ${small ? "is-small" : ""}`}
      aria-hidden="true"
    >
      <span>{letters}</span>
      <i />
    </div>
  );
}

export function StatusPill({
  health,
  label,
}: {
  health: SourceHealth | "success" | "failed";
  label?: string;
}) {
  const text =
    label ??
    {
      healthy: "В норме",
      running: "Обновляется",
      error: "Ошибка",
      success: "Успешно",
      failed: "Ошибка",
    }[health];
  return (
    <span className={`status-pill status-${health}`}>
      {health === "running" ? (
        <LoaderCircle size={13} className="spin" />
      ) : health === "healthy" || health === "success" ? (
        <Check size={13} />
      ) : (
        <X size={13} />
      )}
      {text}
    </span>
  );
}

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
    <div className="section-title">
      <div>
        {eyebrow && <span className="eyebrow">{eyebrow}</span>}
        <h2>{title}</h2>
      </div>
      {action}
    </div>
  );
}

export function LoadingBlock({ rows = 3 }: { rows?: number }) {
  return (
    <div className="loading-stack" aria-label="Загрузка">
      {Array.from({ length: rows }).map((_, index) => (
        <div
          className="skeleton"
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
    <div className="state-block state-error">
      <AlertCircle size={24} />
      <div>
        <strong>Не удалось загрузить данные</strong>
        <p>{message}</p>
      </div>
      {retry && (
        <button className="button button-ghost" onClick={retry}>
          Повторить
        </button>
      )}
    </div>
  );
}

export function EmptyBlock({ title, text }: { title: string; text: string }) {
  return (
    <div className="state-block">
      <Clock3 size={24} />
      <div>
        <strong>{title}</strong>
        <p>{text}</p>
      </div>
    </div>
  );
}

export function SearchField({
  value,
  onChange,
  placeholder,
}: {
  value: string;
  onChange: (value: string) => void;
  placeholder: string;
}) {
  return (
    <label className="search-field">
      <Search size={18} />
      <input
        value={value}
        onChange={(event) => onChange(event.target.value)}
        placeholder={placeholder}
      />
      {value && (
        <button
          type="button"
          onClick={() => onChange("")}
          aria-label="Очистить"
        >
          <X size={15} />
        </button>
      )}
    </label>
  );
}

export function PaginationControls({
  pagination,
  onPage,
}: {
  pagination: Pagination;
  onPage: (page: number) => void;
}) {
  const pages = Math.max(1, Math.ceil(pagination.total / pagination.page_size));
  return (
    <div className="pagination">
      <span>{number.format(pagination.total)} записей</span>
      <div>
        <button
          disabled={pagination.page <= 1}
          onClick={() => onPage(pagination.page - 1)}
          aria-label="Назад"
        >
          <ArrowLeft size={17} />
        </button>
        <strong>{pagination.page}</strong>
        <span>/ {pages}</span>
        <button
          disabled={pagination.page >= pages}
          onClick={() => onPage(pagination.page + 1)}
          aria-label="Вперёд"
        >
          <ArrowRight size={17} />
        </button>
      </div>
    </div>
  );
}

export type ToastMessage = {
  id: number;
  text: string;
  tone: "success" | "error" | "info";
};

export function Toasts({ items }: { items: ToastMessage[] }) {
  return (
    <div className="toast-stack" aria-live="polite">
      {items.map((item) => (
        <div key={item.id} className={`toast toast-${item.tone}`}>
          {item.text}
        </div>
      ))}
    </div>
  );
}
