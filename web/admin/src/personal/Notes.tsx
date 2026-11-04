import { useEffect, useRef, useState } from "react";
import { Check, NotebookPen, Trash2, X } from "lucide-react";
import { dateLabel, request, type Lesson, type PersonalLesson, type Target } from "./api";
import type { Agenda } from "./NowNext";

export interface Note {
  id: string;
  version: number;
  target_id: string;
  lesson_id: string;
  lesson_date: string;
  basis: Lesson;
  body: string;
  due_date: string | null;
  completed: boolean;
  binding: "attached" | "moved" | "cancelled" | "needs_review";
  current_date?: string;
  current_lesson_id?: string;
}
interface NoteSelection { target: string; date: string; item: PersonalLesson }

function NoteBody({ body }: { body: string }) {
  return <p className="personal-note-body">{body.split(/(https?:\/\/[^\s<>]+)/gu).map((part, index) => /^https?:\/\//u.test(part) ? <a key={index} href={part} target="_blank" rel="noopener noreferrer">{part}</a> : part)}</p>;
}

export function NoteEditor({ note, selection, onClose, onSaved }: { note?: Note; selection?: NoteSelection; onClose: () => void; onSaved: () => void }) {
  const dialog = useRef<HTMLDialogElement>(null);
  const [body, setBody] = useState(note?.body || "");
  const [due, setDue] = useState(note?.due_date?.slice(0, 10) || "");
  const [done, setDone] = useState(note?.completed || false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => {
    const element = dialog.current;
    const previous = document.activeElement as HTMLElement | null;
    element?.showModal();
    window.dispatchEvent(new Event("scheduler:dialog-state"));
    return () => { element?.close(); previous?.focus(); window.dispatchEvent(new Event("scheduler:dialog-state")); };
  }, []);
  function close() {
    const changed = body !== (note?.body || "") || due !== (note?.due_date?.slice(0, 10) || "") || done !== (note?.completed || false);
    if (!busy && (!changed || window.confirm("Закрыть заметку без сохранения изменений?"))) onClose();
  }
  async function save() {
    setBusy(true); setError("");
    try {
      await request("notes", "POST", { id: note?.id || "", version: note?.version || 0, body, due_date: due, completed: done, ...(selection ? { target_id: selection.target, lesson_id: selection.item.original.ID, date: selection.date, rebind: !!note } : {}) });
      onSaved();
    } catch (e) { setError(e instanceof Error ? e.message : "Не удалось сохранить заметку"); }
    finally { setBusy(false); }
  }
  return <dialog ref={dialog} className="personal-dialog personal-note-dialog" aria-labelledby="personal-note-title" onCancel={(e) => { e.preventDefault(); close(); }}>
    <form onSubmit={(e) => { e.preventDefault(); void save(); }}>
      <div className="personal-note-editor-heading"><h2 id="personal-note-title">{note ? "Заметка к занятию" : "Новая заметка"}</h2><button type="button" className="button button-ghost" aria-label="Закрыть заметку" disabled={busy} onClick={close}><X size={18} /></button></div>
      <p>{selection?.item.lesson.Subject || note?.basis.Subject} · {dateLabel(selection?.date || note?.lesson_date || "")}</p>
      {selection && note && <p className="personal-note">После сохранения заметка будет привязана к выбранной паре.</p>}
      <label>Задание, заметка или ссылка<textarea autoFocus maxLength={4000} rows={6} required value={body} onChange={(e) => setBody(e.target.value)} disabled={busy} /></label>
      <label>Срок выполнения<input type="date" value={due} onChange={(e) => setDue(e.target.value)} disabled={busy} /></label>
      <label className="personal-check"><input type="checkbox" checked={done} onChange={(e) => setDone(e.target.checked)} disabled={busy} />Готово</label>
      <p className="personal-note">Эта запись видна только вам. Отмена пары не удаляет задание. Напоминания о сроке автоматически не включаются.</p>
      {error && <p role="alert" className="personal-error">{error}</p>}
      <div className="personal-note-actions"><button type="button" className="button button-ghost" disabled={busy} onClick={close}>Отмена</button><button className="button button-primary" disabled={busy || !body.trim()}>{busy ? "Сохраняем…" : "Сохранить"}</button></div>
    </form>
  </dialog>;
}

export function Notes({ targets }: { targets: Target[] }) {
  const [notes, setNotes] = useState<Note[]>();
  const [error, setError] = useState("");
  const [revision, setRevision] = useState(0);
  const [editing, setEditing] = useState<Note>();
  const [binding, setBinding] = useState<Note>();
  const [selection, setSelection] = useState<NoteSelection>();
  const [showCompleted, setShowCompleted] = useState(false);
  const [busy, setBusy] = useState("");
  useEffect(() => {
    let active = true;
    void request<Note[]>("notes").then((items) => { if (active) { setNotes(items); setError(""); } }).catch((e: unknown) => { if (active) setError(e instanceof Error ? e.message : "Не удалось загрузить заметки"); });
    return () => { active = false; };
  }, [revision]);
  async function toggle(note: Note) {
    setBusy(note.id); setError("");
    try { await request("notes", "POST", { id: note.id, version: note.version, body: note.body, due_date: note.due_date?.slice(0, 10) || "", completed: !note.completed }); setRevision((n) => n + 1); }
    catch (e) { setError(e instanceof Error ? e.message : "Не удалось изменить заметку"); }
    finally { setBusy(""); }
  }
  async function remove(note: Note) {
    if (!window.confirm("Удалить эту заметку?")) return;
    setBusy(note.id); setError("");
    try { await request(`notes/${encodeURIComponent(note.id)}?version=${note.version}`, "DELETE"); setRevision((n) => n + 1); }
    catch (e) { setError(e instanceof Error ? e.message : "Не удалось удалить"); }
    finally { setBusy(""); }
  }
  const visible = notes?.filter((note) => showCompleted || !note.completed);
  return <section className="personal-notes" aria-label="Мои заметки">
    <h2><NotebookPen size={22} /> Заметки и задания</h2><p>Откройте нужную пару в расписании и нажмите «Заметка». Записи остаются здесь даже после отмены занятия или смены группы.</p>
    <label className="personal-check"><input type="checkbox" checked={showCompleted} onChange={(e) => setShowCompleted(e.target.checked)} />Показывать выполненные</label>
    {error && <p role="alert" className="personal-error">{error} <button className="text-button" onClick={() => setRevision((n) => n + 1)}>Обновить</button></p>}
    {!notes && !error && <p>Загружаем заметки…</p>}
    {visible?.length === 0 && <p className="personal-empty-day">{notes?.length ? "Все задания выполнены" : "Пока нет заметок"}</p>}
    {visible?.map((note) => <article className={`personal-note-card${note.completed ? " is-completed" : ""}`} key={note.id}>
      <div className="personal-note-editor-heading"><strong>{note.basis.Subject}</strong><button className="button button-ghost" aria-label={note.completed ? "Вернуть в работу" : "Отметить готовым"} disabled={!!busy} onClick={() => void toggle(note)}><Check size={17} />{note.completed ? "Готово" : "Завершить"}</button></div>
      <small>{dateLabel(note.current_date || note.lesson_date)}{note.due_date && ` · срок: ${dateLabel(note.due_date)}`}</small>
      <NoteBody body={note.body} />
      {note.binding === "moved" && <p className="personal-tag">Пара перенесена. Заметка сохранена у нового занятия.</p>}
      {note.binding === "cancelled" && <p className="personal-note">Пара отменена для вас. Задание сохранено.</p>}
      {note.binding === "needs_review" && <p className="personal-review-note">Занятие изменилось или недоступно. Проверьте привязку: запись и срок сохранены.</p>}
      <div className="personal-note-actions"><button className="button button-ghost" disabled={!!busy} onClick={() => { setSelection(undefined); setEditing(note); }}>Изменить</button><button className="button button-ghost" disabled={!!busy || !targets.length} onClick={() => setBinding(note)}>Выбрать пару</button><button className="button button-ghost" aria-label={`Удалить заметку: ${note.basis.Subject}`} disabled={!!busy} onClick={() => void remove(note)}><Trash2 size={16} /></button></div>
    </article>)}
    {binding && <RebindNote note={binding} targets={targets} onCancel={() => setBinding(undefined)} onSelect={(value) => { setSelection(value); setEditing(binding); setBinding(undefined); }} />}
    {editing && <NoteEditor key={`${editing.id}:${selection?.date || ""}`} note={editing} selection={selection} onClose={() => setEditing(undefined)} onSaved={() => { setEditing(undefined); setRevision((n) => n + 1); }} />}
  </section>;
}

function RebindNote({ note, targets, onCancel, onSelect }: { note: Note; targets: Target[]; onCancel: () => void; onSelect: (value: NoteSelection) => void }) {
  const dialog = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    const element = dialog.current;
    const previous = document.activeElement as HTMLElement | null;
    element?.showModal();
    window.dispatchEvent(new Event("scheduler:dialog-state"));
    return () => { element?.close(); previous?.focus(); window.dispatchEvent(new Event("scheduler:dialog-state")); };
  }, []);
  const [target, setTarget] = useState(targets.find((item) => item.id === note.target_id)?.id || targets[0]?.id || "");
  const [date, setDate] = useState(note.current_date || note.lesson_date.slice(0, 10));
  const [loaded, setLoaded] = useState<{ key: string; agenda?: Agenda; error?: string }>();
  const key = `${target}:${date}`;
  useEffect(() => {
    let active = true;
    void request<Agenda>(`schedule?${new URLSearchParams({ target, from: date, to: date })}`).then((agenda) => { if (active) setLoaded({ key, agenda }); }).catch((e: unknown) => { if (active) setLoaded({ key, error: e instanceof Error ? e.message : "Не удалось загрузить занятия" }); });
    return () => { active = false; };
  }, [target, date, key]);
  return <dialog ref={dialog} className="personal-dialog personal-note-dialog personal-rebind" aria-label="Перепривязать заметку" onCancel={(event) => { event.preventDefault(); onCancel(); }}><div className="personal-note-editor-heading"><h2>Выберите занятие для заметки</h2><button className="button button-ghost" aria-label="Закрыть выбор пары" onClick={onCancel}><X size={18} /></button></div><div className="personal-form-grid"><label>Расписание<select value={target} onChange={(e) => setTarget(e.target.value)}>{targets.map((item) => <option key={item.id} value={item.id}>{item.university_name} · {item.name}</option>)}</select></label><label>Дата<input type="date" value={date} onChange={(e) => { if (e.target.value) setDate(e.target.value); }} /></label></div>{loaded?.key !== key ? <p>Загружаем…</p> : loaded.error ? <p role="alert">{loaded.error}</p> : <div className="personal-rebind-options">{!loaded.agenda?.days[0]?.lessons.length && <p>В этот день занятий нет.</p>}{loaded.agenda?.days[0]?.lessons.map((item) => <button className="button button-ghost" key={item.original.ID} onClick={() => onSelect({ target, date, item })}>{item.lesson.TimeStart} · {item.lesson.Subject}</button>)}</div>}<button className="text-button" onClick={onCancel}>Отмена</button></dialog>;
}
