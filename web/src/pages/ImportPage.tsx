import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useId, useMemo, useState, type SyntheticEvent } from "react";
import { Link, useParams } from "react-router";

import { ApiError } from "../api/client";
import { keys, useItemTypes, useMembers, useProjects, useWorkspace } from "../api/hooks";
import { previewImport, runImport, type BoardPerson, type ImportPreview, type ImportReport, type ImportTarget } from "../api/imports";
import { roleAtLeast } from "../api/types";
import { Layout } from "../components/Layout";
import { Button, EmptyState, ErrorState, Label, Select, Spinner } from "../components/ui";
import { t } from "../i18n";
import { ti } from "../i18n-import";
import { cn } from "../lib/cn";
import {
  ACCEPTED_EXTENSIONS,
  checkFile,
  columnLabel,
  initialMapping,
  mappingProblems,
  MAX_UPLOAD_BYTES,
  sampleValues,
  setTarget,
  targetLabel,
  toPayload,
  type ColumnMapping,
} from "../lib/importMapping";

type Step = "upload" | "map" | "report" | "done";
const STEPS: Step[] = ["upload", "map", "report", "done"];

function errorText(err: unknown): string {
  if (err instanceof ApiError) return ti.errors[err.code] ?? err.message;
  return t.errorGeneric;
}

export function ImportPage() {
  const { ws = "", project = "" } = useParams();
  const workspace = useWorkspace(ws);
  const projects = useProjects(ws);
  const projectName = projects.data?.find((p) => p.key === project)?.name ?? project;
  const projectUrl = `/w/${encodeURIComponent(ws)}/p/${encodeURIComponent(project)}`;

  const crumbs = [
    { label: t.workspaces, to: "/" },
    { label: workspace.data?.name ?? ws, to: `/w/${encodeURIComponent(ws)}` },
    { label: projectName, to: projectUrl },
    { label: ti.importAction },
  ];

  let body;
  if (workspace.isPending) body = <Spinner />;
  else if (workspace.isError) body = <ErrorState error={workspace.error} />;
  else if (!roleAtLeast(workspace.data.role, "admin")) body = <EmptyState>{ti.adminOnly}</EmptyState>;
  else body = <ImportWizard ws={ws} project={project} projectUrl={projectUrl} />;

  return (
    <Layout crumbs={crumbs}>
      <div className="h-full overflow-y-auto">
        <div className="mx-auto max-w-5xl p-4 md:p-6">
          <h1 className="text-lg font-semibold">{ti.title}</h1>
          <p className="mt-1 mb-6 text-sm text-zinc-500">{ti.intro}</p>
          {body}
        </div>
      </div>
    </Layout>
  );
}

function StepIndicator({ step }: { step: Step }) {
  const current = STEPS.indexOf(step);
  return (
    <ol aria-label={ti.stepLabel(current + 1, STEPS.length)} className="mb-6 flex flex-wrap gap-2 text-sm">
      {ti.steps.map((label, i) => (
        <li
          key={label}
          aria-current={i === current ? "step" : undefined}
          className={cn(
            "rounded-full px-3 py-1 ring-1",
            i === current
              ? "bg-brand-600 text-white ring-brand-600"
              : i < current
                ? "text-brand-700 ring-brand-200 dark:text-brand-100 dark:ring-brand-700"
                : "text-zinc-500 ring-zinc-200 dark:ring-zinc-700",
          )}
        >
          {String(i + 1)}. {label}
        </li>
      ))}
    </ol>
  );
}

function ImportWizard({ ws, project, projectUrl }: { ws: string; project: string; projectUrl: string }) {
  const qc = useQueryClient();
  const types = useItemTypes(ws);
  const [step, setStep] = useState<Step>("upload");
  const [file, setFile] = useState<File | null>(null);
  const [fileProblem, setFileProblem] = useState<string | null>(null);
  const [preview, setPreview] = useState<ImportPreview | null>(null);
  const [mapping, setMapping] = useState<ColumnMapping>([]);
  const [defaultType, setDefaultType] = useState("");
  const [report, setReport] = useState<ImportReport | null>(null);
  const [people, setPeople] = useState<Record<string, string>>({});
  const members = useMembers(ws);
  // Same default as the server: "task" if it exists, else the first type.
  const effectiveType = defaultType || (types.data?.find((x) => x.key === "task") ?? types.data?.[0])?.key || "";

  const load = useMutation({
    mutationFn: (v: { file: File; sheet: string }) => previewImport(ws, project, v.file, v.sheet),
    onSuccess: (p) => {
      setPreview(p);
      setMapping(initialMapping(p));
      setPeople({});
      setStep("map");
    },
  });
  const check = useMutation({
    mutationFn: (dryRun: boolean) => {
      if (!file || !preview) throw new Error("no file");
      return runImport(ws, project, file, preview.sheet, toPayload(mapping, effectiveType, people), dryRun);
    },
    onSuccess: (r) => {
      setReport(r);
      if (r.created > 0) {
        void qc.invalidateQueries({ queryKey: keys.items(ws, project) });
        setStep("done");
      } else {
        setStep("report");
      }
    },
  });

  const reset = () => {
    setStep("upload");
    setFile(null);
    setPreview(null);
    setReport(null);
    load.reset();
    check.reset();
  };

  return (
    <>
      <StepIndicator step={step} />
      {step === "upload" && (
        <UploadStep
          file={file}
          problem={fileProblem}
          pending={load.isPending}
          error={load.error}
          onFile={(f) => {
            setFile(f);
            const p = f ? checkFile(f) : null;
            setFileProblem(p ? ti.fileProblem[p] : null);
          }}
          onSubmit={() => {
            if (file && !fileProblem) load.mutate({ file, sheet: "" });
          }}
        />
      )}
      {step === "map" && preview && (
        <MapStep
          preview={preview}
          mapping={mapping}
          onMapping={setMapping}
          people={people}
          onPeople={setPeople}
          memberEmails={(members.data ?? []).map((m) => ({ email: m.email, name: m.display_name }))}
          types={types.data ?? []}
          defaultType={effectiveType}
          onDefaultType={setDefaultType}
          sheetPending={load.isPending}
          onSheet={(sheet) => {
            if (file) load.mutate({ file, sheet });
          }}
          pending={check.isPending}
          error={check.error ?? load.error}
          onBack={reset}
          onValidate={() => { check.mutate(true); }}
        />
      )}
      {step === "report" && report && preview && (
        <ReportStep
          report={report}
          preview={preview}
          pending={check.isPending}
          error={check.error}
          onBack={() => { setStep("map"); check.reset(); }}
          onImport={() => { check.mutate(false); }}
        />
      )}
      {step === "done" && report && (
        <div role="status" className="rounded-xl bg-emerald-50 p-6 ring-1 ring-emerald-200 dark:bg-emerald-950 dark:ring-emerald-900">
          <p className="text-base font-semibold text-emerald-800 dark:text-emerald-200">{ti.done(report.created)}</p>
          {report.first_key && report.last_key && (
            <p className="mt-1 font-mono text-sm text-emerald-700 dark:text-emerald-300">{ti.doneRange(report.first_key, report.last_key)}</p>
          )}
          <div className="mt-4 flex gap-2">
            <Link to={projectUrl} className="rounded-md bg-brand-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-brand-700">
              {ti.openProject}
            </Link>
            <Button onClick={reset}>{ti.importAnother}</Button>
          </div>
        </div>
      )}
    </>
  );
}

function Problem({ children }: { children: string }) {
  return (
    <p role="alert" className="mt-3 text-sm text-red-600 dark:text-red-400">
      {children}
    </p>
  );
}

function UploadStep({ file, problem, pending, error, onFile, onSubmit }: {
  file: File | null; problem: string | null; pending: boolean; error: unknown;
  onFile: (f: File | null) => void; onSubmit: () => void;
}) {
  const id = useId();
  const submit = (e: SyntheticEvent) => {
    e.preventDefault();
    onSubmit();
  };
  return (
    <form onSubmit={submit} className="rounded-xl bg-white p-5 ring-1 ring-zinc-200 dark:bg-zinc-900 dark:ring-zinc-800">
      <Label htmlFor={id}>{ti.chooseFile}</Label>
      <input
        id={id}
        type="file"
        accept={ACCEPTED_EXTENSIONS.join(",")}
        aria-describedby={`${id}-hint`}
        onChange={(e) => { onFile(e.target.files?.[0] ?? null); }}
        className="block w-full text-sm file:mr-3 file:rounded-md file:border-0 file:bg-zinc-100 file:px-3 file:py-1.5 file:text-sm file:font-medium dark:file:bg-zinc-800"
      />
      <p id={`${id}-hint`} className="mt-2 text-xs text-zinc-500">
        {ti.fileHint(MAX_UPLOAD_BYTES / (1024 * 1024), 20_000)}
      </p>
      {problem && <Problem>{problem}</Problem>}
      {error != null && <Problem>{errorText(error)}</Problem>}
      <div className="mt-4 flex justify-end">
        <Button type="submit" variant="primary" disabled={!file || !!problem || pending}>
          {pending ? ti.reading : ti.readFile}
        </Button>
      </div>
    </form>
  );
}

function TargetSelect({ id, label, value, targets, onChange }: {
  id: string; label: string; value: string; targets: ImportTarget[]; onChange: (v: string) => void;
}) {
  const builtins = targets.filter((x) => x.kind === "builtin");
  const fields = targets.filter((x) => x.kind === "field");
  return (
    <Select id={id} aria-label={label} value={value} onChange={(e) => { onChange(e.target.value); }}>
      <option value="">{ti.ignore}</option>
      <optgroup label={ti.builtinGroup}>
        {builtins.map((x) => (
          <option key={x.id} value={x.id}>
            {targetLabel(x, ti.builtin)}
            {x.required ? " *" : ""}
          </option>
        ))}
      </optgroup>
      {fields.length > 0 && (
        <optgroup label={ti.fieldsGroup}>
          {fields.map((x) => (
            <option key={x.id} value={x.id}>
              {targetLabel(x, ti.builtin)} ({x.type ? ti.fieldType[x.type] : ""})
            </option>
          ))}
        </optgroup>
      )}
    </Select>
  );
}

function MapStep(props: {
  preview: ImportPreview; mapping: ColumnMapping; onMapping: (m: ColumnMapping) => void;
  people: Record<string, string>; onPeople: (p: Record<string, string>) => void;
  memberEmails: { email: string; name: string }[];
  types: { key: string; name: string }[]; defaultType: string; onDefaultType: (k: string) => void;
  sheetPending: boolean; onSheet: (s: string) => void;
  pending: boolean; error: unknown; onBack: () => void; onValidate: () => void;
}) {
  const { preview, mapping } = props;
  const id = useId();
  const problems = mappingProblems(mapping);
  const delimiter = preview.format === "csv" ? ti.delimiter(preview.delimiter) : null;
  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-end gap-4 rounded-xl bg-white p-4 ring-1 ring-zinc-200 dark:bg-zinc-900 dark:ring-zinc-800">
        <p className="text-sm text-zinc-600 dark:text-zinc-300">
          {ti.summary(preview.row_count, preview.file_name)}
          {delimiter && <span className="ml-2 text-zinc-500">· {delimiter}</span>}
        </p>
        {preview.sheets.length > 1 && (
          <div className="min-w-[min(12rem,100%)]">
            <Label htmlFor={`${id}-sheet`}>{preview.format === "boards" ? ti.board : ti.sheet}</Label>
            <Select id={`${id}-sheet`} value={preview.sheet} disabled={props.sheetPending} onChange={(e) => { props.onSheet(e.target.value); }}>
              {preview.sheets.map((s) => (
                <option key={s} value={s}>
                  {s}
                </option>
              ))}
            </Select>
          </div>
        )}
        <div className="min-w-[min(14rem,100%)]">
          <Label htmlFor={`${id}-type`}>{ti.defaultType}</Label>
          <Select id={`${id}-type`} value={props.defaultType} onChange={(e) => { props.onDefaultType(e.target.value); }}>
            {props.types.map((x) => (
              <option key={x.key} value={x.key}>
                {x.name}
              </option>
            ))}
          </Select>
        </div>
      </div>

      <section aria-labelledby={`${id}-map`} className="rounded-xl bg-white ring-1 ring-zinc-200 dark:bg-zinc-900 dark:ring-zinc-800">
        <header className="border-b border-zinc-200 p-4 dark:border-zinc-800">
          <h2 id={`${id}-map`} className="font-medium">{ti.mappingTitle}</h2>
          <p className="mt-1 text-xs text-zinc-500">{ti.mappingHint}</p>
        </header>
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead className="text-left text-xs text-zinc-500">
              <tr>
                <th scope="col" className="px-4 py-2 font-medium">{ti.column}</th>
                <th scope="col" className="px-4 py-2 font-medium max-sm:hidden">{ti.sample}</th>
                <th scope="col" className="w-56 px-4 py-2 font-medium sm:w-72">{ti.target}</th>
              </tr>
            </thead>
            <tbody>
              {preview.headers.map((h, col) => {
                const label = columnLabel(col, h);
                return (
                  <tr key={col} className="border-t border-zinc-100 dark:border-zinc-800">
                    <th scope="row" className="px-4 py-2 text-left font-medium">{label}</th>
                    <td className="max-w-xs truncate px-4 py-2 text-zinc-500 max-sm:hidden" title={sampleValues(preview.rows, col).join(" · ")}>
                      {sampleValues(preview.rows, col).join(" · ") || "—"}
                    </td>
                    <td className="px-4 py-2">
                      <TargetSelect
                        id={`${id}-col-${String(col)}`}
                        label={`${ti.target}: ${label}`}
                        value={mapping[col] ?? ""}
                        targets={preview.targets}
                        onChange={(v) => { props.onMapping(setTarget(mapping, col, v)); }}
                      />
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </section>

      {preview.people.length > 0 && (
        <PeopleMapping people={preview.people} value={props.people} onChange={props.onPeople} members={props.memberEmails} />
      )}

      <PreviewTable preview={preview} />

      {problems.missingTitle && <Problem>{ti.missingTitle}</Problem>}
      {props.error != null && <Problem>{errorText(props.error)}</Problem>}
      <div className="flex justify-between">
        <Button onClick={props.onBack}>{ti.back}</Button>
        <Button variant="primary" disabled={problems.missingTitle || props.pending || preview.row_count === 0} onClick={props.onValidate}>
          {props.pending ? ti.validating : ti.validate}
        </Button>
      </div>
    </div>
  );
}

// Mattermost Boards archives name people only by id (ADR-0020): the admin
// tells who is who from the cards each id appears on.
function PeopleMapping({ people, value, onChange, members }: {
  people: BoardPerson[];
  value: Record<string, string>;
  onChange: (v: Record<string, string>) => void;
  members: { email: string; name: string }[];
}) {
  const id = useId();
  return (
    <section aria-labelledby={`${id}-people`} className="rounded-xl bg-white ring-1 ring-zinc-200 dark:bg-zinc-900 dark:ring-zinc-800">
      <header className="border-b border-zinc-200 p-4 dark:border-zinc-800">
        <h2 id={`${id}-people`} className="font-medium">{ti.peopleTitle}</h2>
        <p className="mt-1 text-xs text-zinc-500">{ti.peopleHint}</p>
        <p className="mt-1 text-xs text-zinc-500">{ti.personPrivateHint}</p>
      </header>
      <ul className="divide-y divide-zinc-100 dark:divide-zinc-800">
        {people.map((p, i) => {
          const label = ti.personLabel(p.cards, p.samples);
          return (
            <li key={p.id} className="flex flex-wrap items-center gap-3 px-4 py-2 text-sm">
              <label htmlFor={`${id}-p-${String(i)}`} className="min-w-0 flex-1 text-zinc-700 dark:text-zinc-300">
                <span className="block truncate" title={label}>{label}</span>
                <span className="font-mono text-xs text-zinc-400">{p.id}</span>
              </label>
              <Select
                id={`${id}-p-${String(i)}`}
                className="w-full sm:w-72"
                value={value[p.id] ?? ""}
                onChange={(e) => { onChange({ ...value, [p.id]: e.target.value }); }}
              >
                <option value="">{ti.personNone}</option>
                {members.map((m) => (
                  <option key={m.email} value={m.email}>{`${m.name} · ${m.email}`}</option>
                ))}
              </Select>
            </li>
          );
        })}
      </ul>
    </section>
  );
}

function PreviewTable({ preview }: { preview: ImportPreview }) {
  const id = useId();
  if (preview.rows.length === 0) return null;
  return (
    <section aria-labelledby={id} className="rounded-xl bg-white ring-1 ring-zinc-200 dark:bg-zinc-900 dark:ring-zinc-800">
      <h2 id={id} className="border-b border-zinc-200 p-4 font-medium dark:border-zinc-800">{ti.previewTitle}</h2>
      <div className="overflow-x-auto">
        <table className="w-full text-xs">
          <thead className="text-left text-zinc-500">
            <tr>
              <th scope="col" className="px-3 py-2 font-medium">{ti.row}</th>
              {preview.headers.map((h, i) => (
                <th key={i} scope="col" className="px-3 py-2 font-medium whitespace-nowrap">{columnLabel(i, h)}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {preview.rows.map((r) => (
              <tr key={r.row} className="border-t border-zinc-100 dark:border-zinc-800">
                <td className="px-3 py-1.5 text-zinc-500 tabular-nums">{r.row}</td>
                {r.cells.map((c, i) => (
                  <td key={i} className="max-w-48 truncate px-3 py-1.5" title={c}>{c}</td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}

function ReportStep({ report, preview, pending, error, onBack, onImport }: {
  report: ImportReport; preview: ImportPreview; pending: boolean; error: unknown; onBack: () => void; onImport: () => void;
}) {
  const id = useId();
  const ok = report.error_count === 0;
  const rejected = !report.dry_run && !ok;
  const errorRows = useMemo(() => new Set(report.errors.map((e) => e.row)).size, [report.errors]);
  return (
    <div className="space-y-5">
      <div
        role="status"
        className={cn(
          "rounded-xl p-4 text-sm ring-1",
          ok
            ? "bg-emerald-50 text-emerald-800 ring-emerald-200 dark:bg-emerald-950 dark:text-emerald-200 dark:ring-emerald-900"
            : "bg-amber-50 text-amber-900 ring-amber-200 dark:bg-amber-950 dark:text-amber-200 dark:ring-amber-900",
        )}
      >
        {rejected && <p className="mb-1 font-medium">{ti.rejected}</p>}
        {ok ? ti.reportOk(report.valid) : ti.reportErrors(errorRows, report.error_count)}
        {report.truncated && <p className="mt-1">{ti.truncated(report.errors.length)}</p>}
      </div>

      {!ok && (
        <section aria-labelledby={id} className="rounded-xl bg-white ring-1 ring-zinc-200 dark:bg-zinc-900 dark:ring-zinc-800">
          <h2 id={id} className="border-b border-zinc-200 p-4 font-medium dark:border-zinc-800">{ti.errorsTable}</h2>
          <div className="max-h-96 overflow-auto">
            <table className="w-full text-sm">
              <thead className="sticky top-0 bg-white text-left text-xs text-zinc-500 dark:bg-zinc-900">
                <tr>
                  <th scope="col" className="px-4 py-2 font-medium">{ti.row}</th>
                  <th scope="col" className="px-4 py-2 font-medium">{ti.column}</th>
                  <th scope="col" className="px-4 py-2 font-medium">{ti.message}</th>
                </tr>
              </thead>
              <tbody>
                {report.errors.map((e, i) => (
                  <tr key={i} className="border-t border-zinc-100 dark:border-zinc-800">
                    <td className="px-4 py-2 tabular-nums">{e.row}</td>
                    <td className="px-4 py-2">
                      {e.column != null ? columnLabel(e.column, preview.headers[e.column]) : e.field ?? "—"}
                    </td>
                    <td className="px-4 py-2 text-zinc-700 dark:text-zinc-300">{e.message}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      )}

      {error != null && <Problem>{errorText(error)}</Problem>}
      <div className="flex justify-between">
        <Button onClick={onBack}>{ti.fixMapping}</Button>
        {ok && (
          <Button variant="primary" disabled={pending} onClick={onImport}>
            {pending ? ti.importing : ti.importNow(report.valid)}
          </Button>
        )}
      </div>
    </div>
  );
}
