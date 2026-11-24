import { Component, type ErrorInfo, type ReactNode } from "react";

import { t } from "../i18n";
import { isChunkLoadError, reloadOnceForChunk } from "../lib/chunkReload";
import { Button } from "./ui";

interface State {
  error: unknown;
}

/** Catches render errors so the app never turns into a blank page. */
export class ErrorBoundary extends Component<{ children: ReactNode }, State> {
  override state: State = { error: null };

  static getDerivedStateFromError(error: unknown): State {
    return { error };
  }

  override componentDidCatch(error: unknown, info: ErrorInfo) {
    if (isChunkLoadError(error) && reloadOnceForChunk()) return;
    console.error("Unhandled UI error", error, info.componentStack);
  }

  override render() {
    if (this.state.error === null) return this.props.children;
    const newVersion = isChunkLoadError(this.state.error);
    return (
      <div role="alert" className="mx-auto mt-24 max-w-md space-y-4 p-6 text-center">
        <p className="text-base font-semibold">{newVersion ? t.newVersion : t.errorGeneric}</p>
        <p className="text-sm text-zinc-500 dark:text-zinc-400">{newVersion ? t.newVersionHint : t.unexpectedErrorHint}</p>
        <Button variant="primary" onClick={() => { window.location.reload(); }}>{t.reload}</Button>
      </div>
    );
  }
}
