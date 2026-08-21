import type { SnapshotGroupStatus, SnapshotLesson } from "../../types";

export const snapshotDays = [
  "Понедельник",
  "Вторник",
  "Среда",
  "Четверг",
  "Пятница",
  "Суббота",
  "Воскресенье",
];

export const snapshotGroupStatusLabels: Record<SnapshotGroupStatus, string> = {
  added: "Новая группа",
  removed: "Исчезла",
  changed: "Изменена",
  unchanged: "Без изменений",
};

export const snapshotWeekLabels: Record<SnapshotLesson["week_type"], string> = {
  every: "Каждую неделю",
  odd: "Нечётная неделя",
  even: "Чётная неделя",
  date: "Точная дата",
};

export const snapshotLessonTypeLabels: Record<string, string> = {
  lecture: "Лекция",
  practice: "Практика",
  lab: "Лабораторная",
  seminar: "Семинар",
  exam: "Экзамен",
  credit: "Зачёт",
  consultation: "Консультация",
  other: "Другое",
};

export function snapshotLessonDay(lesson: SnapshotLesson) {
  if (lesson.day_of_week) return lesson.day_of_week;
  if (!lesson.special_date) return 1;
  const day = new Date(`${lesson.special_date.slice(0, 10)}T12:00:00`).getDay();
  return day === 0 ? 7 : day;
}

export function formatSnapshotDate(value?: string | null) {
  if (!value || value.startsWith("0001-01-01")) return "не указана";
  return new Intl.DateTimeFormat("ru-RU", {
    day: "2-digit",
    month: "2-digit",
    year: "numeric",
  }).format(new Date(value));
}
