import { expect, test, type Page } from "@playwright/test";
import { expectReadableTheme } from "./theme-contrast";
import type { Broadcast } from "../src/api/broadcasts";

async function setup(page: Page, role = "owner") {
  const broadcast: Broadcast = {
    id: "b1",
    author_id: "42",
    document: { type: "doc", content: [{ type: "paragraph" }] },
    body: "",
    audience_mode: "all",
    status: "draft",
    version: 1,
    created_at: "2026-10-06T07:00:00Z",
    updated_at: "2026-10-06T07:00:00Z",
    sent_at: null,
    completed_at: null,
    attachments: [],
    recipients: [],
    counts: {},
  };
  const sends: Array<{ version: number; key: string }> = [];
  await page.route("**/api/**", async (route) => {
    const request = route.request(),
      path = new URL(request.url()).pathname,
      method = request.method();
    if (!path.startsWith("/api/")) return route.continue();
    let body: unknown = {};
    if (path === "/api/auth/config") body = { access_key_enabled: true };
    else if (path === "/api/auth/me")
      body = {
        user: {
          id: "42",
          name: "Автор",
          role,
          auth_method: "telegram",
          csrf_token: "broadcast-test",
        },
      };
    else if (path === "/api/dashboard")
      body = {
        stats: {},
        operations: {},
        sources: [],
        recent_logs: [],
        trend: [],
        universities: [],
      };
    else if (path === "/api/client-errors")
      return route.fulfill({ status: 204 });
    else if (path === "/api/broadcasts")
      body = method === "POST" ? { id: broadcast.id } : [broadcast];
    else if (path === "/api/broadcasts/audience")
      body = {
        total: 5,
        eligible: 3,
        recipients: [
          {
            user_id: "100",
            username: "alice",
            status: "ready",
            reason: "",
            parts: 0,
          },
          {
            user_id: "101",
            username: "bob",
            status: "no_consent",
            reason: "",
            parts: 0,
          },
          {
            user_id: "",
            username: "missing",
            status: "unknown",
            reason: "",
            parts: 0,
          },
          {
            user_id: "102",
            username: "same",
            status: "ready",
            reason: "ambiguous",
            parts: 0,
          },
          {
            user_id: "103",
            username: "same",
            status: "ready",
            reason: "ambiguous",
            parts: 0,
          },
        ],
      };
    else if (path === "/api/broadcasts/b1" && method === "PUT") {
      expect(request.headers()["x-csrf-token"]).toBe("broadcast-test");
      const input = request.postDataJSON();
      expect(input.version).toBe(broadcast.version);
      Object.assign(broadcast, {
        document: input.document,
        audience_mode: input.audience_mode,
        body: "Обновление сервиса 🎉",
        version: broadcast.version + 1,
      });
      broadcast.attachments.sort(
        (a, b) =>
          input.attachment_ids.indexOf(a.id) -
          input.attachment_ids.indexOf(b.id),
      );
      broadcast.recipients = input.user_ids.map((id: string) => ({
        user_id: id,
        username: id === "100" ? "alice" : "same",
        status: "ready",
        reason: "",
        parts: 0,
      }));
      body = broadcast;
    } else if (path === "/api/broadcasts/b1/attachments" && method === "POST") {
      expect(request.headers()["content-type"]).toContain(
        "multipart/form-data; boundary=",
      );
      expect(request.headers()["x-csrf-token"]).toBe("broadcast-test");
      broadcast.attachments.push({
        id: `f${broadcast.attachments.length}`,
        filename: `Документ-${broadcast.attachments.length + 1}.txt`,
        size: 12,
        media_type: "document",
        content_type: "text/plain",
        position: broadcast.attachments.length,
      });
      broadcast.version++;
      body = broadcast;
    } else if (path === "/api/broadcasts/b1/preview")
      body = {
        broadcast,
        body: broadcast.body,
        eligible:
          broadcast.audience_mode === "all" ? 12 : broadcast.recipients.length,
      };
    else if (path === "/api/broadcasts/b1/send") {
      sends.push(request.postDataJSON());
      broadcast.status = "sending";
      broadcast.sent_at = "2026-10-06T08:00:00Z";
      broadcast.counts = { pending: 12 };
      body = broadcast;
    } else if (path === "/api/broadcasts/b1/stop") {
      broadcast.status = "cancelled";
      broadcast.counts = { skipped: 12 };
      body = broadcast;
    } else if (path === "/api/broadcasts/b1") body = broadcast;
    return route.fulfill({
      contentType: "application/json",
      body: JSON.stringify(body),
    });
  });
  return { broadcast, sends };
}

for (const width of [320, 390, 768, 1440])
  test(`compose and send broadcast at ${width}px`, async ({ page }, info) => {
    await page.setViewportSize({ width, height: 1000 });
    const { sends } = await setup(page);
    await page.goto("/#/broadcasts");
    await page.getByRole("button", { name: "Новая рассылка" }).click();
    await page
      .getByRole("textbox", { name: "Текст сообщения" })
      .fill("Обновление сервиса 🎉");
    await page.locator('input[type="file"]').setInputFiles([
      {
        name: "first.txt",
        mimeType: "text/plain",
        buffer: Buffer.from("Первый"),
      },
      {
        name: "second.txt",
        mimeType: "text/plain",
        buffer: Buffer.from("Второй"),
      },
    ]);
    await expect(
      page.getByRole("button", { name: "Проверить и отправить" }),
    ).toBeEnabled();
    await expect(page.locator(".attachment-list li")).toHaveCount(2);
    await page.getByRole("button", { name: "Выше: Документ-2.txt" }).click();
    await page.getByRole("button", { name: "Проверить и отправить" }).click();
    await expect(
      page.getByRole("button", { name: "Отправить 12 пользователям" }),
    ).toBeVisible();
    await expect(page.locator(".telegram-attachment").first()).toContainText(
      "Документ-2.txt",
    );
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth),
    ).toBeLessThanOrEqual(width);
    await expectReadableTheme(page);
    await page.screenshot({
      path: info.outputPath("broadcast.png"),
      fullPage: true,
      animations: "disabled",
    });
    await page
      .getByRole("button", { name: "Отправить 12 пользователям" })
      .click();
    await expect(
      page.getByRole("heading", { name: "Результат рассылки" }),
    ).toBeVisible();
    expect(sends).toHaveLength(1);
    expect(sends[0].key).toMatch(/^[0-9a-f-]{36}$/);
    await page.getByRole("button", { name: "Остановить отправку" }).click();
    await expect(
      page.getByText("Остановлена", { exact: false }).first(),
    ).toBeVisible();
  });

test("selected audience excludes refusals and requires a choice for duplicate names", async ({
  page,
}) => {
  await setup(page);
  await page.goto("/#/broadcasts");
  await page.getByRole("button", { name: "Новая рассылка" }).click();
  await page
    .getByRole("textbox", { name: "Текст сообщения" })
    .fill("Обновление сервиса 🎉");
  await page.getByRole("radio", { name: "По никам" }).check();
  await page
    .getByRole("textbox", { name: "Ники получателей" })
    .fill("@alice bob missing same");
  await page.getByRole("button", { name: "Проверить ники" }).click();
  await expect(page.getByText("Выбрано: 1")).toBeVisible();
  await expect(page.getByRole("checkbox", { name: /bob/ })).toBeDisabled();
  await expect(page.getByRole("checkbox", { name: /missing/ })).toBeDisabled();
  await expect(
    page.getByRole("checkbox", { name: /ID 102/ }),
  ).not.toBeChecked();
  await page.getByRole("checkbox", { name: /ID 102/ }).check();
  await page.getByRole("button", { name: "Проверить и отправить" }).click();
  await expect(
    page.getByRole("button", { name: "Отправить 2 пользователям" }),
  ).toBeVisible();
});

for (const role of ["read_only", "support", "editor", "reviewer", "operator"])
  test(`${role} cannot open broadcasts`, async ({ page }) => {
    await setup(page, role);
    await page.goto("/#/broadcasts");
    await expect(
      page.getByRole("heading", { name: "Сводка на сегодня" }),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Рассылки", exact: true }),
    ).toHaveCount(0);
  });
