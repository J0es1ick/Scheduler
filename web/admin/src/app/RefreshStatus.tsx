import { useEffect, useState } from "react";
import { AlertCircle } from "lucide-react";
import { DismissibleNotice } from "../components/DismissibleNotice";

type State = { id: string; updatedAt?: number; error?: string; loading?: boolean };
export function RefreshStatus() {
  const [failures, setFailures] = useState<Record<string, State>>({});
  useEffect(() => {
    const update = (event: Event) => {
      const detail = (event as CustomEvent<State>).detail;
      if (detail.loading) return;
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
  const items = Object.values(failures).sort((a, b) => a.id.localeCompare(b.id));
  return (
    <DismissibleNotice
      id="refresh-errors"
      signature={items.length ? JSON.stringify(items.map((item) => [item.id, item.error])) : null}
      title="Не удалось обновить данные"
      className="refresh-warning"
      role="alert"
      icon={AlertCircle}
    >
      {items.map((item) => (
        <p key={item.id}>
          {item.error} Последняя успешная загрузка:{" "}
          {new Date(item.updatedAt!).toLocaleString("ru-RU")}.
        </p>
      ))}
      <span>
        Проверьте соединение и повторите обновление в текущем разделе.
      </span>
    </DismissibleNotice>
  );
}
