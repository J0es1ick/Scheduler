import { expect, test, type Page } from "@playwright/test";
import { expectReadableTheme } from "../tests/theme-contrast";

async function mockSite(page: Page) {
  await page.route("**/api/public-info", (route) => route.fulfill({ json: {
    universities: 3, users: 1240, groups: 522, lessons: 18400, subscriptions: 1600,
    university_names: ["ИГХТУ", "ИГЭУ", "ИВГПУ"],
    sources: ["ИГХТУ", "ИГЭУ", "ИВГПУ"].map((name) => ({
      university_name: name, state: "current", secure: true,
      last_success_at: "2026-10-18T07:00:00Z", schedule_url: "https://example.test/schedule",
    })),
    project_url: "https://example.test/project", bot_url: "https://t.me/schedule_free_bot",
    updated_at: "2026-10-18T07:00:00Z",
  } }));
}

async function mockAdmin(page: Page) {
  await page.route("**/api/**", (route) => {
    const path = new URL(route.request().url()).pathname;
    if (!path.startsWith("/api/")) return route.continue();
    if (path === "/api/auth/config") return route.fulfill({ json: { access_key_enabled: false } });
    if (path === "/api/auth/me") return route.fulfill({ json: { user: {
      id: "42", name: "@scheduler", role: "owner", auth_method: "telegram", csrf_token: "test",
    } } });
    if (path === "/api/dashboard") return route.fulfill({ json: {
      stats: { universities: 3, groups: 522, lessons: 18400, users: 1240, subscriptions: 1600, success_rate: 99 },
      sources: [], recent_logs: [], trend: [], universities: [],
      operations: { status: "healthy", database: true, sources_total: 3, sources_healthy: 3 },
    } });
    if (path === "/api/broadcasts") return route.fulfill({ json: [] });
    return route.fulfill({ json: { items: [] } });
  });
}

for (const width of [320, 390, 768, 1440]) {
  test(`site motion keeps navigation and role preview usable at ${width}px`, async ({ page }, info) => {
    await page.setViewportSize({ width, height: 1000 });
    await mockSite(page);
    await page.goto("http://127.0.0.1:15174");
    const example = page.locator(".public-hero-product");
    await expect(example).toBeVisible();
    await expect.poll(() => page.evaluate(() => document.getAnimations().length)).toBe(0);
    const height = (await example.boundingBox())!.height;
    await page.getByRole("button", { name: "Преподаватель", exact: true }).click();
    await expect(page.getByText("Павлова Елена Андреевна")).toBeVisible();
    await expect(page.getByRole("button", { name: "Преподаватель", exact: true })).toHaveAttribute("aria-pressed", "true");
    await expect.poll(async () => Math.abs((await example.boundingBox())!.height - height)).toBeLessThan(2);
    await page.getByRole("button", { name: "Студент", exact: true }).click();
    await expect(page.getByText("Группа ИВТ-21")).toBeVisible();
    if (width <= 760) {
      await expect(page.getByRole("navigation")).toBeHidden();
      await page.getByRole("button", { name: "Открыть меню" }).click();
      await expect(page.getByRole("navigation")).toBeVisible();
      await page.keyboard.press("Escape");
      await expect(page.getByRole("button", { name: "Открыть меню" })).toBeFocused();
      await expect(page.getByRole("navigation")).toBeHidden();
      await page.getByRole("button", { name: "Открыть меню" }).click();
    }
    await page.getByRole("navigation").getByRole("link", { name: "Вузы", exact: true }).click();
    await expect(page.locator("#universities [data-reveal]")).toHaveAttribute("data-revealed", "true");
    await expect(page.getByText("ИГХТУ", { exact: true }).first()).toBeVisible();
    await expect.poll(() => page.evaluate(() => document.getAnimations().length)).toBe(0);
    await page.locator("#technologies summary").click();
    await expect(page.locator(".public-tech-grid")).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(width);
    await page.screenshot({ path: info.outputPath("site.png"), fullPage: true, animations: "disabled" });
  });

  test(`admin navigation remains stable at ${width}px`, async ({ page }, info) => {
    await page.setViewportSize({ width, height: 1000 });
    await mockAdmin(page);
    await page.goto("/#/overview");
    await expect(page.locator(".metric-strip")).toBeVisible();
    await expect.poll(() => page.evaluate(() => document.getAnimations().length)).toBe(0);
    await page.screenshot({ path: info.outputPath("admin.png"), fullPage: true, animations: "disabled" });
    if (width > 1100) {
      await page.locator(".desktop-nav").getByRole("button", { name: "Рассылки", exact: true }).click();
      const active = page.locator('.desktop-nav [aria-current="page"]');
      await expect(active).toHaveText("Рассылки");
      await expect.poll(async () => {
        const selected = await active.boundingBox();
        const indicator = await page.locator(".nav-indicator").boundingBox();
        return Math.abs(selected!.y - indicator!.y);
      }).toBeLessThan(1);
    } else {
      await page.getByLabel("Раздел", { exact: true }).selectOption("broadcasts");
    }
    await expect(page.getByRole("heading", { name: "Сообщения пользователям" })).toBeVisible();
    await page.getByRole("switch", { name: "Тёмная тема" }).click();
    await expectReadableTheme(page);
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(width);
  });
}

test("reduced motion applies immediately and preserves all content", async ({ page }) => {
  await mockSite(page);
  await page.goto("http://127.0.0.1:15174");
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.locator("#connectors").scrollIntoViewIfNeeded();
  await expect.poll(() => page.evaluate(() => document.getAnimations().length)).toBe(0);
  await expect(page.locator("[data-reveal]:not([data-revealed='true'])")).toHaveCount(0);
  await page.getByRole("button", { name: "Преподаватель", exact: true }).click();
  await expect(page.getByText("Павлова Елена Андреевна")).toBeVisible();
  await expect.poll(() => page.evaluate(() => document.getAnimations().length)).toBe(0);
  await mockAdmin(page);
  await page.goto("http://127.0.0.1:15173/#/overview");
  await expect(page.locator(".metric-strip")).toBeVisible();
  await expect.poll(() => page.evaluate(() => document.getAnimations().length)).toBe(0);
  await page.getByRole("switch", { name: "Тёмная тема" }).click();
  await expect.poll(() => page.evaluate(() => document.getAnimations().length)).toBe(0);
});

test("site content remains accessible without intersection observers", async ({ page }) => {
  await page.addInitScript(() => { delete (window as unknown as { IntersectionObserver?: unknown }).IntersectionObserver; });
  await mockSite(page);
  await page.goto("http://127.0.0.1:15174");
  await expect(page.getByRole("heading", { name: "Ваш вуз. Ваше расписание." })).toBeVisible();
  await expect(page.locator("[data-reveal]:not([data-revealed='true'])")).toHaveCount(0);
});
