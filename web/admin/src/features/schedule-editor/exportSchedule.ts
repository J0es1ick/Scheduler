import type { EditorLesson, EditorSchedule, SemesterOption } from "../../types";
import { datePart, days, lessonDay, lessonTypeLabels, weekLabels } from "./model";

export type ScheduleExportFormat = "json" | "csv" | "ics";

export function downloadSchedule(schedule: EditorSchedule, format: ScheduleExportFormat) {
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
    contents = buildCalendar(schedule);
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
      "Группа", "День", "Дата", "Начало", "Окончание", "Неделя", "Предмет",
      "Тип", "Преподаватель", "Аудитория", "Подгруппа", "Действует с",
      "Действует до", "Источник",
    ],
    ...schedule.lessons.map((lesson) => [
      schedule.group.name,
      days[lessonDay(lesson) - 1],
      datePart(lesson.special_date),
      lesson.time_start,
      lesson.time_end,
      weekLabels[lesson.week_type],
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

function buildCalendar(schedule: EditorSchedule) {
  const semester = schedule.semesters[0];
  const now = new Date().toISOString().replace(/[-:]/g, "").replace(/\.\d{3}Z$/, "Z");
  const events = schedule.lessons.flatMap((lesson) => {
    const firstDate = firstLessonDate(lesson, semester);
    if (!firstDate) return [];
    const lines = [
      "BEGIN:VEVENT",
      `UID:${icsEscape(lesson.id)}@scheduler`,
      `DTSTAMP:${now}`,
      `DTSTART:${icsDateTime(firstDate, lesson.time_start)}`,
      `DTEND:${icsDateTime(firstDate, lesson.time_end)}`,
      `SUMMARY:${icsEscape(lesson.subject)}`,
    ];
    if (lesson.room) lines.push(`LOCATION:${icsEscape(lesson.room)}`);
    const description = [
      lesson.teacher,
      lessonTypeLabels[lesson.type],
      lesson.subgroup ? `Подгруппа ${lesson.subgroup}` : "",
    ].filter(Boolean).join(" · ");
    if (description) lines.push(`DESCRIPTION:${icsEscape(description)}`);
    if (lesson.week_type !== "date") {
      const interval = lesson.week_type === "every" ? 1 : 2;
      const until = datePart(lesson.valid_to) || datePart(semester?.end_date);
      lines.push(`RRULE:FREQ=WEEKLY;INTERVAL=${interval}${until ? `;UNTIL=${until.replaceAll("-", "")}T235959` : ""}`);
    }
    lines.push("END:VEVENT");
    return lines;
  });
  return [
    "BEGIN:VCALENDAR", "VERSION:2.0", "PRODID:-//Scheduler//Admin export//RU",
    "CALSCALE:GREGORIAN", ...events, "END:VCALENDAR", "",
  ].join("\r\n");
}

function csvCell(value: string) {
  return `"${String(value ?? "").replaceAll('"', '""')}"`;
}

function icsEscape(value: string) {
  return value.replaceAll("\\", "\\\\").replaceAll("\n", "\\n").replaceAll(",", "\\,").replaceAll(";", "\\;");
}

function icsDateTime(date: string, time: string) {
  return `${date.replaceAll("-", "")}T${time.replace(":", "")}00`;
}

function firstLessonDate(lesson: EditorLesson, semester?: SemesterOption) {
  if (lesson.special_date) return datePart(lesson.special_date);
  const initial = datePart(lesson.valid_from) || datePart(semester?.start_date);
  if (!initial) return "";
  const cursor = new Date(`${initial}T12:00:00`);
  const target = lessonDay(lesson) % 7;
  while (cursor.getDay() !== target) cursor.setDate(cursor.getDate() + 1);
  return `${cursor.getFullYear()}-${String(cursor.getMonth() + 1).padStart(2, "0")}-${String(cursor.getDate()).padStart(2, "0")}`;
}
