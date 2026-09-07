import { api } from "../../api";
import type { EditorSchedule } from "../../types";
import {
  datePart,
  days,
  lessonDay,
  lessonTypeLabels,
  recurrenceLabel,
} from "./model";

export type ScheduleExportFormat = "json" | "csv" | "ics";

export async function downloadSchedule(
  schedule: EditorSchedule,
  format: ScheduleExportFormat,
  from: string,
  days = 112,
) {
  const baseName = `schedule-${schedule.group.name}`
    .replace(/[<>:"/\\|?*]+/g, "-")
    .replace(/\s+/g, "-");
  let contents: string;
  let mimeType: string;

  if (format === "json") {
    contents = JSON.stringify(
      {
        exported_at: new Date().toISOString(),
        group: schedule.group,
        semesters: schedule.semesters,
        lessons: schedule.lessons,
        deleted_lessons: schedule.deleted_lessons,
      },
      null,
      2,
    );
    mimeType = "application/json;charset=utf-8";
  } else if (format === "csv") {
    contents = buildCSV(schedule);
    mimeType = "text/csv;charset=utf-8";
  } else {
    contents = (await api.editorCalendar(schedule.group.id, from, days))
      .content;
    mimeType = "text/calendar;charset=utf-8";
  }

  const url = URL.createObjectURL(new Blob([contents], { type: mimeType }));
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = `${baseName}.${format}`;
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  window.setTimeout(() => URL.revokeObjectURL(url), 1000);
}

function buildCSV(schedule: EditorSchedule) {
  const rows = [
    [
      "Группа",
      "День",
      "Дата",
      "Начало",
      "Окончание",
      "Неделя",
      "Предмет",
      "Тип",
      "Преподаватель",
      "Аудитория",
      "Подгруппа",
      "Действует с",
      "Действует до",
      "Источник",
    ],
    ...schedule.lessons.map((lesson) => [
      schedule.group.name,
      days[lessonDay(lesson) - 1],
      datePart(lesson.special_date),
      lesson.time_start,
      lesson.time_end,
      recurrenceLabel(lesson),
      lesson.subject,
      lessonTypeLabels[lesson.type] ?? lesson.type,
      lesson.teacher,
      lesson.room,
      lesson.subgroup ? String(lesson.subgroup) : "вся группа",
      datePart(lesson.valid_from),
      datePart(lesson.valid_to),
      lesson.origin === "manual" ? "ручная правка" : "сайт",
    ]),
  ];
  return `\uFEFF${rows.map((row) => row.map(csvCell).join(";")).join("\r\n")}`;
}

function csvCell(value: string) {
  return `"${String(/^[=+\-@\t\r]/.test(value) ? "'" + value : (value ?? "")).replaceAll('"', '""')}"`;
}
