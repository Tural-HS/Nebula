import { useState, type SyntheticEvent } from "react";

import { MAX_DAYS_PER_YEAR, MAX_WEEKEND_DAYS, useCalendarApi, useWorkCalendar, type CalendarDay, type CalendarKind } from "../../api/calendar";
import { useWorkspace } from "../../api/hooks";
import { useToast } from "../../components/toast-context";
import { Button, ErrorState, Input, Label, Select, Spinner } from "../../components/ui";
import { t } from "../../i18n";
import { tc } from "../../i18n-calendar";
import { tw } from "../../i18n-workdays";
import { todayIn } from "../../lib/calendar";
import { cn } from "../../lib/cn";
import { formatDate } from "../../lib/format";
import { WEEKDAYS } from "../../lib/recurrence";
import { daysOfYear, entryProblem, jsWeekday, sortWeekdays, toggleWeekend } from "../../lib/workdays";
import { Card, SettingsLayout } from "./SettingsLayout";
import { useFeedback } from "./useErrorToast";

/** Working-day calendar settings (ADR-0027): weekend, holidays, transfers. Admin-only. */
export function WorkCalendarPage() {
  return <SettingsLayout title={tw.title}>{(ws) => <WorkCalendar ws={ws} />}</SettingsLayout>;
}

function WorkCalendar({ ws }: { ws: string }) {
  const workspace = useWorkspace(ws);
  const thisYear = Number(todayIn(workspace.data?.timezone).slice(0, 4));
  const [year, setYear] = useState(thisYear);
  const cal = useWorkCalendar(ws, `${String(year)}-01-01`, `${String(year)}-12-31`);

  return (
    <div className="space-y-4">
      <p className="max-w-3xl text-sm text-zinc-600 dark:text-zinc-400">{tw.intro}</p>
      {cal.isPending ? (
        <Spinner />
      ) : cal.isError ? (
        <ErrorState error={cal.error} onRetry={() => void cal.refetch()} />
      ) : (
        <>
          <WeekendCard key={cal.data.weekend_days.join(",")} ws={ws} initial={cal.data.weekend_days} />
          <Card label={tw.days}>
            <div className="mb-3 flex flex-wrap items-center gap-2">
              <h2 className="mr-auto text-sm font-semibold">{tw.days}</h2>
              <Button aria-label={tw.prevYear} className="pointer-coarse:min-w-11" onClick={() => { setYear((y) => y - 1); }} disabled={year <= 2000}>‹</Button>
              <span aria-live="polite" className="min-w-12 text-center text-sm font-medium tabular-nums">{year}</span>
              <Button aria-label={tw.nextYear} className="pointer-coarse:min-w-11" onClick={() => { setYear((y) => y + 1); }} disabled={year >= 2100}>›</Button>
            </div>
            <DayList ws={ws} year={year} days={daysOfYear(cal.data.days, year)} weekend={cal.data.weekend_days} />
          </Card>
          <PresetCard ws={ws} year={year} official={cal.data.az_official_years.includes(year)} />
        </>
      )}
    </div>
  );
}

function WeekendCard({ ws, initial }: { ws: string; initial: number[] }) {
  const capi = useCalendarApi(ws);
  const fb = useFeedback();
  const [days, setDays] = useState<number[]>(sortWeekdays(initial));
  const changed = days.join(",") !== sortWeekdays(initial).join(",");
  return (
    <Card label={tw.weekend}>
      <h2 className="mb-2 text-sm font-semibold">{tw.weekend}</h2>
      <div role="group" aria-label={tw.weekend} className="flex flex-wrap gap-1.5">
        {WEEKDAYS.map((d) => {
          const on = days.includes(d.value);
          return (
            <label key={d.value} title={d.long} className={cn("cursor-pointer rounded-md px-2.5 py-1 text-sm ring-1 pointer-coarse:py-2.5", on ? "bg-brand-600 text-white ring-brand-600" : "ring-zinc-300 dark:ring-zinc-700")}>
              <input
                type="checkbox"
                className="sr-only"
                aria-label={d.long}
                checked={on}
                onChange={() => { setDays((cur) => toggleWeekend(cur, d.value, MAX_WEEKEND_DAYS)); }}
              />
              {d.short}
            </label>
          );
        })}
      </div>
      <p className="mt-2 text-xs text-zinc-500 dark:text-zinc-400">{tw.weekendHint}</p>
      <Button
        variant="primary"
        className="mt-3"
        disabled={!changed || capi.setWeekend.isPending}
        onClick={() => { capi.setWeekend.mutate(days, { onSuccess: fb.ok(), onError: fb.fail }); }}
      >
        {t.save}
      </Button>
    </Card>
  );
}

const PROBLEM_TEXT = {
  date: () => tw.invalidDate,
  name: () => tw.nameRequired,
  workdayNeedsWeekend: () => tw.workdayNeedsWeekend,
  limit: () => tw.limitReached(MAX_DAYS_PER_YEAR),
} as const;

function DayList({ ws, year, days, weekend }: { ws: string; year: number; days: CalendarDay[]; weekend: number[] }) {
  const capi = useCalendarApi(ws);
  const fb = useFeedback();
  const [date, setDate] = useState("");
  const [kind, setKind] = useState<CalendarKind>("holiday");
  const [name, setName] = useState("");
  const [tried, setTried] = useState(false);

  const exists = days.some((d) => d.date === date);
  const problem = date ? entryProblem({ date, kind, name }, weekend, days.length, MAX_DAYS_PER_YEAR, exists) : "date";
  const submit = (e: SyntheticEvent) => {
    e.preventDefault();
    setTried(true);
    if (problem) return;
    capi.setDay.mutate({ date, kind, name: name.trim() }, {
      onSuccess: () => { fb.ok(tw.added)(); setDate(""); setName(""); setTried(false); },
      onError: fb.fail,
    });
  };

  return (
    <>
      <p className="mb-2 text-xs text-zinc-500 dark:text-zinc-400">{tw.count(days.length, MAX_DAYS_PER_YEAR)}</p>
      {days.length === 0 ? (
        <p className="rounded-lg bg-zinc-50 p-3 text-sm text-zinc-500 dark:text-zinc-400 dark:bg-zinc-950">{tw.empty(year)}</p>
      ) : (
        <ul className="divide-y divide-zinc-200 dark:divide-zinc-800" aria-label={tw.days}>
          {days.map((d) => {
            const wd = jsWeekday(d.date);
            const plainWeekend = d.kind === "holiday" && weekend.includes(wd);
            return (
              <li key={d.date} aria-label={`${formatDate(d.date)}: ${d.name}`} className="flex flex-wrap items-center gap-x-3 gap-y-1 py-2 text-sm">
                <span className="w-36 shrink-0 tabular-nums">
                  {formatDate(d.date)}
                  <span className="block text-xs text-zinc-500 dark:text-zinc-400">{tc.weekdaysLong[(wd + 6) % 7]}</span>
                </span>
                <span className={cn("rounded px-1.5 py-0.5 text-xs font-medium", d.kind === "holiday" ? "bg-amber-100 text-amber-900 dark:bg-amber-900/40 dark:text-amber-100" : "bg-emerald-100 text-emerald-900 dark:bg-emerald-900/40 dark:text-emerald-100")}>
                  {tw.kindShort[d.kind]}
                </span>
                <span className="min-w-0 flex-1 [overflow-wrap:anywhere]">
                  {d.name}
                  {plainWeekend && <span className="ml-1 text-xs text-zinc-500 dark:text-zinc-400">({tw.onWeekend})</span>}
                </span>
                <Button
                  variant="ghost"
                  className="text-red-600 pointer-coarse:min-h-11"
                  aria-label={tw.remove(formatDate(d.date))}
                  disabled={capi.removeDay.isPending}
                  onClick={() => { capi.removeDay.mutate(d.date, { onSuccess: fb.ok(tw.removed), onError: fb.fail }); }}
                >
                  {t.delete}
                </Button>
              </li>
            );
          })}
        </ul>
      )}
      <form onSubmit={submit} className="mt-4 grid grid-cols-1 gap-3 border-t border-zinc-200 pt-4 sm:grid-cols-[10rem_14rem_1fr_auto] sm:items-end dark:border-zinc-800">
        <div>
          <Label htmlFor="cal-date">{tw.date}</Label>
          <Input
            id="cal-date"
            type="date"
            required
            min={`${String(year)}-01-01`}
            max={`${String(year)}-12-31`}
            value={date}
            onChange={(e) => { setDate(e.target.value); }}
          />
        </div>
        <div>
          <Label htmlFor="cal-kind">{tw.kind}</Label>
          <Select
            id="cal-kind"
            value={kind}
            onChange={(e) => {
              const k = e.target.value === "workday" ? "workday" : "holiday";
              setKind(k);
              if (k === "workday" && name === "") setName(tw.defaultWorkdayName);
            }}
          >
            <option value="holiday">{tw.kinds.holiday}</option>
            <option value="workday">{tw.kinds.workday}</option>
          </Select>
        </div>
        <div>
          <Label htmlFor="cal-name">{tw.name}</Label>
          <Input id="cal-name" required maxLength={100} value={name} onChange={(e) => { setName(e.target.value); }} />
        </div>
        <Button type="submit" variant="primary" disabled={capi.setDay.isPending}>{tw.add}</Button>
        {tried && problem && (
          <p role="alert" className="text-sm text-red-600 sm:col-span-4">
            {PROBLEM_TEXT[problem]()}
          </p>
        )}
      </form>
    </>
  );
}

function PresetCard({ ws, year, official }: { ws: string; year: number; official: boolean }) {
  const capi = useCalendarApi(ws);
  const fb = useFeedback();
  const toast = useToast();
  return (
    <Card label={tw.presetTitle}>
      <h2 className="text-sm font-semibold">{tw.presetTitle}</h2>
      {official ? (
        <p role="note" className="mt-1 max-w-3xl rounded-lg bg-emerald-50 p-3 text-sm text-emerald-900 ring-1 ring-emerald-200 dark:bg-emerald-950/40 dark:text-emerald-100 dark:ring-emerald-900">
          {tw.presetOfficial(year)}
        </p>
      ) : (
        <>
          <p className="mt-1 max-w-3xl text-sm text-zinc-600 dark:text-zinc-400">{tw.presetHint}</p>
          <p role="note" className="mt-3 max-w-3xl rounded-lg bg-amber-50 p-3 text-sm text-amber-900 ring-1 ring-amber-200 dark:bg-amber-950/40 dark:text-amber-100 dark:ring-amber-900">
            {tw.presetYearly}
          </p>
        </>
      )}
      <Button
        className="mt-3"
        disabled={capi.addAzerbaijanFixed.isPending}
        onClick={() => {
          capi.addAzerbaijanFixed.mutate(year, {
            onSuccess: (r) => { toast("success", tw.presetAdded(r.added.length)); },
            onError: fb.fail,
          });
        }}
      >
        {tw.presetButton(year)}
      </Button>
    </Card>
  );
}
