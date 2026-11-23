/** One id per run: every workspace this run creates is tagged with it, so the
 *  teardown removes exactly those and nothing else. Workers inherit the env. */
export default function globalSetup() {
  process.env.E2E_RUN = Date.now().toString(36);
}
