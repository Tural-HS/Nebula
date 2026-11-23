// Turns a display name into an API key ("Müştəri növü" -> "musteri_novu").
const MAP: Record<string, string> = { ə: "e", ı: "i", ö: "o", ü: "u", ş: "s", ç: "c", ğ: "g", İ: "i" };

export function slugKey(name: string): string {
  const ascii = name
    .toLocaleLowerCase("az")
    .replace(/[əıöüşçğ]/g, (ch) => MAP[ch] ?? ch)
    .normalize("NFKD")
    .replace(/[̀-ͯ]/g, "");
  const key = ascii.replace(/[^a-z0-9]+/g, "_").replace(/^_+|_+$/g, "").replace(/^[0-9_]+/, "");
  return key.slice(0, 63);
}
