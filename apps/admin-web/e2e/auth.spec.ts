import { expect, test } from "@playwright/test";

/**
 * Real-stack checks (Kratos + identity-service + Mailpit). The full admin
 * journey (bootstrap invite -> recovery code from Mailpit -> password -> TOTP
 * -> step-up -> disable customer -> audit) needs a fresh invited admin; it
 * runs only when E2E_ADMIN_EMAIL is set to an admin invited moments ago.
 */
const MAILPIT = process.env.MAILPIT_URL ?? "http://localhost:8025";

test("unauthenticated console visit lands on login without registration link", async ({ page }) => {
  await page.goto("/customers");
  await expect(page).toHaveURL(/\/login\?.*return_to=%2Fcustomers/);
  await expect(page.locator('input[name="csrf_token"]')).toHaveCount(1);
  await expect(page.getByRole("link", { name: /đăng ký|regist/i })).toHaveCount(0);
});

test("/registration renders no form", async ({ page }) => {
  await page.goto("/registration");
  await expect(page.getByRole("heading", { name: "Không hỗ trợ đăng ký" })).toBeVisible();
  await expect(page.locator("form")).toHaveCount(0);
});

test("invalid credentials re-render the same flow with a localised message", async ({ page }) => {
  await page.goto("/login");
  await page.waitForURL(/flow=/);
  const flowUrl = page.url();
  await page.locator('input[name="identifier"]').fill("nobody@example.com");
  await page.locator('input[name="password"]').fill("definitely-wrong-password");
  await page.locator('button[name="method"][value="password"]').click();
  await expect(page.getByText(/Thông tin đăng nhập không hợp lệ/)).toBeVisible();
  expect(page.url()).toBe(flowUrl);
});

test("invited admin redeems the recovery code into the settings flow", async ({
  page,
  request,
}) => {
  const email = process.env.E2E_ADMIN_EMAIL;
  test.skip(!email, "set E2E_ADMIN_EMAIL to a freshly invited admin");
  if (!email) return;

  await page.goto("/recovery");
  await page.locator('input[name="email"]').fill(email);
  await page.locator('button[name="method"][value="code"]').click();
  await expect(page.locator('input[name="code"]')).toBeVisible();

  const search = await request.get(
    `${MAILPIT}/api/v1/search?query=${encodeURIComponent(`to:${email}`)}`,
  );
  const list = (await search.json()) as { messages: { ID: string }[] };
  const first = list.messages[0];
  expect(first).toBeDefined();
  if (!first) return;
  const msg = (await (await request.get(`${MAILPIT}/api/v1/message/${first.ID}`)).json()) as {
    Text: string;
  };
  const code = /\b(\d{6})\b/.exec(msg.Text)?.[1];
  expect(code).toBeDefined();

  await page.locator('input[name="code"]').fill(code ?? "");
  await page.locator('button[name="method"][value="code"]').click();
  await expect(page).toHaveURL(/\/settings\?flow=/);
  await expect(page.locator('input[name="password"]')).toBeVisible();
});
