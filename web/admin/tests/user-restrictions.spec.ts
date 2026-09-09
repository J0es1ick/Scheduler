import { expect, test, type Page } from "@playwright/test";

async function setup(page: Page, role = "owner") {
  const user = {
    id: "100",
    username: "Student",
    is_admin: false,
    admin_role: "none",
    subscriptions: 2,
    default_group_name: "4/147",
    notifications_enabled: true,
    bot_blocked: false,
    support_blocked: false,
    created_at: "2026-09-01T12:00:00Z",
  };
  await page.route("**/api/**", (route) => {
    const path = new URL(route.request().url()).pathname;
    if (!path.startsWith("/api/")) return route.continue();
    const body =
      path === "/api/auth/me"
        ? {
            user: {
              id: "42",
              name: "Owner",
              role,
              auth_method: "telegram",
              csrf_token: "synthetic-csrf",
            },
          }
        : path === "/api/auth/config"
          ? { access_key_enabled: false }
          : path === "/api/users"
            ? { items: [user] }
            : { items: [] };
    return route.fulfill({
      contentType: "application/json",
      body: JSON.stringify(body),
    });
  });
  return user;
}

for (const width of [320, 390, 768, 1440])
  test(`independent user restrictions at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 });
    const user = await setup(page);
    const updates: unknown[] = [];
    await page.route("**/api/users/100/restrictions", async (route) => {
      expect(route.request().method()).toBe("PATCH");
      expect(route.request().headers()["x-csrf-token"]).toBe("synthetic-csrf");
      const patch = route.request().postDataJSON();
      updates.push(patch);
      Object.assign(user, patch);
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({
          bot_blocked: user.bot_blocked,
          support_blocked: user.support_blocked,
        }),
      });
    });
    await page.goto("/#/users");
    const card = page.getByRole("article", { name: "Пользователь Student" });
    await card
      .getByRole("button", { name: "Запретить обращения", exact: true })
      .click();
    await expect(
      card.getByText("Обращения: запрещены", { exact: true }),
    ).toBeVisible();
    await expect(
      card.getByText("Бот: доступен", { exact: true }),
    ).toBeVisible();
    await card
      .getByRole("button", { name: "Заблокировать бота", exact: true })
      .click();
    await expect(
      card.getByText("Бот: заблокирован", { exact: true }),
    ).toBeVisible();
    await card
      .getByRole("button", { name: "Разблокировать бота", exact: true })
      .click();
    await expect(
      card.getByText("Бот: доступен", { exact: true }),
    ).toBeVisible();
    await expect(
      card.getByText("Обращения: запрещены", { exact: true }),
    ).toBeVisible();
    await card
      .getByRole("button", { name: "Разрешить обращения", exact: true })
      .click();
    await expect(
      card.getByText("Обращения: разрешены", { exact: true }),
    ).toBeVisible();
    expect(updates).toEqual([
      { support_blocked: true },
      { bot_blocked: true },
      { bot_blocked: false },
      { support_blocked: false },
    ]);
    for (const selector of [
      ".user-card",
      ".user-actions",
      ".user-restrictions",
    ]) {
      const bounds = await page.locator(selector).boundingBox();
      expect(bounds!.x).toBeGreaterThanOrEqual(0);
      expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(width + 1);
    }
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth),
    ).toBeLessThanOrEqual(width);
  });

test("restriction stays unchanged until server confirms and remains unchanged on failure", async ({
  page,
}) => {
  await setup(page);
  let release: (() => void) | undefined;
  await page.route("**/api/users/100/restrictions", async (route) => {
    await new Promise<void>((resolve) => {
      release = resolve;
    });
    await route.fulfill({
      status: 503,
      headers: { "X-Request-ID": "restriction-failure" },
      contentType: "application/json",
      body: JSON.stringify({ error: "Не удалось изменить ограничения" }),
    });
  });
  await page.goto("/#/users");
  const card = page.getByRole("article", { name: "Пользователь Student" });
  const block = card.getByRole("button", {
    name: "Заблокировать бота",
    exact: true,
  });
  await block.click();
  await expect(block).toBeDisabled();
  await expect(
    card.getByRole("button", { name: "Запретить обращения", exact: true }),
  ).toBeDisabled();
  await expect(card.getByText("Бот: доступен", { exact: true })).toBeVisible();
  await expect.poll(() => Boolean(release)).toBe(true);
  release?.();
  await expect(page.getByText(/restriction-failure/)).toBeVisible();
  await expect(block).toBeEnabled();
  await expect(card.getByText("Бот: доступен", { exact: true })).toBeVisible();
  await expect(
    card.getByRole("button", { name: "Разблокировать бота", exact: true }),
  ).toHaveCount(0);
});

for (const role of ["read_only", "support", "editor", "reviewer", "operator"])
  test(`${role} cannot open user moderation`, async ({ page }) => {
    await setup(page, role);
    await page.goto("/#/users");
    await expect(
      page.getByRole("switch", { name: "Тёмная тема" }),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Заблокировать бота", exact: true }),
    ).toHaveCount(0);
    await expect(
      page.getByRole("button", { name: "Запретить обращения", exact: true }),
    ).toHaveCount(0);
  });
