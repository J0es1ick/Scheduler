import { expect, test, type Page } from "@playwright/test";
import { expectReadableTheme } from "./theme-contrast";

async function mockShell(page: Page, authenticated = true) {
  await page.route(
    (url) => url.pathname.startsWith("/api/"),
    (route) => {
      const path = new URL(route.request().url()).pathname;
      const body =
        path === "/api/auth/config"
          ? { access_key_enabled: true }
          : path === "/api/auth/me"
            ? {
                user: {
                  id: "42",
                  name: "Theme preview",
                  role: "owner",
                  auth_method: "telegram",
                  csrf_token: "theme-fixture",
                },
              }
            : path === "/api/dashboard"
              ? {
                  stats: {
                    universities: 2,
                    groups: 522,
                    lessons: 1200,
                    users: 10,
                    subscriptions: 14,
                    success_rate: 99,
                  },
                  sources: [],
                  recent_logs: [],
                  trend: [],
                  universities: [],
                  operations: {
                    status: "healthy",
                    database: true,
                    sources_total: 2,
                    sources_healthy: 2,
                    sources_running: 0,
                    sources_stale: 0,
                    sources_error: 0,
                    sources_quarantined: 0,
                    sources_disabled: 0,
                    pending_notifications: 0,
                    failed_notifications: 0,
                    pending_outbox: 0,
                    failed_outbox: 0,
                    oldest_pending_seconds: 0,
                  },
                }
              : path === "/api/service-logs"
                ? {
                    entries: [],
                    components: [],
                    modules: [],
                    warnings: [],
                    checked_at: new Date().toISOString(),
                  }
                : {
                    items: [],
                    pagination: { page: 1, page_size: 20, total: 0 },
                  };
      return route.fulfill({
        status: path === "/api/auth/me" && !authenticated ? 401 : 200,
        contentType: "application/json",
        body: JSON.stringify(body),
      });
    },
  );
}

test("theme slider follows the device until overridden and remembers manual choice", async ({
  page,
}) => {
  await page.emulateMedia({ colorScheme: "light" });
  await mockShell(page);
  await page.goto("/#/overview");
  const toggle = page.getByRole("switch", { name: "Тёмная тема" });
  await expect(toggle).not.toBeChecked();
  await page.emulateMedia({ colorScheme: "dark" });
  await expect(toggle).toBeChecked();
  await toggle.focus();
  await page.keyboard.press("Space");
  await expect(toggle).not.toBeChecked();
  await page.reload();
  await expect(toggle).not.toBeChecked();
  await page.emulateMedia({ colorScheme: "light" });
  await page.emulateMedia({ colorScheme: "dark" });
  await expect(toggle).not.toBeChecked();
  await page
    .getByRole("button", { name: "Использовать тему устройства" })
    .click();
  await expect(toggle).toBeChecked();
  await page.emulateMedia({ colorScheme: "light" });
  await expect(toggle).not.toBeChecked();
});

test("theme can be changed before login and with unavailable storage", async ({
  page,
}) => {
  await page.emulateMedia({ colorScheme: "light" });
  await page.addInitScript(() => {
    Object.defineProperty(window, "localStorage", {
      get() {
        throw new DOMException("Storage blocked", "SecurityError");
      },
    });
  });
  await mockShell(page, false);
  await page.goto("/");
  await expect(
    page.getByRole("heading", { name: "Вход в админку" }),
  ).toBeVisible();
  await page.getByRole("switch", { name: "Тёмная тема" }).click();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  await expect(page.getByLabel("Ключ доступа")).toBeVisible();
});

test("Telegram theme updates respect the manual override and native chrome", async ({
  page,
}) => {
  await page.emulateMedia({ colorScheme: "light" });
  await page.route("https://telegram.org/js/telegram-web-app.js", (route) =>
    route.fulfill({ contentType: "application/javascript", body: "" }),
  );
  await page.addInitScript(() => {
    const listeners = new Set<() => void>();
    const app = {
      initData: "theme-test",
      colorScheme: "dark" as "light" | "dark",
      ready() {},
      expand() {},
      onEvent(name: string, callback: () => void) {
        if (name === "themeChanged") listeners.add(callback);
      },
      offEvent(name: string, callback: () => void) {
        if (name === "themeChanged") listeners.delete(callback);
      },
      setHeaderColor(color: string) {
        document.documentElement.dataset.testHeaderColor = color;
      },
      setBackgroundColor(color: string) {
        document.documentElement.dataset.testBackgroundColor = color;
      },
    };
    window.Telegram = { WebApp: app };
    window.addEventListener("test:telegram-theme", () => {
      app.colorScheme = app.colorScheme === "dark" ? "light" : "dark";
      listeners.forEach((listener) => listener());
    });
  });
  await mockShell(page);
  await page.goto("/");
  await expect(page.getByRole("switch", { name: "Тёмная тема" })).toBeChecked();
  await page.evaluate(() =>
    window.dispatchEvent(new Event("test:telegram-theme")),
  );
  await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
  await page.evaluate(() =>
    window.dispatchEvent(new Event("test:telegram-theme")),
  );
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  await page.getByRole("switch", { name: "Тёмная тема" }).click();
  await page.evaluate(() => {
    window.dispatchEvent(new Event("test:telegram-theme"));
    window.dispatchEvent(new Event("test:telegram-theme"));
  });
  await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
  await page
    .getByRole("button", { name: "Использовать тему устройства" })
    .click();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  expect(
    await page.evaluate(() => {
      const root = document.documentElement,
        styles = getComputedStyle(root);
      return (
        root.dataset.testHeaderColor ===
          styles.getPropertyValue("--paper-deep").trim() &&
        root.dataset.testBackgroundColor ===
          styles.getPropertyValue("--paper").trim()
      );
    }),
  ).toBe(true);
});

for (const width of [320, 390, 768, 1440])
  test(`theme controls and dark sections fit ${width}px`, async ({
    page,
  }, info) => {
    await mockShell(page);
    await page.emulateMedia({ colorScheme: "dark", reducedMotion: "reduce" });
    await page.setViewportSize({ width, height: 900 });
    for (const section of [
      "overview",
      "editor",
      "sources",
      "connectors",
      "logs",
      "data",
      "support",
      "users",
      "audit",
    ]) {
      await page.goto(`/#/${section}`);
      await expect(
        page.getByRole("switch", { name: "Тёмная тема" }),
      ).toBeVisible();
      await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth + 1,
        ),
        section,
      ).toBe(true);
      await expectReadableTheme(page);
      if (
        process.env.SCHEDULER_THEME_CAPTURE === "1" &&
        (width === 390 || width === 1440) &&
        ["overview", "connectors", "support", "audit"].includes(section)
      ) {
        await page.screenshot({
          path: info.outputPath(`${section}.png`),
          fullPage: false,
          animations: "disabled",
        });
      }
    }
  });
