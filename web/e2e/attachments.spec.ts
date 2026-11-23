import { apiAs, authFile, expect, seedWorkspace, test, type Seed } from "./fixtures";

// File attachments and Markdown (ADR-0022), as alice (owner) with bob as member.
test.use({ storageState: authFile("alice") });

// 1x1 transparent PNG.
const PNG = Buffer.from(
  "89504e470d0a1a0a0000000d49484452000000010000000108060000001f15c4890000000d4944415478da63f8cfc0f01f0005000201a5f6cb2b0000000049454e44ae426082",
  "hex",
);

let seed: Seed;
test.beforeAll(async () => {
  seed = await seedWorkspace();
  const alice = await apiAs("alice");
  expect((await alice.post(`workspaces/${seed.ws}/projects/QA/items`, { data: { title: "Fayl testi" } })).status()).toBe(201);
  await alice.dispose();
});

test("upload, preview, download and delete attachments", async ({ page }) => {
  await page.goto(seed.url + "?item=1");
  const panel = page.getByRole("dialog", { name: "QA-1" });
  const files = panel.getByTestId("attachments");
  await expect(files).toContainText("Fayl yoxdur.");

  // An image gets a thumbnail; an HTML page named .png does not (type is sniffed).
  await files.getByLabel("Fayl əlavə et").setInputFiles([
    { name: "ekran.png", mimeType: "image/png", buffer: PNG },
    { name: "saxta.png", mimeType: "image/png", buffer: Buffer.from("<!DOCTYPE html><script>alert(1)</script>") },
  ]);
  await expect(files.getByRole("listitem")).toHaveCount(2);
  const image = files.getByRole("listitem").filter({ hasText: "ekran.png" });
  await expect(image.getByRole("button", { name: "ekran.png önizləməsi" })).toBeVisible();
  await expect(image.locator("img")).toHaveAttribute("src", /^blob:/);
  const fake = files.getByRole("listitem").filter({ hasText: "saxta.png" });
  await expect(fake.locator("img")).toHaveCount(0);

  // Downloads go through the API with the bearer token, under the original name.
  const download = page.waitForEvent("download");
  await fake.getByRole("button", { name: "saxta.png", exact: true }).click();
  expect((await download).suggestedFilename()).toBe("saxta.png");

  // Expanding the thumbnail shows the image itself.
  await image.getByRole("button", { name: "ekran.png önizləməsi" }).click();
  await expect(image.getByRole("img", { name: "ekran.png" })).toBeVisible();

  // The activity feed records the upload.
  await panel.getByRole("tab", { name: "Aktivlik" }).click();
  await expect(panel.getByText('"ekran.png" faylını əlavə etdi')).toBeVisible();

  page.once("dialog", (d) => void d.accept());
  await fake.getByRole("button", { name: "Faylı sil: saxta.png" }).click();
  await expect(files.getByRole("listitem")).toHaveCount(1);
});

test("bob cannot delete alice's file; outsiders of a private project get 404", async () => {
  const alice = await apiAs("alice");
  const bob = await apiAs("bob");
  const base = `workspaces/${seed.ws}/projects`;
  expect((await alice.post(base, { data: { key: "SEC", name: "Gizli", visibility: "private" } })).status()).toBe(201);
  expect((await alice.post(`${base}/SEC/items`, { data: { title: "Gizli sənəd" } })).status()).toBe(201);
  const up = await alice.post(`${base}/SEC/items/1/attachments`, {
    multipart: { file: { name: "gizli.txt", mimeType: "text/plain", buffer: Buffer.from("gizli") } },
  });
  expect(up.status()).toBe(201);
  const { id } = (await up.json()) as { id: string };
  expect((await bob.get(`${base}/SEC/items/1/attachments`)).status()).toBe(404);
  expect((await bob.get(`${base}/SEC/items/1/attachments/${id}/content`)).status()).toBe(404);
  expect((await bob.get(`${base}/QA/items/1/attachments/${id}/content`)).status()).toBe(404);

  const pub = await alice.post(`${base}/QA/items/1/attachments`, {
    multipart: { file: { name: "a.txt", mimeType: "text/plain", buffer: Buffer.from("a") } },
  });
  const pubId = ((await pub.json()) as { id: string }).id;
  expect((await bob.delete(`${base}/QA/items/1/attachments/${pubId}`)).status()).toBe(403);
  const dl = await bob.get(`${base}/QA/items/1/attachments/${pubId}/content`);
  expect(dl.status()).toBe(200);
  expect(dl.headers()["content-disposition"]).toMatch(/^attachment/);
  expect(dl.headers()["x-content-type-options"]).toBe("nosniff");
  await alice.dispose();
  await bob.dispose();
});

test("descriptions and comments render Markdown safely", async ({ page }) => {
  await page.goto(seed.url + "?item=1");
  const panel = page.getByRole("dialog", { name: "QA-1" });

  await panel.getByRole("button", { name: "Təsviri redaktə et" }).click();
  const editor = panel.getByLabel("Təsvir");
  await editor.fill("## Plan\n\n- [x] **hazır**\n- [ ] növbəti\n\n[pis](javascript:alert(1)) <img src=x onerror=alert(1)>\n\n| A | B |\n|---|---|\n| 1 | 2 |");
  await panel.getByRole("tab", { name: "Önizləmə" }).first().click();
  await expect(panel.getByRole("heading", { name: "Plan" })).toBeVisible();
  await panel.getByRole("button", { name: "Yadda saxla" }).click();

  const desc = panel.locator(".nebula-md").first();
  await expect(desc.locator("strong")).toHaveText("hazır");
  await expect(desc.getByRole("checkbox")).toHaveCount(2);
  await expect(desc.getByRole("checkbox").first()).toBeDisabled();
  await expect(desc.locator("table td")).toHaveCount(2);
  await expect(desc.locator("img")).toHaveCount(0);
  await expect(desc.locator('a[href^="javascript"]')).toHaveCount(0);

  // Reload: the stored Markdown renders the same way.
  await page.reload();
  await expect(panel.locator(".nebula-md").first().locator("strong")).toHaveText("hazır");

  const box = panel.getByRole("textbox", { name: /Şərh yazın/ });
  await box.fill("Bax: `kod` və [sənəd](https://example.com/doc)");
  await panel.getByRole("button", { name: "Göndər" }).click();
  const comment = panel.getByRole("listitem").filter({ hasText: "Bax:" });
  await expect(comment.locator("code")).toHaveText("kod");
  const link = comment.getByRole("link", { name: "sənəd" });
  await expect(link).toHaveAttribute("rel", "noopener noreferrer nofollow");
  await expect(link).toHaveAttribute("target", "_blank");
});
