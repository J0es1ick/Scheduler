import { CalendarRange, Download, FileJson, FileSpreadsheet, X } from "lucide-react";
import type { ToastMessage } from "../../../components";
import type { EditorSchedule } from "../../../types";
import { downloadSchedule, type ScheduleExportFormat } from "../exportSchedule";
import { pluralLessons } from "../model";

const exportOptions = [
  { format: "json" as const, icon: FileJson, title: "JSON", text: "Полная структура и служебные поля" },
  { format: "csv" as const, icon: FileSpreadsheet, title: "CSV", text: "Таблица для Excel и других редакторов" },
  { format: "ics" as const, icon: CalendarRange, title: "iCalendar", text: "Импорт в календарное приложение" },
];

export function ExportDialog({
  schedule,
  onClose,
  notify,
}: {
  schedule: EditorSchedule;
  onClose: () => void;
  notify: (text: string, tone?: ToastMessage["tone"]) => void;
}) {
  function download(format: ScheduleExportFormat) {
    try {
      downloadSchedule(schedule, format);
      notify(`Расписание выгружено в ${format.toUpperCase()}`);
      onClose();
    } catch (caught) {
      notify(caught instanceof Error ? caught.message : "Не удалось выгрузить расписание", "error");
    }
  }

  return (
    <div className="dialog-backdrop" role="presentation">
      <section className="export-dialog" role="dialog" aria-modal="true" aria-labelledby="export-title">
        <header>
          <div>
            <span className="eyebrow">Резервная копия</span>
            <h2 id="export-title">Выгрузить расписание</h2>
            <p>Группа {schedule.group.name} · {schedule.lessons.length} {pluralLessons(schedule.lessons.length)}</p>
          </div>
          <button className="dialog-close" onClick={onClose} aria-label="Закрыть">
            <X size={18} />
          </button>
        </header>
        <div className="export-options">
          {exportOptions.map(({ format, icon: Icon, title, text }) => (
            <button key={format} onClick={() => download(format)}>
              <span><Icon size={19} /></span>
              <div><strong>{title}</strong><p>{text}</p></div>
              <Download size={17} />
            </button>
          ))}
        </div>
      </section>
    </div>
  );
}
