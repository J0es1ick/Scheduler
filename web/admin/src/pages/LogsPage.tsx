import { useViewState } from "../hooks/useViewState";
import type { AdminRole } from "../types";
import { ServiceLogs } from "./ServiceLogs";
import { ParserRuns } from "./ParserRuns";
import { LogDelivery } from "./LogDelivery";
import { LogTabs } from "./LogTabs";

export function LogsPage({ role }: { role: AdminRole }) {
  const canReadLogs = role === "owner" || role === "operator";
  const [selection, setSelection] = useViewState(
    "LogsPage:tab",
    canReadLogs ? "logs" : "parsers",
  );
  const tab = selection === "logs" && !canReadLogs ? "parsers" : selection;
  const options = [
    ...(canReadLogs ? [{ value: "logs", label: "Логи" }] : []),
    { value: "parsers", label: "Запуски парсеров" },
    { value: "delivery", label: "Доставка" },
  ];
  return (
    <div className="page-stack logs-page">
      <LogTabs
        id="diagnostic"
        label="Разделы диагностики"
        options={options}
        value={tab}
        onChange={setSelection}
      />
      <div
        role="tabpanel"
        id={`diagnostic-panel-${tab}`}
        aria-labelledby={`diagnostic-tab-${tab}`}
        className="log-tab-panel"
      >
        {tab === "logs" ? (
          <ServiceLogs />
        ) : tab === "parsers" ? (
          <ParserRuns />
        ) : (
          <LogDelivery />
        )}
      </div>
    </div>
  );
}
