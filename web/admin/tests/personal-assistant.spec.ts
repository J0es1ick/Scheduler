import { expect, test } from "@playwright/test";
import { setup } from "./personal-fixture";
import { dayOverview, type Agenda } from "../src/features/personal-agenda/model";

test("day overview follows local time without repeated network requests", async ({ page }) => {
  await setup(page);
  let requests = 0; page.on("request", (request) => { if (request.url().includes("/agenda?")) requests++; });
  await page.goto("/app");
  const overview = page.getByRole("region", { name: "Сейчас и дальше" });
  await expect(overview).toContainText("Сейчас: Теоретическая механика");
  await expect(overview).toContainText("Завтра");
  await expect(overview).toContainText("10:10");
  const initial = requests;
  await page.clock.fastForward("02:00:00");
  await expect(overview).toContainText("Сегодня занятий больше нет");
  expect(requests).toBe(initial);
});

test("unpublished schedule is distinct from a free day", async ({ page }) => {
  const state = await setup(page); state.empty = true; state.unpublished = true;
  await page.goto("/app");
  await expect(page.getByRole("region", { name: "Сейчас и дальше" })).toContainText("Расписание ещё не опубликовано");
  await expect(page.getByText("Занятий нет", { exact: true })).toHaveCount(0);
});

test("notification settings preserve opt-outs and preview saved tomorrow settings", async ({ page }) => {
  await page.setViewportSize({ width: 375, height: 812 });
  const state = await setup(page,true);
  await page.goto("/app?section=notifications");
  await expect(page.getByRole("heading", { name: "Что будет приходить" })).toBeVisible();
  await page.getByLabel("Присылать расписание по времени").check();
  await page.getByLabel("Какое расписание").selectOption("1");
  await page.getByLabel("Время отправки").fill("20:00");
  await page.getByRole("button", { name: "Сб", exact: true }).click();
  await page.getByRole("button", { name: "Вс", exact: true }).click();
  await page.getByLabel("Пропускать дни без занятий").check();
  await page.getByRole("button", { name: "Сохранить настройки" }).click();
  await expect(page.getByRole("status")).toHaveText("Сохранено");
  expect(state.notificationRequests[0]).toMatchObject({ daily_enabled:true,daily_day_offset:1,daily_time:"20:00",daily_weekdays:31,daily_skip_empty:true,changes:false,reminder_enabled:false,service_updates:null });
  await expect(page.locator("[class*=\"_personal-preview_\"]")).toContainText("Расписание на завтра");
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

test("notes can be created, completed and rebound on a phone", async ({ page }, testInfo) => {
  await page.setViewportSize({ width: 375, height: 812 });
  const state = await setup(page);
  await page.goto("/app");
  await page.getByRole("button", { name: "Заметка", exact: true }).click();
  await page.getByLabel("Задание, заметка или ссылка").fill("Сдать отчёт https://example.com/materials");
  await page.getByLabel("Срок выполнения").fill("2026-10-09");
  await page.getByRole("button", { name: "Сохранить", exact: true }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await page.getByRole("navigation", { name: "Личное расписание" }).getByRole("button", { name: "Заметки", exact: true }).click();
  await expect(page.getByRole("link", { name: "https://example.com/materials" })).toHaveAttribute("rel", "noopener noreferrer");
  await page.getByRole("button", { name: "Выбрать пару" }).click();
  const rebind = page.getByRole("dialog", { name: "Перепривязать заметку" });
  await expect(rebind).toBeVisible();
  await rebind.getByLabel("Дата").fill("2026-10-06");
  await rebind.getByRole("button", { name: "09:00 · Теоретическая механика" }).click();
  await page.getByRole("button", { name: "Сохранить", exact: true }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  expect(state.noteRequests.at(-1)).toMatchObject({ rebind:true,date:"2026-10-06" });
  await page.screenshot({ path:testInfo.outputPath("notes-mobile.png"),fullPage:true });
  await page.getByRole("button", { name: "Отметить готовым" }).click();
  await expect(page.getByText("Все задания выполнены")).toBeVisible();
  await page.getByLabel("Показывать выполненные").check();
  await expect(page.getByRole("button", { name: "Вернуть в работу" })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

test("sharing contains only a group invitation", async ({ page }) => {
  await setup(page); await page.goto("/app");
  const link=page.getByRole("link", { name:"Поделиться группой" });
  await expect(link).toBeVisible();
  const href=await link.getAttribute("href");
  expect(href).toContain(encodeURIComponent("https://t.me/scheduler_bot?start=group_token"));
  expect(href).not.toContain("personal");
});

test("overview recognizes explicit buildings and lesson overlaps", () => {
  const lesson = { ID:"a",Subject:"Math",Teacher:"Teacher",Room:"корпус 1, 214",TimeStart:"09:00",TimeEnd:"10:30",Type:"lecture",Subgroup:0 };
  const next = { ...lesson,ID:"b",Room:"корпус 2, 308",TimeStart:"10:20",TimeEnd:"11:50" };
  const agenda = { target:{timezone:"Europe/Moscow"},days:[{date:"2026-10-05",lessons:[{lesson,original:lesson,cancelled:false},{lesson:next,original:next,cancelled:false}]}] } as Agenda;
  const info=dayOverview(agenda,new Date("2026-10-05T06:30:00Z"));
  expect(info.gap).toBe(-10); expect(info.relocation).toBe(true); expect(info.remaining).toHaveLength(2);
});

test("closing a changed note preserves the draft when cancellation is rejected", async ({ page }) => {
  await setup(page); await page.goto("/app");
  await page.getByRole("button", { name:"Заметка",exact:true }).click();
  await page.getByLabel("Задание, заметка или ссылка").fill("Не потерять черновик");
  page.once("dialog",dialog=>dialog.dismiss());
  await page.getByRole("button", { name:"Закрыть заметку" }).click();
  await expect(page.getByLabel("Задание, заметка или ссылка")).toHaveValue("Не потерять черновик");
  page.once("dialog",dialog=>dialog.accept());
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
});
