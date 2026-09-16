import assert from "node:assert/strict";
import process from "node:process";
import console from "node:console";
import { URL } from "node:url";
import { chromium } from "playwright";

const backend = process.env.SCHEDULER_FRAME_TEST_URL;
assert.match(backend ?? "", /^https:\/\/127\.0\.0\.1:\d+$/);
const browser = await chromium.launch({
  args: ["--test-third-party-cookie-phaseout"],
});
try {
  const context = await browser.newContext({ ignoreHTTPSErrors: true });
  await context.route("**/*", async (route) => {
    const url = new URL(route.request().url());
    if (["web.telegram.org", "untrusted.example"].includes(url.hostname)) {
      await route.fulfill({
        contentType: "text/html",
        body: '<iframe src="https://scheduler.example/"></iframe>',
      });
    } else if (url.hostname === "scheduler.example") {
      const response = await route.fetch({ url: backend + url.pathname });
      await route.fulfill({ response });
    } else {
      await route.abort();
    }
  });
  const page = await context.newPage();
  await page.goto("https://web.telegram.org/");
  const frame = page.frameLocator("iframe");
  await frame.getByText("Scheduler frame", { exact: true }).waitFor();
  const result = await frame.locator("body").evaluate(async () => {
    const login = await (
      await globalThis.fetch("/login", { method: "POST" })
    ).json();
    const before = (await globalThis.fetch("/me")).status;
    const withoutCSRF = (await globalThis.fetch("/logout", { method: "POST" }))
      .status;
    const logout = (
      await globalThis.fetch("/logout", {
        method: "POST",
        headers: { "X-CSRF-Token": login.csrf_token },
      })
    ).status;
    return {
      before,
      withoutCSRF,
      logout,
      after: (await globalThis.fetch("/me")).status,
    };
  });
  assert.deepEqual(result, {
    before: 204,
    withoutCSRF: 403,
    logout: 204,
    after: 401,
  });
  assert.equal((await context.cookies("https://scheduler.example/")).length, 0);
  const violations = [];
  page.on("console", (message) => {
    if (message.type() === "error") violations.push(message.text());
  });
  await page.goto("https://untrusted.example/");
  assert.equal(
    await page.frameLocator("iframe").getByText("Scheduler frame").count(),
    0,
  );
  assert.ok(violations.some((message) => message.includes("frame-ancestors")));
  console.log(
    "Telegram frame: cookie session, CSRF, logout and untrusted ancestor rejection passed",
  );
} finally {
  await browser.close();
}
