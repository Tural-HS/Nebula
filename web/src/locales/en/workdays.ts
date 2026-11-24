// English strings for the working-day calendar (ADR-0027).
import type { Messages } from "../../i18n/locale";

const MONTHS = ["January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"] as const;

function ordinal(n: number): string {
  const rem100 = n % 100;
  if (rem100 >= 11 && rem100 <= 13) return `${String(n)}th`;
  return `${String(n)}${["th", "st", "nd", "rd"][n % 10] ?? "th"}`;
}

const workDayLabel = (workDay: number) => (workDay === -1 ? "last working day" : `${ordinal(workDay)} working day`);

export const tw = {
  nav: "Working days",
  title: "Working-day calendar",
  intro:
    "Weekends, holidays and transferred working days decide the dates of recurring items: \"last working day of the month\", \"5th working day of the month\" and moving a date that lands on a non-working day. A change recomputes the next date of the rules at once; no items are created for past dates and existing items are not touched.",
  weekend: "Weekend days",
  weekendHint: "At most 3 days. The calendar view marks these days too.",
  year: "Year",
  prevYear: "Previous year",
  nextYear: "Next year",
  days: "Holidays and transfers",
  empty: (year: number) => `No dates added for ${String(year)} yet.`,
  count: (n: number, max: number) => `${String(n)} / ${String(max)} dates`,
  date: "Date",
  name: "Name",
  kind: "Kind",
  kinds: { holiday: "Non-working day (holiday)", workday: "Working day (transfer)" },
  kindShort: { holiday: "Holiday", workday: "Working day" },
  defaultWorkdayName: "Transferred working day",
  add: "Add",
  remove: (date: string) => `Remove ${date}`,
  added: "Date added",
  removed: "Date removed",
  workdayNeedsWeekend: "Only a weekend date can be declared a working day.",
  invalidDate: "Pick a valid date between 2000 and 2100.",
  nameRequired: "A name is required (at most 100 characters).",
  limitReached: (max: number) => `A year can have at most ${String(max)} dates.`,
  onWeekend: "falls on a weekend",
  presetTitle: "Fixed-date holidays of Azerbaijan",
  presetButton: (year: number) => `Add to ${String(year)}`,
  presetHint:
    "Adds the non-working days with a fixed date in article 105 of the Labour Code: 1–2 January, 8 March, 9 May, 28 May, 15 June, 26 June, 8 and 9 November, 31 December. Dates you already added are left alone.",
  presetYearly:
    "Confirm every year: the days of Novruz, Ramazan and Qurban bayramı, the rest days moved when a holiday falls on a weekend, and working-day transfers are announced each year by a decision of the Cabinet of Ministers. Add them here by hand once the decision is published.",
  presetAdded: (n: number) => (n === 0 ? "Nothing new: every date is already there." : n === 1 ? "1 date added" : `${String(n)} dates added`),

  holidayLabel: (name: string) => `Non-working day: ${name}`,
  workdayLabel: (name: string) => `Working day: ${name}`,
  weekendLabel: "Weekend",

  dayMode: "Day",
  dayModes: { date: "Day of the month", workday: "Working day" },
  workDay: "Which working day",
  lastWorkDay: "Last working day",
  workDayOption: (n: number) => (n === -1 ? "Last working day" : `${ordinal(n)} working day`),
  shift: "If it lands on a non-working day",
  shifts: { none: "Keep the date", previous: "Previous working day", next: "Next working day" },
  calendarHint: "Weekends and holidays come from the workspace's working-day calendar.",
  describe: {
    monthly: (n: number, workDay: number) =>
      n === 1 ? `Monthly on the ${workDayLabel(workDay)}` : `Every ${String(n)} months on the ${workDayLabel(workDay)}`,
    yearly: (n: number, month: number, workDay: number) =>
      `${n === 1 ? "Every year" : `Every ${String(n)} years`} on the ${workDayLabel(workDay)} of ${MONTHS[month - 1] ?? ""}`,
    shift: { previous: " (if a non-working day: the previous working day)", next: " (if a non-working day: the next working day)" },
  },
} satisfies Messages["workdays"];
