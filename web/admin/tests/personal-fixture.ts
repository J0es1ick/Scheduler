import { expect, type Page } from "@playwright/test";
import type { Note } from "../src/personal/Notes";
import { shiftDate } from "../src/personal/api";

export async function setup(page: Page, admin = false, options: { week?: boolean; teacher?: boolean } = {}) {
  await page.route("**/telegram-web-app.js", (route) => route.fulfill({ contentType: "text/javascript", body: "" }));
  await page.clock.install({ time: new Date("2026-10-05T06:00:00Z") });
  const target = {
    id: "group",
    role: options.teacher ? "teacher" : "student",
    name: options.teacher ? "Иванов Иван Иванович" : "4/147",
    university_name: "Ивановский энергетический университет",
    timezone: "Europe/Moscow",
    primary: true,
    share_url: options.teacher ? undefined : "https://t.me/scheduler_bot?start=group_token",
  };
  const original = {
    ID: "lesson",
    personal_key: "stable",
    Subject: "Теоретическая механика",
    Teacher: "Иванов Иван Иванович",
    Room: "Б-204",
    Type: "lecture",
    TimeStart: "09:00",
    TimeEnd: "10:30",
    Subgroup: 0,
  };
  const state = {
    notes: [] as Note[],
    noteRequests: [] as Record<string, unknown>[],
    notificationRequests: [] as Record<string, unknown>[],
    unpublished: false,
    empty: false,
    preferences: { changes: false, daily_enabled: false, daily_time: "06:00", daily_day_offset: 0, daily_weekdays: 127, daily_skip_empty: false, paused_until: "", reminder_enabled: false, reminder_minutes: 20, quiet_enabled: false, quiet_start: "22:00", quiet_end: "07:00", service_updates: null as boolean | null, version: "2026-10-05T05:00:00Z" },
    changes: [] as Record<string, unknown>[],
    requests: [] as Record<string, unknown>[],
    cancelled: false,
    room: original.Room,
    conflict: false,
    review: false,
    scheduleGate: undefined as Promise<void> | undefined,
    scheduleError: false,
    reviewRequests: [] as Record<string, unknown>[],
  };
  await page.route("**/api/personal/**", async (route) => {
    const url = new URL(route.request().url());
    let body: unknown;
    if (url.pathname.endsWith("/me"))
      body = {
        id: "42",
        name: "Иван",
        is_admin: admin,
        csrf_token: "personal-csrf",
        targets: [target],
      };
    else if (url.pathname.endsWith("/notifications")) {
      if (route.request().method() === "PUT") {
        expect(route.request().headers()["x-csrf-token"]).toBe("personal-csrf");
        const input = route.request().postDataJSON();
        state.notificationRequests.push(input);
        state.preferences = { ...input, version: "2026-10-05T06:01:00Z" };
      }
      body = { preferences: state.preferences, timezone: target.timezone, preview: state.preferences.daily_enabled ? [{ kind: "daily", at: "2026-10-05T17:00:00Z", date: "2026-10-06", label: "Расписание на завтра" }] : [] };
    } else if (url.pathname.includes("/notes")) {
      const method = route.request().method();
      if (method === "POST") {
        expect(route.request().headers()["x-csrf-token"]).toBe("personal-csrf");
        const input = route.request().postDataJSON(); state.noteRequests.push(input);
        const existing = state.notes.find((note) => note.id === input.id);
        const note = { ...existing, ...input, id: input.id || `note-${state.notes.length + 1}`, version: (existing?.version || 0) + 1, target_id: input.target_id || existing?.target_id || target.id, lesson_id: input.lesson_id || existing?.lesson_id || "lesson-0", lesson_date: input.date || existing?.lesson_date || "2026-10-05", basis: original, binding: "attached" as const, current_date: input.date || existing?.current_date || "2026-10-05", current_lesson_id: input.lesson_id || "lesson-0" };
        state.notes = [...state.notes.filter((item) => item.id !== note.id), note]; body = note;
      } else if (method === "DELETE") {
        state.notes = state.notes.filter((item) => item.id !== url.pathname.split("/").at(-1)); body = { ok: true };
      } else body = state.notes;
    } else if (url.pathname.endsWith("/agenda")) {
      body = { target, freshness: { State: state.unpublished ? "stale" : "current", LastSuccess: state.unpublished ? null : "2026-10-05T05:00:00Z" }, days: [{ date: "2026-10-05", lessons: state.empty ? [] : [{ original, lesson: original, cancelled: state.cancelled, group_name: "4/147" }] }, { date: "2026-10-06", lessons: state.empty ? [] : [{ original, lesson: { ...original, TimeStart: "10:10", TimeEnd: "11:40" }, cancelled: false }] }] };
    } else if (url.pathname.endsWith("/schedule")) {
      await state.scheduleGate;
      if (state.scheduleError) return route.fulfill({ status: 503, json: { error: "Временно недоступно" } });
      const date = url.searchParams.get("from") || "2026-10-05";
      body = {
        target,
        publication: "snapshot-2",
        freshness: { State: "current", LastSuccess: state.unpublished ? null : "2026-10-05T05:00:00Z" },
        patterns: [
          {
            group_id: "group",
            semester_id: "term",
            group_name: "4/147",
            kind: "biweekly",
            weeks: 12,
            period: 2,
            from: "2026-09-01",
            to: "2026-12-20",
          },
        ],
        ...(state.review
          ? {
            review: {
              kept: 2,
              dropped: 1,
              items: [
                {
                  id: "change",
                  version: 2,
                  subject: "Теоретическая механика",
                  kept: 2,
                  dropped: 1,
                },
              ],
            },
          }
          : {}),
        days: Array.from({ length: options.week ? 7 : 1 }, (_, index) => {
          const dayDate = shiftDate(date, index);
          const changes = state.changes.filter((change) => change.scope === "semester" || (change.scope === "selected" ? Array.isArray(change.dates) && change.dates.includes(dayDate) : change.date === dayDate));
          return {
          date: dayDate,
          lessons: state.empty || index === 5 ? [] : [
            {
              original: { ...original, ID: `lesson-${index}` },
              lesson: { ...original, Room: changes.length ? state.room : original.Room },
              group_name: "4/147",
              cancelled: changes.length > 0 && state.cancelled,
              changes,
              semester_end: "2026-12-31",
              can_repeat: true,
              repeats: [
                { lesson_id: "stable", date: "2026-10-05" },
                { lesson_id: "second", date: "2026-10-19" },
                { lesson_id: "third", date: "2026-11-02" },
              ],
            },
          ],
        }; }),
        changes: state.changes,
      };
    } else if (url.pathname.endsWith("/review")) {
      expect(route.request().headers()["x-csrf-token"]).toBe("personal-csrf");
      state.reviewRequests.push(route.request().postDataJSON());
      state.review = false;
      body = { ok: true };
    } else if (route.request().method() === "POST") {
      expect(route.request().headers()["x-csrf-token"]).toBe("personal-csrf");
      const input = route.request().postDataJSON();
      state.requests.push(input);
      if (state.conflict)
        return route.fulfill({
          status: 409,
          json: {
            error: "Правка уже изменилась. Обновите расписание и повторите",
          },
        });
      state.cancelled = input.cancelled;
      state.room = input.patch.room || original.Room;
      const change = {
        ...input,
        id: "change",
        version: 1,
        valid_from: input.date + "T00:00:00Z",
        valid_to:
          (input.scope === "day" ? input.date : "2026-12-31") + "T00:00:00Z",
        lesson_id: "stable",
      };
      state.changes = [change];
      body = change;
    } else {
      state.cancelled = false;
      state.room = original.Room;
      state.changes = [];
      body = { ok: true };
    }
    return route.fulfill({ json: body });
  });
  return state;
}
