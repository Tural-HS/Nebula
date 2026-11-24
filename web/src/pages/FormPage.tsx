import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { Link, useParams } from "react-router";

import { ApiError } from "../api/client";
import { formKeys, sendSubmission, usePublicForm, type PublicForm, type Submission } from "../api/forms";
import { FormFields } from "../components/FormFields";
import { Layout } from "../components/Layout";
import { Button, EmptyState, ErrorState, Spinner } from "../components/ui";
import { t } from "../i18n";
import { tf } from "../i18n-forms";
import { formatBytes } from "../lib/format";
import { inputId, toValues, valueKey, type Draft, type FieldProblem } from "../lib/forms";

const problemText: Record<FieldProblem, () => string> = {
  required: () => tf.isRequired,
  checked: () => tf.mustBeChecked,
  number: () => tf.invalidNumber,
};

/** /f/:ref — a form for signed-in requesters (ADR-0032), mobile first. */
export function FormPage() {
  const { ref = "" } = useParams();
  const q = usePublicForm(ref);
  let body;
  if (q.isPending) body = <Spinner />;
  else if (q.isError) body = q.error instanceof ApiError && q.error.status === 404 ? <EmptyState>{tf.notFound}</EmptyState> : <ErrorState error={q.error} onRetry={() => void q.refetch()} />;
  else body = <FillIn key={q.data.form.id} form={q.data.form} maxFileBytes={q.data.limits?.max_file_bytes ?? 0} />;
  return (
    <Layout crumbs={[{ label: tf.myRequests, to: "/requests" }, { label: q.data?.form.title ?? "…" }]}>
      <div className="h-full overflow-y-auto">
        <div className="mx-auto max-w-xl px-4 py-6 pb-[max(1.5rem,env(safe-area-inset-bottom))]">{body}</div>
      </div>
    </Layout>
  );
}

function FillIn({ form, maxFileBytes }: { form: PublicForm; maxFileBytes: number }) {
  const qc = useQueryClient();
  const [draft, setDraft] = useState<Draft>({});
  const [files, setFiles] = useState<File[]>([]);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [general, setGeneral] = useState<string | null>(null);
  const [done, setDone] = useState<Submission | null>(null);
  const summaryRef = useRef<HTMLDivElement>(null);
  const [showSummary, setShowSummary] = useState(0);

  useEffect(() => {
    if (showSummary > 0) summaryRef.current?.focus();
  }, [showSummary]);

  const send = useMutation({
    mutationFn: (values: Record<string, unknown>) => sendSubmission(form.slug, values, files),
    onSuccess: (s) => {
      setDone(s);
      void qc.invalidateQueries({ queryKey: formKeys.myRequests });
    },
    onError: (e) => {
      if (!(e instanceof ApiError)) {
        setGeneral(t.errorGeneric);
        return;
      }
      const key = valueKey(e.field);
      if (e.status === 422 && key && form.fields.some((f) => f.key === key)) {
        setErrors({ [key]: e.message });
        setShowSummary((n) => n + 1);
        return;
      }
      if (e.code === "form_closed") setGeneral(tf.closed);
      else if (e.code === "form_misconfigured") setGeneral(tf.misconfigured);
      else if (e.status === 429) setGeneral(tf.rateLimited);
      else setGeneral(e.message);
      setShowSummary((n) => n + 1);
    },
  });

  if (done) {
    return (
      <section aria-labelledby="form-done" className="space-y-4 rounded-xl bg-white p-6 text-center ring-1 ring-zinc-200 dark:bg-zinc-900 dark:ring-zinc-800">
        <p aria-hidden className="text-4xl">✅</p>
        <h1 id="form-done" ref={(el) => el?.focus()} tabIndex={-1} className="text-lg font-semibold outline-none">{tf.successTitle}</h1>
        <p className="text-sm text-zinc-600 dark:text-zinc-400">{tf.successBody(done.key)}</p>
        <p className="text-base font-semibold tabular-nums" data-testid="submission-key">{done.key}</p>
        <div className="flex flex-col gap-2 sm:flex-row sm:justify-center">
          <Link to={`/requests/${encodeURIComponent(done.id)}`} className="inline-flex items-center justify-center rounded-md bg-brand-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-brand-700 pointer-coarse:min-h-11">
            {tf.myRequests}
          </Link>
          <Button onClick={() => { setDone(null); setDraft({}); setFiles([]); setErrors({}); setGeneral(null); }}>{tf.submitAnother}</Button>
        </div>
      </section>
    );
  }

  const submit = () => {
    setGeneral(null);
    const { values, problems } = toValues(form.fields, draft);
    const next: Record<string, string> = {};
    for (const [k, p] of problems) next[k] = problemText[p]();
    setErrors(next);
    if (problems.length > 0) {
      setShowSummary((n) => n + 1);
      return;
    }
    send.mutate(values);
  };

  const prefix = "ff";
  const errorList = form.fields.filter((f) => errors[f.key]);
  const addFiles = (list: FileList | null) => {
    if (!list) return;
    const picked = [...files, ...Array.from(list)];
    if (picked.length > form.max_files) {
      setGeneral(tf.tooManyFiles(form.max_files));
      return;
    }
    const big = picked.find((f) => maxFileBytes > 0 && f.size > maxFileBytes);
    if (big) {
      setGeneral(tf.fileTooLarge(big.name, formatBytes(maxFileBytes)));
      return;
    }
    setGeneral(null);
    setFiles(picked);
  };

  return (
    <form
      noValidate
      aria-labelledby="form-title"
      className="space-y-6"
      onSubmit={(e) => {
        e.preventDefault();
        submit();
      }}
    >
      <header className="space-y-2">
        <h1 id="form-title" className="text-xl font-semibold">{form.title}</h1>
        <p className="text-xs text-zinc-500 dark:text-zinc-400">{tf.sentTo(form.workspace)}</p>
        {form.intro && <p className="text-sm whitespace-pre-line text-zinc-700 dark:text-zinc-300">{form.intro}</p>}
      </header>

      {!form.open && !form.can_manage ? (
        <p role="status" className="rounded-md bg-amber-50 p-3 text-sm text-amber-900 ring-1 ring-amber-200 dark:bg-amber-950 dark:text-amber-200 dark:ring-amber-900">
          {tf.closed}
        </p>
      ) : (
        <>
          {!form.open && <p role="note" className="rounded-md bg-amber-50 p-3 text-sm text-amber-900 dark:bg-amber-950 dark:text-amber-200">{tf.closed}</p>}
          {(errorList.length > 0 || general) && (
            <div ref={summaryRef} tabIndex={-1} role="alert" aria-labelledby="form-errors" className="rounded-lg border border-red-300 bg-red-50 p-4 text-sm text-red-900 outline-none focus:ring-2 focus:ring-red-500 dark:border-red-900 dark:bg-red-950 dark:text-red-200">
              {errorList.length > 0 && (
                <>
                  <h2 id="form-errors" className="font-semibold">{tf.errorSummary(errorList.length)}</h2>
                  <ul className="mt-2 list-disc space-y-1 pl-5">
                    {errorList.map((f) => (
                      <li key={f.key}>
                        <a
                          href={`#${inputId(prefix, f.key)}`}
                          className="underline"
                          onClick={(e) => {
                            e.preventDefault();
                            document.getElementById(inputId(prefix, f.key))?.focus();
                          }}
                        >
                          {f.label}: {errors[f.key]}
                        </a>
                      </li>
                    ))}
                  </ul>
                </>
              )}
              {general && <p id={errorList.length > 0 ? undefined : "form-errors"} className={errorList.length > 0 ? "mt-2" : undefined}>{general}</p>}
            </div>
          )}
          <FormFields
            fields={form.fields}
            draft={draft}
            errors={errors}
            prefix={prefix}
            disabled={send.isPending}
            onChange={(k, v) => {
              setDraft((d) => ({ ...d, [k]: v }));
              if (errors[k]) {
                setErrors((cur) => Object.fromEntries(Object.entries(cur).filter(([key]) => key !== k)));
              }
            }}
          />
          {form.max_files > 0 && (
            <div>
              <label htmlFor="ff-files" className="mb-1.5 block text-sm font-medium">{tf.files}</label>
              <input
                id="ff-files"
                type="file"
                multiple
                disabled={send.isPending || files.length >= form.max_files}
                aria-describedby="ff-files-hint"
                className="block w-full text-sm file:mr-3 file:rounded-md file:border-0 file:bg-zinc-100 file:px-3 file:py-2 dark:file:bg-zinc-800"
                onChange={(e) => { addFiles(e.target.files); e.target.value = ""; }}
              />
              <p id="ff-files-hint" className="mt-1 text-xs text-zinc-500 dark:text-zinc-400">{tf.filesHint(form.max_files, formatBytes(maxFileBytes))}</p>
              {files.length > 0 && (
                <ul className="mt-2 divide-y divide-zinc-100 text-sm dark:divide-zinc-800">
                  {files.map((f, i) => (
                    <li key={`${f.name}-${String(i)}`} className="flex items-center gap-2 py-1.5">
                      <span className="min-w-0 flex-1 truncate">{f.name}</span>
                      <span className="text-xs text-zinc-500 dark:text-zinc-400 tabular-nums">{formatBytes(f.size)}</span>
                      <Button variant="ghost" aria-label={tf.removeFile(f.name)} onClick={() => { setFiles(files.filter((_, j) => j !== i)); }}>✕</Button>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          )}
          <Button type="submit" variant="primary" className="w-full sm:w-auto" disabled={send.isPending || !form.open}>
            {send.isPending ? tf.submitting : tf.submit}
          </Button>
        </>
      )}
    </form>
  );
}
