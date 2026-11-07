

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
