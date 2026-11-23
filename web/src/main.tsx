import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import { App } from "./App";
import { AuthProvider } from "./auth/auth";
import { createUserManager } from "./auth/oidc";
import { loadConfig } from "./config";
import "./index.css";

const root = createRoot(document.getElementById("root") ?? document.body);

loadConfig()
  .then((cfg) => {
    const manager = createUserManager(cfg);
    root.render(
      <StrictMode>
        <AuthProvider manager={manager}>
          <App />
        </AuthProvider>
      </StrictMode>,
    );
  })
  .catch((err: unknown) => {
    console.error(err);
    root.render(<p style={{ padding: 24, fontFamily: "system-ui" }}>Nebula konfiqurasiyası yüklənmədi.</p>);
  });
