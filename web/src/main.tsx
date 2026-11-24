import { loadConfig } from "./config";
import { loadMessages, messages } from "./i18n/locale";
import "./index.css";
import { startTheme } from "./lib/theme";

// The UI language is loaded before the app module (ADR-0025): dictionaries are
// module constants, so nothing that reads them may be imported earlier.
const root = document.getElementById("root") ?? document.body;

// theme-init.js already set the class; this follows OS and other-tab changes.
startTheme();

// The app chunk starts downloading as soon as the language is known, in
// parallel with config.json.
const app = loadMessages().then(() => import("./bootstrap"));

Promise.all([loadConfig(), app])
  .then(([cfg, { render }]) => {
    render(root, cfg);
  })
  .catch((err: unknown) => {
    console.error(err);
    const p = document.createElement("p");
    p.style.padding = "24px";
    p.style.fontFamily = "system-ui";
    p.textContent = messages().core.configFailed;
    root.replaceChildren(p);
  });
