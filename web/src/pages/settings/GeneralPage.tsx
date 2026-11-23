import { useState } from "react";

import { useAdminApi } from "../../api/admin";
import { useWorkspace } from "../../api/hooks";
import type { Workspace } from "../../api/types";
import { Button, Input, Select, Spinner } from "../../components/ui";
import { t } from "../../i18n";
import { ta } from "../../i18n-admin";
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
  return <GeneralForm key={workspace.data.id} ws={ws} workspace={workspace.data} />;
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
          <span className="mt-1 block text-xs text-zinc-500">{ta.general.timezoneHint}</span>
        </label>
        <label className="block text-sm">
          <span className="mb-1 block text-zinc-600 dark:text-zinc-400">{ta.general.digestHour}</span>
          <Select value={hour} onChange={(e) => { setHour(e.target.value); }} className="w-40">
            <option value="">{ta.general.digestOff}</option>
            {Array.from({ length: 24 }, (_, h) => (
              <option key={h} value={String(h)}>{String(h).padStart(2, "0")}:00</option>
            ))}
          </Select>
          <span className="mt-1 block text-xs text-zinc-500">{ta.general.digestHint}</span>
        </label>
        <Button type="submit" variant="primary" disabled={adm.updateWorkspace.isPending}>{t.save}</Button>
      </form>
    </Card>
  );
}
