// English strings for recurring items (ADR-0012).
import type { Messages } from "../../i18n/locale";

const MONTHS = ["January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"] as const;

/** 1 → "1st", 2 → "2nd", 11 → "11th", 22 → "22nd". */
function ordinal(n: number): string {
  const rem100 = n % 100;
  if (rem100 >= 11 && rem100 <= 13) return `${String(n)}th`;
  const suffix = ["th", "st", "nd", "rd"][n % 10] ?? "th";
  return `${String(n)}${suffix}`;
}

/** day: 1..31, or -1 for the last day of the month. */
const dayLabel = (day: number) => (day === -1 ? "the last day" : `the ${ordinal(day)}`);

const every = (n: number, one: string, many: string) => (n === 1 ? `Every ${one}` : `Every ${String(n)} ${many}`);

export const tr = {
  nav: "Recurring",
  title: "Recurring items",
  intro: "Each time, a rule creates only the next item, a few days before its due date. The item's due date is the occurrence date, so unfinished work automatically becomes \"overdue\".",
  newRule: "New rule",
  editRule: "Edit rule",
  empty: "No recurring items yet.",
  titleLabel: "Title template",
  titleHint: "{{month}}, {{year}} and {{date}} are replaced with the date: \"Office rent — {{month}} payment\".",
  freq: "Frequency",
  freqs: { daily: "Daily", weekly: "Weekly", monthly: "Monthly", yearly: "Yearly" },
  every: "Repeat every",
  everyUnit: { daily: "day(s)", weekly: "week(s)", monthly: "month(s)", yearly: "year(s)" },
  weekdays: "Days of the week",
  monthDay: "Day of the month",
  lastDay: "Last day of the month",
  month: "Month",
  months: [...MONTHS],
  start: "Start",
  end: "End (optional)",
  lead: "Create how many days in advance",
  enabled: "Active",
  next: "Next",
  upcoming: "Upcoming dates",
  ended: "Ended",
  disabled: "Paused",
  failed: "Error: rule paused",
  confirmDelete: "Delete this rule? Items already created will be kept.",
  adminOnlyEdit: "Only admins can change rules.",
  noAccess: "Recurring items are not available to guests.",

  /** Monday first. */
  weekdayShort: ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"],
  weekdayLong: ["Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"],
  /** One-line schedule summaries ("Monthly on the 9th"); day is 1..31 or -1 (last day), month 1..12. */
  describe: {
    daily: (n: number) => every(n, "day", "days"),
    weekly: (n: number, days: string) => `${every(n, "week", "weeks")}: ${days}`,
    monthly: (n: number, day: number) => (n === 1 ? `Monthly on ${dayLabel(day)}` : `Every ${String(n)} months on ${dayLabel(day)}`),
    yearly: (n: number, month: number, day: number) => {
      const name = MONTHS[month - 1] ?? "";
      const date = day === -1 ? `the last day of ${name}` : `${name} ${String(day)}`;
      return `${every(n, "year", "years")} on ${date}`;
    },
  },
} satisfies Messages["recur"];
