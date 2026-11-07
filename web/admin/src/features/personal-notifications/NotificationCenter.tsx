import { classNames } from "../../../../shared/classNames";
import styles from "./NotificationCenter.module.css";
import buttonStyles from "../../ui/button/Button.module.css";
import personalUIStyles from "../personal-shared/PersonalUI.module.css";
import { useEffect, useState } from "react";
import { Bell, Check, Moon, Send } from "lucide-react";
import { request, dateLabel } from "../personal-shared/api";

interface Preferences {
  changes: boolean;
  daily_enabled: boolean;
  daily_time: string;
  daily_day_offset: number;
  daily_weekdays: number;
  daily_skip_empty: boolean;
  paused_until: string;
  reminder_enabled: boolean;
  reminder_minutes: number;
  quiet_enabled: boolean;
  quiet_start: string;
  quiet_end: string;
  service_updates: boolean | null;
  version: string;
}
interface Settings { preferences: Preferences; timezone: string; preview: { kind: string; at: string; date: string; label: string }[] }
const weekdays = ["Пн", "Вт", "Ср", "Чт", "Пт", "Сб", "Вс"];

export function NotificationCenter() {
  const [data, setData] = useState<Settings>();
  const [draft, setDraft] = useState<Preferences>();
  const [error, setError] = useState("");
  const [saved, setSaved] = useState(false);
  const [busy, setBusy] = useState(false);
  const [retry, setRetry] = useState(0);
  useEffect(() => {
    let active = true;
    void request<Settings>("notifications").then((result) => { if (active) { setData(result); setDraft(result.preferences); setError(""); } }).catch((e: unknown) => { if (active) setError(e instanceof Error ? e.message : "Не удалось загрузить настройки"); });
    return () => { active = false; };
  }, [retry]);
  function update(fields: Partial<Preferences>) { setDraft((value) => value ? { ...value, ...fields } : value); setSaved(false); }
  async function save() {
    setBusy(true); setError(""); setSaved(false);
    try { const result = await request<Settings>("notifications", "PUT", draft); setData(result); setDraft(result.preferences); setSaved(true); }
    catch (e) { setError(e instanceof Error ? e.message : "Не удалось сохранить"); }
    finally { setBusy(false); }
  }
  if (!data || !draft) return <section className={styles["personal-settings"]}><p>{error || "Загружаем настройки…"}</p>{error && <button className={classNames(buttonStyles["button"], buttonStyles["button-ghost"])} onClick={() => setRetry((n) => n + 1)}>Повторить</button>}</section>;
  const p = data.preferences;
  const dirty = JSON.stringify(draft) !== JSON.stringify(p);
  return <section className={styles["personal-settings"]} aria-label="Уведомления">
    <div className={styles["personal-settings-summary"]}><h2><Bell size={22} /> Что будет приходить</h2><ul>
      <li>Изменения вашего расписания и подгруппы — {p.changes ? "сразу, с учётом тихих часов" : "выключены"}</li>
      <li>Расписание на {p.daily_day_offset ? "завтра" : "сегодня"} — {p.daily_enabled ? `${p.daily_time} · ${weekdays.filter((_, i) => p.daily_weekdays & (1 << i)).join(", ")}` : "выключено"}</li>
      <li>Напоминания — {p.reminder_enabled ? `за ${p.reminder_minutes} мин.` : "выключены"}</li>
      <li>Тихие часы — {p.quiet_enabled ? `${p.quiet_start}–${p.quiet_end}` : "выключены"}</li>
      <li>Новости Scheduler — {p.service_updates ? "включены" : "выключены"}</li>
      {p.paused_until && <li>Уведомления о расписании возобновятся {dateLabel(p.paused_until)}</li>}
    </ul><small>Все часы указаны по времени вуза: {data.timezone}</small></div>
    <form onSubmit={(e) => { e.preventDefault(); void save(); }}>
      <fieldset disabled={busy}>
        <legend>Расписание и напоминания</legend>
        <label className={personalUIStyles["personal-check"]}><input type="checkbox" checked={draft.changes} onChange={(e) => update({ changes: e.target.checked })} />Сообщать об изменениях расписания</label>
        <label className={personalUIStyles["personal-check"]}><input type="checkbox" checked={draft.daily_enabled} onChange={(e) => update({ daily_enabled: e.target.checked })} />Присылать расписание по времени</label>
        <div className={personalUIStyles["personal-form-grid"]}><label>Какое расписание<select value={draft.daily_day_offset} onChange={(e) => update({ daily_day_offset: Number(e.target.value) })}><option value={0}>На сегодня</option><option value={1}>На завтра</option></select></label><label>Время отправки<input type="time" required value={draft.daily_time} onChange={(e) => update({ daily_time: e.target.value })} /></label></div>
        <div className={styles["personal-weekdays"]} role="group" aria-label="Дни отправки">{weekdays.map((day, i) => <button type="button" key={day} aria-pressed={!!(draft.daily_weekdays & (1 << i))} onClick={() => update({ daily_weekdays: draft.daily_weekdays ^ (1 << i) })}>{day}</button>)}</div>
        <p className={personalUIStyles["personal-note"]}>Выбраны дни отправки сообщения. Для режима «на завтра» расписание относится к следующему дню.</p>
        <label className={personalUIStyles["personal-check"]}><input type="checkbox" checked={draft.daily_skip_empty} onChange={(e) => update({ daily_skip_empty: e.target.checked })} />Пропускать дни без занятий</label>
        <label className={personalUIStyles["personal-check"]}><input type="checkbox" checked={draft.reminder_enabled} onChange={(e) => update({ reminder_enabled: e.target.checked })} />Напоминать перед парами</label>
        <label>За сколько минут<input type="number" min={5} max={180} required value={draft.reminder_minutes} onChange={(e) => update({ reminder_minutes: Number(e.target.value) })} /></label>
      </fieldset>
      <fieldset disabled={busy}><legend><Moon size={18} /> Тишина и пауза</legend>
        <label className={personalUIStyles["personal-check"]}><input type="checkbox" checked={draft.quiet_enabled} onChange={(e) => update({ quiet_enabled: e.target.checked })} />Включить тихие часы</label>
        <div className={personalUIStyles["personal-form-grid"]}><label>С<input type="time" required value={draft.quiet_start} onChange={(e) => update({ quiet_start: e.target.value })} /></label><label>До<input type="time" required value={draft.quiet_end} onChange={(e) => update({ quiet_end: e.target.value })} /></label></div>
        <p className={personalUIStyles["personal-note"]}>Изменения подождут окончания тихих часов. Напоминания в это время пропускаются. Отправка расписания по времени — исключение: оно придёт в выбранный час.</p>
        <label>Пауза до даты — возобновить с<input type="date" value={draft.paused_until} onChange={(e) => update({ paused_until: e.target.value })} /></label>
        {draft.paused_until && <button type="button" className={buttonStyles["text-button"]} onClick={() => update({ paused_until: "" })}>Снять паузу</button>}
        <p className={personalUIStyles["personal-note"]}>Пауза действует на изменения расписания, ежедневную отправку и напоминания. Ответы поддержки и важные сообщения сервиса могут приходить.</p>
      </fieldset>
      <fieldset disabled={busy}><legend>Новости</legend><label className={personalUIStyles["personal-check"]}><input type="checkbox" checked={draft.service_updates === true} onChange={(e) => update({ service_updates: e.target.checked })} />Получать новости об обновлениях Scheduler</label></fieldset>
      {error && <p role="alert" className={personalUIStyles["personal-error"]}>{error} <button type="button" className={buttonStyles["text-button"]} onClick={() => setRetry((n) => n + 1)}>Загрузить сохранённые настройки</button></p>}
      <div className={styles["personal-save-bar"]}><button type="submit" className={classNames(buttonStyles["button"], buttonStyles["button-primary"])} disabled={busy || !dirty || !draft.daily_weekdays}>{busy ? "Сохраняем…" : "Сохранить настройки"}</button>{saved && <span role="status"><Check size={16} /> Сохранено</span>}</div>
    </form>
    <section className={styles["personal-preview"]}><h2><Send size={20} /> Ближайшие сообщения</h2><p className={personalUIStyles["personal-note"]}>Предпросмотр на 7 дней по сохранённым настройкам. Время может измениться вместе с расписанием.</p>{dirty && <p>Сохраните изменения, чтобы обновить предпросмотр.</p>}{!data.preview.length ? <p>Плановых сообщений в ближайшие 7 дней нет.</p> : <ol>{data.preview.map((item, i) => <li key={`${item.at}:${i}`}><time>{new Intl.DateTimeFormat("ru-RU", { timeZone: data.timezone, day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" }).format(new Date(item.at))}</time><span>{item.kind === "reminder" ? `Напоминание: ${item.label}` : `${item.label} · ${dateLabel(item.date)}`}</span></li>)}</ol>}</section>
  </section>;
}
