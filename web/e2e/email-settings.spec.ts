import { apiAs, authFile, expect, MAILPIT, seedWorkspace, test, type Seed } from "./fixtures";

// Workspace SMTP settings (ADR-0038). The dev stack allows Mailpit as a
// workspace server through NEBULA_SMTP_WORKSPACE_ALLOW_PRIVATE/_PLAINTEXT and
// port 1025 in NEBULA_SMTP_WORKSPACE_PORTS (docker-compose.yml, dev only).
test.use({ storageState: authFile("alice") });

let seed: Seed;
test.beforeAll(async () => {
  seed = await seedWorkspace();
});

test("admin points the workspace at Mailpit and the test email arrives", async ({ page }) => {
  const sender = `smtp-${Math.random().toString(36).slice(2, 8)}@example.com`; // unique per run: found by From in Mailpit
  await page.goto(`/w/${seed.ws}/settings/email`);
  await expect(page.getByRole("status")).toContainText("Hazırda server konfiqurasiyası istifadə olunur");

  // A preset fills host, port and encryption.
  await page.getByRole("button", { name: "Microsoft 365" }).click();
  await expect(page.getByLabel("SMTP server")).toHaveValue("smtp.office365.com");
  await expect(page.getByLabel(/^Port/)).toHaveValue("587");
  await expect(page.getByText(/SMTP AUTH aktiv olmalıdır/)).toBeVisible();

  await page.getByLabel(/^Göndərən \(From\)/).fill(`E2E SMTP <${sender}>`);
  await page.getByLabel("SMTP server").fill("mailpit");
  await page.getByLabel(/^Port/).fill("1025");
  await page.getByLabel("Şifrələmə").selectOption("none");
  await page.getByRole("button", { name: "Yadda saxla" }).click();
  await expect(page.getByRole("status")).toContainText("Bu workspace öz SMTP serverindən göndərir.");

  await page.getByRole("button", { name: "Sınaq məktubu göndər" }).click();
  await expect(page.getByText(/ünvanına göndərildi \(workspace serveri\)/)).toBeVisible();

  const mail = await apiAs("alice");
  await expect
    .poll(async () => {
      const r = await mail.get(`${MAILPIT}/search?query=${encodeURIComponent(`from:${sender}`)}`);
      return ((await r.json()) as { messages: unknown[] }).messages.length;
    }, { timeout: 10_000 })
    .toBe(1);
  await mail.dispose();

  // Another address must belong to a member.
  await page.getByLabel(/^Kimə/).fill("stranger@example.org");
  await page.getByRole("button", { name: "Sınaq məktubu göndər" }).click();
  await expect(page.getByRole("alert").filter({ hasText: "member of this workspace" })).toBeVisible();

  // Back to the server configuration.
  page.once("dialog", (d) => void d.accept());
  await page.getByRole("button", { name: "Server konfiqurasiyasına qayıt" }).click();
  await expect(page.getByRole("status")).toContainText("Hazırda server konfiqurasiyası istifadə olunur");
});

test("a saved password is write-only", async ({ page }) => {
  const alice = await apiAs("alice");
  const secret = "e2e-Secret-" + Math.random().toString(36).slice(2, 10);
  const put = await alice.put(`workspaces/${seed.ws}/email-settings`, {
    data: { host: "smtp.gmail.com", port: 587, tls_mode: "starttls", username: "team@example.com", password: secret, from_address: "Team <team@example.com>" },
  });
  expect(put.status()).toBe(200);
  expect(await put.text()).not.toContain(secret);
  await alice.dispose();

  await page.goto(`/w/${seed.ws}/settings/email`);
  await expect(page.getByText("•••• (saxlanılıb)")).toBeVisible();
  await expect(page.locator("body")).not.toContainText(secret);
  await page.getByRole("button", { name: "Dəyiş" }).click();
  await expect(page.getByLabel(/^Parol/)).toHaveValue("");
});

test("members cannot open the email settings", async ({ browser }) => {
  const ctx = await browser.newContext({ storageState: authFile("bob") });
  const page = await ctx.newPage();
  await page.goto(`/w/${seed.ws}/settings/email`);
  await expect(page.getByText(/yalnız admin/i)).toBeVisible();
  await ctx.close();
});
