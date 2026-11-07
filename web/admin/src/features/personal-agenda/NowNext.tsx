import { dayOverview, type Agenda } from "./model";
import styles from "./NowNext.module.css";
import buttonStyles from "../../ui/button/Button.module.css";
import personalUIStyles from "../personal-shared/PersonalUI.module.css";
import { useEffect, useState } from "react";
import { ArrowRight, Clock3, MapPin } from "lucide-react";
import { request, today, shiftDate, dateLabel, type Target } from "../personal-shared/api";

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
  if (!agenda) return <section className={styles["personal-now"]} aria-label="Сейчас и дальше"><p>{error || "Собираем ваш день…"}</p>{error && <button className={buttonStyles["text-button"]} onClick={() => setRefresh((n) => n + 1)}>Повторить</button>}</section>;
  const info = dayOverview(agenda, now);
  const published = !!agenda.freshness?.LastSuccess;
  const unavailable = !published && !agenda.days.some((day) => day.lessons.length);
  return <section className={styles["personal-now"]} aria-label="Сейчас и дальше">
    <div className={styles["personal-now-heading"]}><span className={personalUIStyles["personal-eyebrow"]}><Clock3 size={16} /> Сейчас и дальше</span><small>{target.timezone}</small></div>
    {unavailable ? <><h2>Расписание ещё не опубликовано</h2><p>Проверим занятия после обновления источника.</p></> : <>
      {info.current.length ? info.current.map((item) => <div key={item.original.ID}><h2>Сейчас: {item.lesson.Subject}</h2><p>Сейчас · до {item.lesson.TimeEnd}{target.role === "teacher" && item.group_name ? ` · ${item.group_name}` : ""}{item.lesson.Room ? ` · ${item.lesson.Room}` : ""}</p></div>) : <><h2>{info.remaining.length ? "До следующей пары" : "Сегодня занятий больше нет"}</h2><p>{info.wait !== null ? `${info.wait} мин.` : "Можно спланировать оставшуюся часть дня."}</p></>}
      {info.next ? <div className={styles["personal-next"]}><ArrowRight size={20} /><div><small>{info.next.date === info.date ? "Следующая" : info.next.date === shiftDate(info.date, 1) ? "Завтра" : dateLabel(info.next.date)}</small><strong>{info.next.lesson.TimeStart} · {info.next.lesson.Subject}</strong><span><MapPin size={14} />{info.next.lesson.Room || "Аудитория не указана"}</span>{target.role === "teacher" && info.next.group_name && <span>Группа {info.next.group_name}</span>}</div></div> : <p>В ближайшие семь дней занятий не найдено.</p>}
      {info.gap !== null && <p>{info.gap >= 0 ? `Между занятиями ${info.gap} мин.` : `Занятия пересекаются на ${-info.gap} мин.`}</p>}
      {info.relocation && <p className={styles["personal-transfer"]}>Смена корпуса: {info.currentRoom} → {info.nextRoom}</p>}
      {!!info.remaining.length && <p className={styles["personal-now-footer"]}>Незавершённых занятий: {info.remaining.length}. Последнее заканчивается в {info.lastEnd}.</p>}
    </>}
    {published && agenda.freshness?.State !== "current" && <p className={personalUIStyles["personal-note"]}>Показана последняя доступная версия. Источник сейчас не подтверждает актуальность.</p>}
    {error && <p role="status">{error} Показаны ранее загруженные данные. <button className={buttonStyles["text-button"]} onClick={() => setRefresh((n) => n + 1)}>Повторить</button></p>}
  </section>;
}
