import { useEffect, useState } from "react";

type State = { id: string; updatedAt?: number; error?: string };
export function RefreshStatus() {
  const [failures, setFailures] = useState<Record<string, State>>({});
  useEffect(() => {
    const update = (event: Event) => {
      const detail = (event as CustomEvent<State>).detail;
      setFailures((current) => {
        const result = { ...current };
        if (detail.error) result[detail.id] = detail;
        else delete result[detail.id];
        return result;
      });
    };
    window.addEventListener("scheduler:refresh-status", update);
    return () => window.removeEventListener("scheduler:refresh-status", update);
  }, []);
  const items = Object.values(failures);
  return items.length ? (
    <aside className="refresh-warning" role="alert">
      <strong>Не удалось обновить данные</strong>
      {items.map((item) => (
        <p key={item.id}>
          {item.error} Последняя успешная загрузка:{" "}
          {new Date(item.updatedAt!).toLocaleString("ru-RU")}.
        </p>
      ))}
      <span>
        Проверьте соединение и повторите обновление в текущем разделе.
      </span>
    </aside>
  ) : null;
}
