import type { PersonalLesson, Target } from "../personal-shared/api";

export interface Agenda {
  target: Target;
  days: { date: string; lessons: PersonalLesson[] }[];
  freshness: { State: string; LastSuccess: string | null } | null;
}

function minutes(clock: string) {
  const [hours, minute] = clock.split(":").map(Number);
  return hours * 60 + minute;
}

export function dayOverview(agenda: Agenda, now: Date) {
  const date = new Intl.DateTimeFormat("sv-SE", { timeZone: agenda.target.timezone }).format(now);
  const clock = new Intl.DateTimeFormat("en-GB", { timeZone: agenda.target.timezone, hour: "2-digit", minute: "2-digit", hourCycle: "h23" }).format(now);
  const lessons = (agenda.days.find((day) => day.date === date)?.lessons || []).filter((item) => !item.cancelled).sort((a, b) => a.lesson.TimeStart.localeCompare(b.lesson.TimeStart));
  const current = lessons.filter((item) => item.lesson.TimeStart <= clock && item.lesson.TimeEnd > clock);
  const next = agenda.days.flatMap((day) => day.lessons.filter((item) => !item.cancelled && (day.date > date || day.date === date && item.lesson.TimeStart > clock)).map((item) => ({ ...item, date: day.date }))).sort((a, b) => `${a.date} ${a.lesson.TimeStart}`.localeCompare(`${b.date} ${b.lesson.TimeStart}`))[0];
  const remaining = lessons.filter((item) => item.lesson.TimeEnd > clock);
  const lastEnd = remaining.map((item) => item.lesson.TimeEnd).sort().at(-1);
  const wait = next?.date === date ? minutes(next.lesson.TimeStart) - minutes(clock) : null;
  const gap = current.length && next?.date === date ? minutes(next.lesson.TimeStart) - Math.max(...current.map((item) => minutes(item.lesson.TimeEnd))) : null;
  const building = (room: string) => room.match(/(?:корпус|корп\.?|к\.)\s*([\p{L}\d-]+)/iu)?.[1].toLocaleLowerCase("ru");
  const currentRoom = current[0]?.lesson.Room || "";
  const nextRoom = next?.lesson.Room || "";
  const relocation = building(currentRoom) && building(nextRoom) && building(currentRoom) !== building(nextRoom);
  return { date, current, next, remaining, lastEnd, wait, gap, relocation, currentRoom, nextRoom };
}
