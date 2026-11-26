import { t } from "../i18n";
// Thin, typed fetch wrapper around the Nebula REST API (same origin: /api/v1).

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly field: string | undefined;
  /** Seconds the server asked us to wait (Retry-After on 429/503), if any. */
  readonly retryAfter: number | undefined;

  constructor(status: number, code: string, message: string, field?: string, retryAfter?: number) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.field = field;
    this.retryAfter = retryAfter;
  }

  get isConflict(): boolean {
    return this.status === 409;
  }
}

type TokenProvider = () => Promise<string | null>;
let tokenProvider: TokenProvider = () => Promise.resolve(null);
let onUnauthorized: () => void = () => undefined;

export function setTokenProvider(p: TokenProvider): void {
  tokenProvider = p;
}

export function setUnauthorizedHandler(h: () => void): void {
  onUnauthorized = h;
}

interface ErrorBody {
  error?: { code?: string; message?: string; field?: string };
}

export async function parseError(res: Response): Promise<ApiError> {
  let body: ErrorBody = {};
  try {
    body = (await res.json()) as ErrorBody;
  } catch {
    // non-JSON error (proxy, gateway): fall back to the status line
  }
  const code = body.error?.code ?? `http_${String(res.status)}`;
  return new ApiError(
    res.status,
    code,
    t.errorCodes[code] ?? body.error?.message ?? (res.statusText || "request failed"),
    body.error?.field,
    parseRetryAfter(res.headers.get("Retry-After")),
  );
}

/** Retry-After in delta-seconds form; the HTTP-date form is not used by our API. */
function parseRetryAfter(v: string | null): number | undefined {
  if (!v || !/^\d{1,5}$/.test(v.trim())) return undefined;
  return Number(v.trim());
}

export async function api<T>(method: string, path: string, body?: unknown, fetchImpl: typeof fetch = fetch): Promise<T> {
  if (!path.startsWith("/")) throw new Error("api path must start with /");
  const headers: Record<string, string> = { Accept: "application/json" };
  const token = await tokenProvider();
  if (token) headers.Authorization = `Bearer ${token}`;
  if (body !== undefined) headers["Content-Type"] = "application/json";

  const res = await fetchImpl("/api/v1" + path, {
    method,
    headers,
    body: body === undefined ? null : JSON.stringify(body),
    credentials: "omit", // bearer tokens only; no ambient cookies (CSRF-proof)
  });
  if (res.status === 401) {
    onUnauthorized();
  }
  if (!res.ok) throw await parseError(res);
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

/** Encodes path segments so user data can never alter the URL structure. */
export function path(strings: TemplateStringsArray, ...values: (string | number)[]): string {
  return strings.reduce((acc, s, i) => acc + s + (i < values.length ? encodeURIComponent(String(values[i])) : ""), "");
}

/**
 * Posts a multipart form (file uploads). The browser sets the multipart
 * boundary; we only add the bearer token. Statuses in `bodyOn` resolve with
 * the parsed body instead of throwing (e.g. 422 import reports).
 */
export async function apiUpload(
  path: string,
  form: FormData,
  bodyOn: readonly number[] = [],
  fetchImpl: typeof fetch = fetch,
): Promise<{ status: number; body: unknown }> {
  if (!path.startsWith("/")) throw new Error("api path must start with /");
  const headers: Record<string, string> = { Accept: "application/json" };
  const token = await tokenProvider();
  if (token) headers.Authorization = `Bearer ${token}`;
  const res = await fetchImpl("/api/v1" + path, { method: "POST", headers, body: form, credentials: "omit" });
  if (res.status === 401) onUnauthorized();
  if (!res.ok && !bodyOn.includes(res.status)) throw await parseError(res);
  return { status: res.status, body: (await res.json()) as unknown };
}

/**
 * Downloads a binary response (attachments) with the bearer token. Files are
 * never linked directly: there are no public or cookie-authenticated URLs.
 */
export async function apiBlob(path: string, fetchImpl: typeof fetch = fetch): Promise<Blob> {
  if (!path.startsWith("/")) throw new Error("api path must start with /");
  const headers: Record<string, string> = {};
  const token = await tokenProvider();
  if (token) headers.Authorization = `Bearer ${token}`;
  const res = await fetchImpl("/api/v1" + path, { headers, credentials: "omit" });
  if (res.status === 401) onUnauthorized();
  if (!res.ok) throw await parseError(res);
  return res.blob();
}

/**
 * GETs a file (table exports, ADR-0040) with its Content-Disposition, so
 * the caller can keep the server's file name.
 */
export async function apiGetFile(path: string, fetchImpl: typeof fetch = fetch): Promise<{ blob: Blob; disposition: string | null }> {
  if (!path.startsWith("/")) throw new Error("api path must start with /");
  const headers: Record<string, string> = {};
  const token = await tokenProvider();
  if (token) headers.Authorization = `Bearer ${token}`;
  const res = await fetchImpl("/api/v1" + path, { headers, credentials: "omit" });
  if (res.status === 401) onUnauthorized();
  if (!res.ok) throw await parseError(res);
  return { blob: await res.blob(), disposition: res.headers.get("Content-Disposition") };
}

/**
 * POSTs JSON and returns the binary response (evidence packages, ADR-0029)
 * with its Content-Disposition, so the caller can keep the server's file name.
 */
export async function apiPostBlob(
  path: string,
  body: unknown,
  fetchImpl: typeof fetch = fetch,
): Promise<{ blob: Blob; disposition: string | null; headers: Headers }> {
  if (!path.startsWith("/")) throw new Error("api path must start with /");
  const headers: Record<string, string> = { Accept: "application/zip, application/json", "Content-Type": "application/json" };
  const token = await tokenProvider();
  if (token) headers.Authorization = `Bearer ${token}`;
  const res = await fetchImpl("/api/v1" + path, { method: "POST", headers, body: JSON.stringify(body), credentials: "omit" });
  if (res.status === 401) onUnauthorized();
  if (!res.ok) throw await parseError(res);
  return { blob: await res.blob(), disposition: res.headers.get("Content-Disposition"), headers: res.headers };
}
