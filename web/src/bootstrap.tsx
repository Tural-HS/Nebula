import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import { App } from "./App";
import { AuthProvider } from "./auth/auth";
import { createUserManager } from "./auth/oidc";
import type { AppConfig } from "./config";

export function render(container: HTMLElement, cfg: AppConfig): void {
  createRoot(container).render(
    <StrictMode>
      <AuthProvider manager={createUserManager(cfg)}>
        <App />
      </AuthProvider>
    </StrictMode>,
  );
}
