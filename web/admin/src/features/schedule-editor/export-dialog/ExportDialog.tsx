import dialogStyles from "../../../ui/dialog/Dialog.module.css";
import styles from "./ExportDialog.module.css";
import layoutStyles from "../../../ui/layout/Layout.module.css";
import formStyles from "../../../ui/form/Form.module.css";
import { useState } from "react";
import {
  CalendarRange,
  Download,
  FileJson,
  FileSpreadsheet,
  X,
} from "lucide-react";
import { DialogPortal, type ToastMessage } from "../../../ui";
import type { EditorSchedule } from "../../../shared/model";
import { downloadSchedule, type ScheduleExportFormat } from "../exportSchedule";
import { pluralLessons } from "../model";

const exportOptions = [
  {
    format: "json" as const,
    icon: FileJson,
    title: "JSON",
    text: "Полная структура и служебные поля",
  },
  {
    format: "csv" as const,
    icon: FileSpreadsheet,
    title: "CSV",
    text: "Таблица для Excel и других редакторов",
  },
  {
    format: "ics" as const,
    icon: CalendarRange,
    title: "iCalendar",
    text: "Разовый импорт на 112 дней; не обновляемая подписка",
  },
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
  const [from, setFrom] = useState(() =>
    (schedule.semesters[0]?.start_date ?? new Date().toISOString()).slice(
      0,
      10,
    ),
  );
  const [busy, setBusy] = useState(false);
  async function download(format: ScheduleExportFormat) {
    try {
      setBusy(true);
      await downloadSchedule(schedule, format, from);
      notify(`Расписание выгружено в ${format.toUpperCase()}`);
      onClose();
    } catch (caught) {
      notify(
        caught instanceof Error
          ? caught.message
          : "Не удалось выгрузить расписание",
        "error",
      );
    } finally {
      setBusy(false);
    }
  }

  return (
    <DialogPortal>
      <div className={dialogStyles["dialog-backdrop"]} role="presentation">
        <section
          className={styles["export-dialog"]}
          role="dialog"
          aria-modal="true"
          aria-labelledby="export-title"
        >
          <header>
            <div>
              <span className={layoutStyles["eyebrow"]}>Экспорт</span>
              <h2 id="export-title">Выгрузить расписание</h2>
              <p>
                Группа {schedule.group.name} · {schedule.lessons.length}{" "}
                {pluralLessons(schedule.lessons.length)}
              </p>
            </div>
            <button
              className={dialogStyles["dialog-close"]}
              data-dialog-dismiss
              onClick={onClose}
              aria-label="Закрыть"
            >
              <X size={18} />
            </button>
          </header>
          <label className={formStyles["field"]}>
            <span>ICS: 112 дней с даты</span>
            <input
              type="date"
              value={from}
              onChange={(event) => setFrom(event.target.value)}
            />
          </label>
          <div className={styles["export-options"]}>
            {exportOptions.map(({ format, icon: Icon, title, text }) => (
              <button
                key={format}
                disabled={busy || !from}
                onClick={() => void download(format)}
              >
                <span>
                  <Icon size={19} />
                </span>
                <div>
                  <strong>{title}</strong>
                  <p>{text}</p>
                </div>
                <Download size={17} />
              </button>
            ))}
          </div>
        </section>
      </div>
    </DialogPortal>
  );
}
