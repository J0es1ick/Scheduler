import { useEffect, useState } from "react";
import { ArrowRight, Clock3, MapPin } from "lucide-react";
import { request, today, shiftDate, dateLabel, type PersonalLesson, type Target } from "./api";

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

export function NowNext({ target, revision }: { target: Target; revision: number }) {
  const [agenda, setAgenda] = useState<Agenda>();
  const [error, setError] = useState("");
  const [now, setNow] = useState(() => new Date());
  const [refresh, setRefresh] = useState(0);
  useEffect(() => {
    const clock = window.setInterval(() => setNow(new Date()), 30000);
    const reload = () => { if (!document.hidden) setRefresh((n) => n + 1); };
    document.addEventListener("visibilitychange", reload);
    return () => { window.clearInterval(clock); document.removeEventListener("visibilitychange", reload); };
  }, []);
  const localDay = today(target.timezone);
  useEffect(() => {
    let active = true;
    void request<Agenda>(`agenda?target=${encodeURIComponent(target.id)}`).then((value) => { if (active) { setAgenda(value); setError(""); } }).catch(() => { if (active) setError("Не удалось обновить сводку."); });
    return () => { active = false; };
  }, [target.id, revision, refresh, localDay]);
  if (!agenda) return <section className="personal-now" aria-label="Сейчас и дальше"><p>{error || "Собираем ваш день…"}</p>{error && <button className="text-button" onClick={() => setRefresh((n) => n + 1)}>Повторить</button>}</section>;
  const info = dayOverview(agenda, now);
  const published = !!agenda.freshness?.LastSuccess;
  const unavailable = !published && !agenda.days.some((day) => day.lessons.length);
  return <section className="personal-now" aria-label="Сейчас и дальше">
    <div className="personal-now-heading"><span className="personal-eyebrow"><Clock3 size={16} /> Сейчас и дальше</span><small>{target.timezone}</small></div>
    {unavailable ? <><h2>Расписание ещё не опубликовано</h2><p>Проверим занятия после обновления источника.</p></> : <>
      {info.current.length ? info.current.map((item) => <div key={item.original.ID}><h2>Сейчас: {item.lesson.Subject}</h2><p>Сейчас · до {item.lesson.TimeEnd}{target.role === "teacher" && item.group_name ? ` · ${item.group_name}` : ""}{item.lesson.Room ? ` · ${item.lesson.Room}` : ""}</p></div>) : <><h2>{info.remaining.length ? "До следующей пары" : "Сегодня занятий больше нет"}</h2><p>{info.wait !== null ? `${info.wait} мин.` : "Можно спланировать оставшуюся часть дня."}</p></>}
      {info.next ? <div className="personal-next"><ArrowRight size={20} /><div><small>{info.next.date === info.date ? "Следующая" : info.next.date === shiftDate(info.date, 1) ? "Завтра" : dateLabel(info.next.date)}</small><strong>{info.next.lesson.TimeStart} · {info.next.lesson.Subject}</strong><span><MapPin size={14} />{info.next.lesson.Room || "Аудитория не указана"}</span>{target.role === "teacher" && info.next.group_name && <span>Группа {info.next.group_name}</span>}</div></div> : <p>В ближайшие семь дней занятий не найдено.</p>}
      {info.gap !== null && <p>{info.gap >= 0 ? `Между занятиями ${info.gap} мин.` : `Занятия пересекаются на ${-info.gap} мин.`}</p>}
      {info.relocation && <p className="personal-transfer">Смена корпуса: {info.currentRoom} → {info.nextRoom}</p>}
      {!!info.remaining.length && <p className="personal-now-footer">Незавершённых занятий: {info.remaining.length}. Последнее заканчивается в {info.lastEnd}.</p>}
    </>}
    {published && agenda.freshness?.State !== "current" && <p className="personal-note">Показана последняя доступная версия. Источник сейчас не подтверждает актуальность.</p>}
    {error && <p role="status">{error} Показаны ранее загруженные данные. <button className="text-button" onClick={() => setRefresh((n) => n + 1)}>Повторить</button></p>}
  </section>;
}
