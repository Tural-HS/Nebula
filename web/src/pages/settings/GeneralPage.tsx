import { useState } from "react";

import { useAdminApi } from "../../api/admin";
import { useWorkspace } from "../../api/hooks";
import type { Workspace } from "../../api/types";
import { Button, Input, Select, Spinner } from "../../components/ui";
import { t } from "../../i18n";
import { ta } from "../../i18n-admin";
import { tth } from "../../i18n-theme";
import { ACCENT_SCALES, ACCENTS, DEFAULT_ACCENT, isAccent, type Accent } from "../../lib/accent";
import { cn } from "../../lib/cn";
import { Card, SettingsLayout } from "./SettingsLayout";
import { useFeedback } from "./useErrorToast";

/** IANA zones known to this browser; the server validates again. */
function timeZones(current: string): string[] {
  let zones: string[] = [];
  try {
    zones = Intl.supportedValuesOf("timeZone");
  } catch {
    // very old browser: offer the current zone and UTC only
  }
  return [...new Set(["UTC", current, ...zones])];
}

export function GeneralPage() {
  return <SettingsLayout title={ta.nav.general}>{(ws) => <General ws={ws} />}</SettingsLayout>;
}

function General({ ws }: { ws: string }) {
  const workspace = useWorkspace(ws);
  if (!workspace.data) return <Spinner />;
  return (
    <div className="space-y-4">
      <GeneralForm key={workspace.data.id} ws={ws} workspace={workspace.data} />
      {/* Hidden if the API predates accents (ADR-0035, migration 00028). */}
      {workspace.data.accent !== undefined && <AccentPicker ws={ws} accent={workspace.data.accent} />}
    </div>
  );
}

/** Curated, contrast-checked accents (lib/accent.ts); saved on pick. */
function AccentPicker({ ws, accent }: { ws: string; accent: string | null }) {
  const adm = useAdminApi(ws);
  const fb = useFeedback();
  const current: Accent = isAccent(accent) ? accent : DEFAULT_ACCENT;
  return (
    <Card>
      <fieldset disabled={adm.updateWorkspace.isPending}>
        <legend className="text-sm font-medium">{tth.accent}</legend>
        <p className="mt-1 text-xs text-zinc-500 dark:text-zinc-400">{tth.accentHint}</p>
        <div className="mt-3 grid grid-cols-2 gap-2 sm:grid-cols-4">
          {ACCENTS.map((a) => (
            <label
              key={a}
              className={cn(
                "flex cursor-pointer items-center gap-2 rounded-lg px-2.5 py-2 text-sm ring-1 ring-zinc-200 has-[:checked]:ring-2 has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-offset-2 has-[:focus-visible]:outline-brand-500 pointer-coarse:min-h-11 dark:ring-zinc-700",
                a === current && "ring-zinc-900 dark:ring-zinc-100",
              )}
            >
              <input
                type="radio"
                name="accent"
                value={a}
                checked={a === current}
                className="sr-only"
                onChange={() => {
                  adm.updateWorkspace.mutate({ accent: a === DEFAULT_ACCENT ? null : a }, { onSuccess: fb.ok(tth.accentSaved), onError: fb.fail });
                }}
              />
              <span aria-hidden className="flex shrink-0 overflow-hidden rounded-full ring-1 ring-black/10">
                <span className="size-4" style={{ background: ACCENT_SCALES[a][600] }} />
                <span className="size-4" style={{ background: ACCENT_SCALES[a][300] }} />
              </span>
              <span className="min-w-0 truncate">{tth.accents[a]}</span>
            </label>
          ))}
        </div>
      </fieldset>
    </Card>
  );
}

function GeneralForm({ ws, workspace }: { ws: string; workspace: Workspace }) {
  const adm = useAdminApi(ws);
  const fb = useFeedback();
  const [name, setName] = useState(workspace.name);
  const [timezone, setTimezone] = useState(workspace.timezone);
  const [hour, setHour] = useState(workspace.overdue_digest_hour === null ? "" : String(workspace.overdue_digest_hour));

  return (
    <Card>
      <form
        className="max-w-lg space-y-4"
        onSubmit={(e) => {
          e.preventDefault();
          adm.updateWorkspace.mutate(
            { name, timezone, overdue_digest_hour: hour === "" ? null : Number(hour) },
            { onSuccess: fb.ok(), onError: fb.fail },
          );
        }}
      >
        <label className="block text-sm">
          <span className="mb-1 block text-zinc-600 dark:text-zinc-400">{ta.general.name}</span>
          <Input required maxLength={200} value={name} onChange={(e) => { setName(e.target.value); }} />
        </label>
        <label className="block text-sm">
          <span className="mb-1 block text-zinc-600 dark:text-zinc-400">{ta.general.timezone}</span>
          <Select value={timezone} onChange={(e) => { setTimezone(e.target.value); }}>
            {timeZones(workspace.timezone).map((z) => <option key={z} value={z}>{z}</option>)}
          </Select>
          <span className="mt-1 block text-xs text-zinc-500 dark:text-zinc-400">{ta.general.timezoneHint}</span>
        </label>
        <label className="block text-sm">
          <span className="mb-1 block text-zinc-600 dark:text-zinc-400">{ta.general.digestHour}</span>
          <Select value={hour} onChange={(e) => { setHour(e.target.value); }} className="w-40">
            <option value="">{ta.general.digestOff}</option>
            {Array.from({ length: 24 }, (_, h) => (
              <option key={h} value={String(h)}>{String(h).padStart(2, "0")}:00</option>
            ))}
          </Select>
          <span className="mt-1 block text-xs text-zinc-500 dark:text-zinc-400">{ta.general.digestHint}</span>
        </label>
        <Button type="submit" variant="primary" disabled={adm.updateWorkspace.isPending}>{t.save}</Button>
      </form>
    </Card>
  );
}
