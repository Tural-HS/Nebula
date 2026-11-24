import { readFile } from "node:fs/promises";
import { inflateRawSync } from "node:zlib";

import type { Download, Page } from "@playwright/test";

import { apiAs, authFile, expect, seedWorkspace, test, type Seed } from "./fixtures";

// Item table export (ADR-0040): what the table shows (filters, search,
// visible columns, all pages) as CSV or Excel. alice exports on a desktop;
// bob, a guest, on a 320 px phone from the filter sheet. Synthetic data only.
test.describe.configure({ mode: "serial" });

let seed: Seed;

test.beforeAll(async () => {
  seed = await seedWorkspace("guest");
  const alice = await apiAs("alice");
  const ws = `workspaces/${seed.ws}`;
  expect((await alice.post(`${ws}/fields`, { data: { key: "amount", name: "Məbləğ", type: "number" } })).status()).toBe(201);
  expect((await alice.put(`${ws}/item-types/task/fields/amount`, { data: { position: 1 } })).status()).toBe(200);
  for (const [title, amount] of [["=1+1 ödəniş", -1500.5], ["Kassa çeki 2", 200], ["Kassa çeki 3", 75.25]] as const) {
    const r = await alice.post(`${ws}/projects/QA/items`, { data: { title, fields: { amount } } });
    expect(r.status(), await r.text()).toBe(201);
  }
  // A private project bob is not in: never in his exports.
  expect((await alice.post(`${ws}/projects`, { data: { key: "SEC", name: "Məxfi", visibility: "private" } })).status()).toBe(201);
  expect((await alice.post(`${ws}/projects/SEC/items`, { data: { title: "Məxfi iş" } })).status()).toBe(201);
  await alice.dispose();
});

const BOM = String.fromCharCode(0xfeff);

/** RFC 4180 parser, enough for the export (quoted fields, "" escapes, CRLF). */
function parseCSV(text: string, sep = ","): string[][] {
  const rows: string[][] = [];
  let row: string[] = [];
  let cell = "";
  let quoted = false;
  for (let i = 0; i < text.length; i++) {
    const c = text[i];
    if (quoted) {
      if (c === '"' && text[i + 1] === '"') {
        cell += '"';
        i++;
      } else if (c === '"') quoted = false;
      else cell += c;
    } else if (c === '"') quoted = true;
    else if (c === sep) {
      row.push(cell);
      cell = "";
    } else if (c === "\r" && text[i + 1] === "\n") {
      row.push(cell);
      rows.push(row);
      row = [];
      cell = "";
      i++;
    } else cell += c;
  }
  if (cell !== "" || row.length > 0) rows.push([...row, cell]);
  return rows;
}

/** The entries of a zip file (central directory; stored or deflated). */
function unzip(buf: Buffer): Map<string, string> {
  const out = new Map<string, string>();
  const end = buf.lastIndexOf(Buffer.from([0x50, 0x4b, 0x05, 0x06]));
  expect(end, "zip end of central directory").toBeGreaterThan(0);
  const count = buf.readUInt16LE(end + 10);
  let p = buf.readUInt32LE(end + 16);
  for (let n = 0; n < count; n++) {
    const method = buf.readUInt16LE(p + 10);
    const size = buf.readUInt32LE(p + 20);
    const nameLen = buf.readUInt16LE(p + 28);
    const extra = buf.readUInt16LE(p + 30);
    const comment = buf.readUInt16LE(p + 32);
    const local = buf.readUInt32LE(p + 42);
    const name = buf.subarray(p + 46, p + 46 + nameLen).toString("utf8");
    const start = local + 30 + buf.readUInt16LE(local + 26) + buf.readUInt16LE(local + 28);
    const data = buf.subarray(start, start + size);
    out.set(name, (method === 8 ? inflateRawSync(data) : data).toString("utf8"));
    p += 46 + nameLen + extra + comment;
  }
  return out;
}

async function download(page: Page, click: () => Promise<void>): Promise<{ file: Download; body: Buffer }> {
  const pending = page.waitForEvent("download");
  await click();
  const file = await pending;
  return { file, body: await readFile(await file.path()) };
}

const today = () => new Date().toISOString().slice(0, 10); // the seeded workspace's zone

test.describe("on a desktop", () => {
  test.use({ storageState: authFile("alice") });

  test("exports the filtered table as CSV and Excel", async ({ page }) => {
    // A search and the amount column: the export follows the table.
    await page.goto(`${seed.url}?q=Kassa&cols=key,title,field.amount`);
    await expect(page.getByRole("row")).toHaveCount(4); // header, 2 rows, totals
    await page.locator("summary", { hasText: "İxrac" }).click();
    const csv = await download(page, () => page.getByRole("button", { name: "CSV" }).click());
    expect(csv.file.suggestedFilename()).toBe(`Keyfiyyət-${today()}.csv`);
    const text = csv.body.toString("utf8");
    expect(text.startsWith(BOM)).toBe(true);
    expect(parseCSV(text.slice(1))).toEqual([
      ["#", "Başlıq", "Məbləğ"],
      ["QA-3", "Kassa çeki 3", "75.25"],
      ["QA-2", "Kassa çeki 2", "200"],
    ]);
    await expect(page.getByText(`Keyfiyyət-${today()}.csv endirildi`)).toBeVisible();

    // Semicolons and decimal commas for az/ru/tr Excel.
    await page.getByRole("checkbox", { name: /nöqtəli vergül/ }).check();
    const semi = await download(page, () => page.getByRole("button", { name: "CSV" }).click());
    expect(parseCSV(semi.body.toString("utf8").slice(1), ";")[1]).toEqual(["QA-3", "Kassa çeki 3", "75,25"]);

    const xlsx = await download(page, () => page.getByRole("button", { name: "Excel (.xlsx)" }).click());
    expect(xlsx.file.suggestedFilename()).toBe(`Keyfiyyət-${today()}.xlsx`);
    expect(xlsx.body.subarray(0, 2).toString()).toBe("PK");
    const parts = unzip(xlsx.body);
    expect(parts.get("xl/workbook.xml")).toContain('name="QA"');
    const sheet = parts.get("xl/worksheets/sheet1.xml") ?? "";
    expect(sheet.match(/<row /g)).toHaveLength(3); // header + 2
    expect(sheet).toContain("Kassa çeki 3");
    expect(sheet).toMatch(/<c r="C2"[^>]*><v>75\.25<\/v><\/c>/); // a number cell, not text
  });

  test("explains a refused export and offers a retry", async ({ page }) => {
    await page.route("**/items/export?**", (route) =>
      route.fulfill({ status: 429, contentType: "application/json", headers: { "Retry-After": "30" },
        body: JSON.stringify({ error: { code: "rate_limited", message: "too many exports" } }) }),
    );
    await page.goto(seed.url);
    await page.locator("summary", { hasText: "İxrac" }).click();
    await page.getByRole("button", { name: "Excel (.xlsx)" }).click();
    await expect(page.getByRole("alert")).toContainText("Çox sayda ixrac");
    await page.unroute("**/items/export?**");
    const again = await download(page, () => page.getByRole("button", { name: "Yenidən cəhd et" }).click());
    expect(again.file.suggestedFilename()).toMatch(/\.xlsx$/);
    await expect(page.getByRole("alert")).toHaveCount(0);
  });
});

test.describe("on a 320 px phone, as a guest", () => {
  test.use({ storageState: authFile("bob"), viewport: { width: 320, height: 640 } });

  test("exports from the filter sheet, neutralizing formulas", async ({ page }) => {
    await page.goto(`${seed.url}?cols=key,title,field.amount`);
    await expect(page.getByText("Kassa çeki 2")).toBeVisible();
    await page.getByRole("button", { name: /Filtrlər/ }).click();
    const sheet = page.getByRole("dialog");
    await sheet.getByRole("heading", { name: "İxrac" }).scrollIntoViewIfNeeded();
    const csv = await download(page, () => sheet.getByRole("button", { name: "CSV" }).click());
    const rows = parseCSV(csv.body.toString("utf8").slice(1));
    expect(rows).toHaveLength(4); // header + every item of QA, none of SEC
    expect(rows.map((r) => r[1])).toEqual(["Başlıq", "Kassa çeki 3", "Kassa çeki 2", "'=1+1 ödəniş"]);
    expect(rows[3]?.[2]).toBe("-1500.5"); // a real negative amount stays a number
    // Nothing on the page scrolls sideways.
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(320);

    // The API refuses the private project like the list does.
    const bob = await apiAs("bob");
    expect((await bob.get(`workspaces/${seed.ws}/projects/SEC/items/export?format=csv`)).status()).toBe(404);
    await bob.dispose();
  });
});
