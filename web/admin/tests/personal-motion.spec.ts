import { expect, test, type Page } from "@playwright/test";
import { setup } from "./personal-fixture";
import { expectReadableTheme } from "./theme-contrast";

async function settled(page: Page) {
  await expect.poll(() => page.evaluate(() => document.getAnimations().length)).toBe(0);
}

for (const width of [320, 390, 768, 1440]) {
  test(`personal week and editor remain usable at ${width}px`, async ({ page }, info) => {
    await page.setViewportSize({ width, height: 844 });
    await setup(page, false, { week: true });
    await page.goto("/app");
    await expect(page.getByRole("navigation", { name: "Дни недели" })).toBeVisible();
    await settled(page);
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(width);
    await page.screenshot({ path: info.outputPath("schedule.png"), fullPage: true, animations: "disabled" });
    await page.getByRole("button", { name: "Перейти к 7 октября" }).click();
    const heading = page.locator("#personal-day-2026-10-07");
    await expect.poll(async () => {
      const box = (await heading.boundingBox())!;
      const controls = (await page.locator(".personal-calendar-controls").boundingBox())!;
      return controls.y >= 0 && box.y >= controls.y + controls.height && box.y < 500;
    }).toBe(true);
    const edit = heading.locator("..").getByRole("button", { name: /Изменить Теоретическая/ });
    const editBox = (await edit.boundingBox())!;
    expect(editBox.height).toBeGreaterThanOrEqual(44);
    await edit.click();
    const dialog = page.getByRole("dialog");
    await expect(dialog).toBeVisible();
    await settled(page);
    await expect(page.locator(".personal-dialog header")).toBeFocused();
    const box = (await dialog.boundingBox())!;
    expect(box.x).toBeGreaterThanOrEqual(0);
    expect(box.x + box.width).toBeLessThanOrEqual(width);
    expect(box.y + box.height).toBeLessThanOrEqual(845);
    await expect(page.getByRole("button", { name: "Сохранить", exact: true })).toBeInViewport();
    await page.getByLabel("Применить", { exact: true }).selectOption("selected");
    await expect(page.getByLabel("19 октября", { exact: true })).toBeVisible();
    await expectReadableTheme(page);
    await page.screenshot({ path: info.outputPath("editor.png"), animations: "disabled" });
    page.once("dialog", (confirmation) => confirmation.accept());
    await page.getByRole("button", { name: "Закрыть", exact: true }).click();
    await expect(dialog).toHaveCount(0);
    await expect(edit).toBeFocused();
    expect(await page.evaluate(() => document.body.style.overflow)).toBe("");
  });
}

test("refresh keeps lessons and scroll position through slow and failed responses", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.setViewportSize({ width: 390, height: 844 });
  const state = await setup(page, false, { week: true });
  await page.goto("/app");
  await page.getByRole("button", { name: "Перейти к 7 октября" }).click();
  const heading = page.locator("#personal-day-2026-10-07");
  await expect(heading).toBeInViewport();
  await settled(page);
  const position = await page.evaluate(() => scrollY);
  let release = () => { };
  state.scheduleGate = new Promise<void>((resolve) => { release = resolve; });
  await page.getByRole("button", { name: "Обновить расписание" }).click();
  await expect(page.locator(".personal-days")).toHaveAttribute("aria-busy", "true");
  await expect(heading).toBeVisible();
  await expect(page.getByRole("button", { name: /Изменить Теоретическая/ }).first()).toBeDisabled();
  expect(Math.abs(await page.evaluate(() => scrollY) - position)).toBeLessThan(2);
  state.scheduleError = true;
  release();
  await expect(page.getByRole("alert")).toContainText("Временно недоступно");
  await expect(page.locator(".personal-days")).toHaveAttribute("aria-busy", "false");
  await expect(heading).toBeVisible();
  state.scheduleError = false;
  state.scheduleGate = undefined;
  await page.getByRole("button", { name: "Обновить расписание" }).click();
  await expect(page.getByRole("alert")).toHaveCount(0);
  await expect(page.locator(".personal-days")).toHaveAttribute("aria-busy", "false");
});

test("editor fits the keyboard viewport and Telegram safe areas", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.addInitScript(() => {
    Object.assign(window, { Telegram: { WebApp: { safeAreaInset: { top: 16, bottom: 24 }, contentSafeAreaInset: { top: 24, bottom: 12 } } } });
  });
  await setup(page);
  await page.goto("/app");
  await page.getByRole("button", { name: /Изменить Теоретическая/ }).click();
  await page.getByLabel("Аудитория", { exact: true }).focus();
  await page.evaluate(() => {
    Object.defineProperty(window.visualViewport, "height", { configurable: true, value: 380 });
    Object.defineProperty(window.visualViewport, "offsetTop", { configurable: true, value: 20 });
    window.visualViewport!.dispatchEvent(new Event("resize"));
  });
  await settled(page);
  const box = (await page.getByRole("dialog").boundingBox())!;
  const save = (await page.getByRole("button", { name: "Сохранить", exact: true }).boundingBox())!;
  expect(box.y).toBeGreaterThanOrEqual(44);
  expect(box.y + box.height).toBeLessThanOrEqual(401);
  expect(save.y + save.height).toBeLessThanOrEqual(400);
  expect(save.y).toBeGreaterThan(box.y + 60);
  await page.getByLabel("Аудитория", { exact: true }).fill("А-101");
  await page.getByRole("button", { name: "Сохранить", exact: true }).click();
  await expect(page.getByText("Аудитория А-101", { exact: true })).toBeVisible();
});

test("teacher schedule retains group names and honors reduced motion", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.setViewportSize({ width: 390, height: 844 });
  await setup(page, false, { teacher: true });
  await page.goto("/app");
  await expect(page.locator(".personal-lesson-copy small")).toContainText("4/147");
  await expect(page.getByRole("combobox", { name: "Расписание", exact: true })).toHaveValue("group");
  await settled(page);
  await page.getByRole("button", { name: /Изменить Теоретическая/ }).click();
  await expect(page.getByRole("dialog")).toBeVisible();
  expect(await page.evaluate(() => document.getAnimations().length)).toBe(0);
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
});

test("Telegram back respects unsaved edits and returns to the chooser", async ({ page }) => {
  await page.addInitScript(() => {
    const back = { click: () => { }, shown: false };
    Object.assign(window, {
      personalTestBack: back, Telegram: {
        WebApp: {
          BackButton: {
            show: () => { back.shown = true; }, hide: () => { back.shown = false; },
            onClick: (callback: () => void) => { back.click = callback; },
            offClick: (callback: () => void) => { if (back.click === callback) back.click = () => { }; },
          }
        }
      }
    });
  });
  await setup(page, true);
  const back = () => page.evaluate(() => (window as unknown as { personalTestBack: { click: () => void } }).personalTestBack.click());
  await page.goto("/app");
  await page.getByRole("button", { name: /Личное расписание/ }).click();
  await page.getByRole("button", { name: /Изменить Теоретическая/ }).click();
  await page.getByLabel("Аудитория", { exact: true }).fill("А-101");
  page.once("dialog", (confirmation) => confirmation.dismiss());
  await back();
  await expect(page.getByRole("dialog")).toBeVisible();
  page.once("dialog", (confirmation) => confirmation.accept());
  await back();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await back();
  await expect(page.getByRole("heading", { name: "Куда перейдём?" })).toBeVisible();
});


test("saving preserves the visible day and returns keyboard focus", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.setViewportSize({ width: 390, height: 844 });
  await setup(page, false, { week: true });
  await page.goto("/app");
  await page.getByRole("button", { name: "Перейти к 7 октября" }).click();
  const heading = page.locator("#personal-day-2026-10-07");
  await expect(heading).toBeInViewport();
  const position = await page.evaluate(() => scrollY);
  await heading.locator("..").getByRole("button", { name: /Изменить Теоретическая/ }).click();
  await page.getByLabel("Аудитория", { exact: true }).fill("А-101");
  await page.getByRole("button", { name: "Сохранить", exact: true }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(heading.locator("..").getByText("Аудитория А-101", { exact: true })).toBeVisible();
  await expect(heading).toBeFocused();
  expect(Math.abs(await page.evaluate(() => scrollY) - position)).toBeLessThan(2);
});
