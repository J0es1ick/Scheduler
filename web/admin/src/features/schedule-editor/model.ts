import type { EditorLesson, SemesterOption } from "../../types";
import type { ScheduleWeekFilter } from "../schedule-shared/weekSections";

export const days = [
  "Понедельник",
  "Вторник",
  "Среда",
  "Четверг",
  "Пятница",
  "Суббота",
  "Воскресенье",
];

export const daysAfterPreposition = [
  "понедельник",
  "вторник",
  "среду",
  "четверг",
  "пятницу",
  "субботу",
  "воскресенье",
];

export const lessonTypes = [
  ["lecture", "Лекция"],
  ["practice", "Практика"],
  ["lab", "Лабораторная"],
  ["seminar", "Семинар"],
  ["exam", "Экзамен"],
  ["credit", "Зачёт"],
  ["consultation", "Консультация"],
  ["other", "Другое"],
] as const;

export const lessonTypeLabels: Record<string, string> = Object.fromEntries(lessonTypes);

export const weekLabels: Record<EditorLesson["week_type"], string> = {
  every: "Каждую неделю",
  odd: "Нечётная неделя",
  even: "Чётная неделя",
  date: "Точная дата",
};

export type WeekFilter = ScheduleWeekFilter;

export type LessonForm = {
  semester_id: string;
  day_of_week: number;
  special_date: string;
  time_start: string;
  time_end: string;
  week_type: EditorLesson["week_type"];
  subject: string;
  type: string;
  teacher: string;
  room: string;
  subgroup: number;
  valid_from: string;
  valid_to: string;
};

export function formFromLesson(
  lesson: EditorLesson | null,
  day: number,
  semesters: SemesterOption[],
): LessonForm {
  const semester = semesters[0];
  if (!lesson) {
    return {
      semester_id: semester?.id ?? "",
      day_of_week: day,
      special_date: "",
      time_start: "09:00",
      time_end: "10:30",
      week_type: "every",
      subject: "",
      type: "lecture",
      teacher: "",
      room: "",
      subgroup: 0,
      valid_from: datePart(semester?.start_date),
      valid_to: datePart(semester?.end_date),
    };
  }
  return {
    semester_id: lesson.semester_id,
    day_of_week: lessonDay(lesson),
    special_date: datePart(lesson.special_date),
    time_start: lesson.time_start,
    time_end: lesson.time_end,
    week_type: lesson.week_type,
    subject: lesson.subject,
    type: lesson.type,
    teacher: lesson.teacher,
    room: lesson.room,
    subgroup: lesson.subgroup,
    valid_from: datePart(lesson.valid_from),
    valid_to: datePart(lesson.valid_to),
  };
}

export function datePart(value?: string | null) {
  return value ? value.slice(0, 10) : "";
}

export function lessonDay(lesson: EditorLesson) {
  if (lesson.day_of_week) return lesson.day_of_week;
  if (!lesson.special_date) return 1;
  const day = new Date(`${datePart(lesson.special_date)}T12:00:00`).getDay();
  return day === 0 ? 7 : day;
}

export function pluralLessons(value: number) {
  const mod10 = value % 10;
  const mod100 = value % 100;
  if (mod10 === 1 && mod100 !== 11) return "занятие";
  if (mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14)) {
    return "занятия";
  }
  return "занятий";
}
