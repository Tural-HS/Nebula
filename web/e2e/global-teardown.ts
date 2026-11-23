import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

/**
 * Deletes the workspaces this run created from the local compose database.
 * There is deliberately no "delete workspace" API yet, so this goes through
 * psql in the postgres container. A failure only warns: the tests already
 * passed or failed on their own, and `make e2e-clean` removes leftovers.
 */
export default function globalTeardown() {
  const run = process.env.E2E_RUN;
  if (!run || !/^[0-9a-z]+$/.test(run) || process.env.E2E_KEEP_DATA) return;
  const root = resolve(import.meta.dirname, "../..");
  try {
    execFileSync(
      "docker",
      ["compose", "exec", "-T", "postgres", "psql", "-q", "-U", "nebula", "-d", "nebula", "-v", `pattern=^e2e-${run}-[0-9a-z]+$`],
      { cwd: root, input: readFileSync(resolve(import.meta.dirname, "cleanup.sql")), stdio: ["pipe", "ignore", "pipe"] },
    );
  } catch (err) {
    console.warn(`E2E cleanup failed (run ${run}); run \`make e2e-clean\`:`, err instanceof Error ? err.message : err);
  }
}
