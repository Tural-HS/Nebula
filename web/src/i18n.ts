// UI strings in the active language (ADR-0025); the texts are in locales/<lang>/core.ts.
import { messages } from "./i18n/locale";

export const t = messages().core;
