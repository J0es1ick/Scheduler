export interface Target {
  id: string;
  role: "student" | "teacher";
  name: string;
  university_name: string;
  timezone: string;
  primary: boolean;
}
export interface Profile {
  id: string;
  name: string;
  is_admin: boolean;
  csrf_token: string;
  targets: Target[];
}
export interface Lesson {
  ID: string;
  personal_key?: string;
  Subject: string;
  Teacher: string;
  Room: string;
  Type: string;
  TimeStart: string;
  TimeEnd: string;
  Subgroup: number;
}
export interface Patch {
  subject?: string;
  teacher?: string;
  room?: string;
  type?: string;
  time_start?: string;
  time_end?: string;
}
export interface Change {
  id: string;
  version: number;
  lesson_id: string;
  scope: "day" | "semester";
  valid_from: string;
  valid_to: string;
  patch: Patch;
  cancelled: boolean;
}
export interface PersonalLesson {
  lesson: Lesson;
  original: Lesson;
  cancelled: boolean;
  changes: Change[];
  semester_end: string;
  can_repeat: boolean;
  group_name: string;
}
export interface Schedule {
  target: Target;
  days: { date: string; lessons: PersonalLesson[] }[];
  changes: Change[];
}
export class PersonalError extends Error {
  constructor(
    message: string,
    readonly status: number,
  ) {
    super(message);
  }
}
let csrf = "";
export async function request<T>(
  path: string,
  method = "GET",
  body?: unknown,
): Promise<T> {
  const response = await fetch(`/api/personal/${path}`, {
    method,
    credentials: "same-origin",
    signal: AbortSignal.timeout(25000),
    headers: {
      "Content-Type": "application/json",
      ...(method === "GET" ? {} : { "X-CSRF-Token": csrf }),
    },
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  });
  const result = await response.json();
  if (!response.ok)
    throw new PersonalError(
      result.error || "Не удалось выполнить запрос",
      response.status,
    );
  return result as T;
}
export async function login(): Promise<Profile> {
  let profile: Profile;
  try {
    profile = await request<Profile>("me");
  } catch (error) {
    if (!(error instanceof PersonalError) || error.status !== 401) throw error;
    const initData = window.Telegram?.WebApp?.initData;
    if (!initData)
      throw new Error(
        "Откройте приложение кнопкой «Расписание» в Telegram-боте.",
        { cause: error },
      );
    profile = await request<Profile>("auth/telegram", "POST", {
      init_data: initData,
    });
  }
  csrf = profile.csrf_token;
  return profile;
}
export function dateLabel(date: string): string {
  return new Date(`${date.slice(0, 10)}T12:00:00`).toLocaleDateString("ru-RU", {
    day: "numeric",
    month: "long",
  });
}
export function shiftDate(date: string, days: number): string {
  const value = new Date(`${date}T12:00:00Z`);
  value.setUTCDate(value.getUTCDate() + days);
  return value.toISOString().slice(0, 10);
}
export function today(timezone: string): string {
  return new Intl.DateTimeFormat("sv-SE", { timeZone: timezone }).format(
    new Date(),
  );
}
export const lessonTypes: Record<string, string> = {
  lecture: "Лекция",
  practice: "Практика",
  lab: "Лабораторная",
  seminar: "Семинар",
  exam: "Экзамен",
  credit: "Зачёт",
  consultation: "Консультация",
  other: "Занятие",
};
