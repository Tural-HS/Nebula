// Azerbaijani date formatting implemented locally: browser Intl data for "az"
// is incomplete in Chromium (it renders "2026 M11 23" and "-31 min").

const MONTHS = ["yan", "fev", "mar", "apr", "may", "iyn", "iyl", "avq", "sen", "okt", "noy", "dek"] as const;

const pad = (n: number) => String(n).padStart(2, "0");

export function formatDateTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "—";
  return `${String(d.getDate())} ${MONTHS[d.getMonth()] ?? ""} ${String(d.getFullYear())}, ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

const UNITS: [label: string, seconds: number][] = [
  ["il", 365 * 24 * 3600],
  ["ay", 30 * 24 * 3600],
  ["gün", 24 * 3600],
  ["saat", 3600],
  ["dəqiqə", 60],
];

/** "5 dəqiqə əvvəl", "2 gün sonra", "indicə". */
export function formatRelative(iso: string, now: number = Date.now()): string {
  const t = new Date(iso).getTime();
  if (Number.isNaN(t)) return "—";
  const diff = Math.round((t - now) / 1000);
  for (const [label, secs] of UNITS) {
    const n = Math.floor(Math.abs(diff) / secs);
    if (n >= 1) return `${String(n)} ${label} ${diff < 0 ? "əvvəl" : "sonra"}`;
  }
  return "indicə";
}
