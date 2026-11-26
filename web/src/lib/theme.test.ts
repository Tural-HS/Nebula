// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import initSrc from "../../public/theme-init.js?raw";
import { applyTheme, parseTheme, resolveTheme, THEME_COLORS, THEME_STORAGE_KEY } from "./theme";

describe("parseTheme", () => {
  it("keeps a known choice", () => {
    expect(parseTheme("dark")).toBe("dark");
    expect(parseTheme("light")).toBe("light");
    expect(parseTheme("system")).toBe("system");
  });
  it("falls back to the system setting", () => {
    expect(parseTheme(null)).toBe("system");
    expect(parseTheme("")).toBe("system");
    expect(parseTheme("DARK")).toBe("system");
    expect(parseTheme("sepia")).toBe("system");
  });
});

describe("resolveTheme", () => {
  it("follows the OS only for system", () => {
    expect(resolveTheme("system", true)).toBe("dark");
    expect(resolveTheme("system", false)).toBe("light");
    expect(resolveTheme("light", true)).toBe("light");
    expect(resolveTheme("dark", false)).toBe("dark");
  });
});

describe("applyTheme", () => {
  beforeEach(() => {
    document.head.innerHTML = '<meta name="theme-color" content="x" media="(prefers-color-scheme: light)"><meta name="theme-color" content="y">';
    document.documentElement.className = "";
  });

  it("toggles the dark class, color-scheme and theme-color", () => {
    applyTheme(document, "dark");
    expect(document.documentElement.classList.contains("dark")).toBe(true);
    expect(document.documentElement.style.colorScheme).toBe("dark");
    for (const m of document.querySelectorAll<HTMLMetaElement>('meta[name="theme-color"]')) expect(m.content).toBe(THEME_COLORS.dark);

    applyTheme(document, "light");
    expect(document.documentElement.classList.contains("dark")).toBe(false);
    expect(document.documentElement.style.colorScheme).toBe("light");
    for (const m of document.querySelectorAll<HTMLMetaElement>('meta[name="theme-color"]')) expect(m.content).toBe(THEME_COLORS.light);
  });
});

function mockMatchMedia(dark: boolean) {
  const listeners: ((e: { matches: boolean }) => void)[] = [];
  const mq = {
    matches: dark,
    addEventListener: (_: string, l: (e: { matches: boolean }) => void) => listeners.push(l),
    removeEventListener: () => undefined,
  };
  vi.stubGlobal("matchMedia", () => mq);
  return {
    flip(next: boolean) {
      mq.matches = next;
      for (const l of listeners) l({ matches: next });
    },
  };
}

describe("setTheme and startTheme", () => {
  beforeEach(() => {
    vi.resetModules();
    localStorage.clear();
    document.documentElement.className = "";
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("stores the choice, applies it at once and forgets it for system", async () => {
    mockMatchMedia(false);
    const theme = await import("./theme");
    theme.startTheme();
    expect(theme.currentTheme()).toBe("system");
    expect(document.documentElement.classList.contains("dark")).toBe(false);

    theme.setTheme("dark");
    expect(localStorage.getItem(THEME_STORAGE_KEY)).toBe("dark");
    expect(document.documentElement.classList.contains("dark")).toBe(true);

    theme.setTheme("system");
    expect(localStorage.getItem(THEME_STORAGE_KEY)).toBeNull();
    expect(document.documentElement.classList.contains("dark")).toBe(false);
  });

  it("follows the OS while system is chosen, and ignores it otherwise", async () => {
    const os = mockMatchMedia(false);
    const theme = await import("./theme");
    theme.startTheme();
    os.flip(true);
    expect(document.documentElement.classList.contains("dark")).toBe(true);

    theme.setTheme("light");
    os.flip(true);
    expect(document.documentElement.classList.contains("dark")).toBe(false);
  });

  it("reads the stored choice at start-up", async () => {
    mockMatchMedia(false);
    localStorage.setItem(THEME_STORAGE_KEY, "dark");
    const theme = await import("./theme");
    theme.startTheme();
    expect(theme.currentTheme()).toBe("dark");
    expect(document.documentElement.classList.contains("dark")).toBe(true);
  });

  it("still switches when storage is blocked", async () => {
    mockMatchMedia(false);
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => { throw new DOMException("blocked"); });
    const theme = await import("./theme");
    theme.startTheme();
    theme.setTheme("dark");
    expect(document.documentElement.classList.contains("dark")).toBe(true);
    vi.restoreAllMocks();
  });
});

describe("theme-init.js", () => {
  // The pre-paint script duplicates the logic; run it against the same cases.
  const run = () => {
    // eslint-disable-next-line @typescript-eslint/no-implied-eval -- runs our own public/theme-init.js, as the browser would
    (new Function(initSrc) as () => void)();
  };

  beforeEach(() => {
    localStorage.clear();
    document.documentElement.className = "";
    document.head.innerHTML = '<meta name="theme-color" content="x">';
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it.each([
    [null, false, "light"],
    [null, true, "dark"],
    ["light", true, "light"],
    ["dark", false, "dark"],
    ["junk", true, "dark"],
  ] as const)("stored %s, OS dark %s → %s", (stored, osDark, want) => {
    mockMatchMedia(osDark);
    if (stored) localStorage.setItem(THEME_STORAGE_KEY, stored);
    run();
    expect(document.documentElement.classList.contains("dark")).toBe(want === "dark");
    expect(document.documentElement.style.colorScheme).toBe(want);
    expect(document.querySelector<HTMLMetaElement>('meta[name="theme-color"]')?.content).toBe(THEME_COLORS[want]);
    expect(resolveTheme(parseTheme(stored), osDark)).toBe(want);
  });
});

describe("extra palettes", () => {
  it("keeps the dark variant for midnight and marks palettes with data-theme", () => {
    applyTheme(document, "midnight");
    expect(document.documentElement.classList.contains("dark")).toBe(true);
    expect(document.documentElement.dataset.theme).toBe("midnight");
    expect(document.documentElement.style.colorScheme).toBe("dark");
    applyTheme(document, "paper");
    expect(document.documentElement.classList.contains("dark")).toBe(false);
    expect(document.documentElement.dataset.theme).toBe("paper");
    applyTheme(document, "green");
    expect(document.documentElement.classList.contains("dark")).toBe(false);
    expect(document.documentElement.dataset.theme).toBe("green");
    applyTheme(document, "contrast");
    expect(document.documentElement.dataset.theme).toBe("contrast");
    expect(document.documentElement.style.colorScheme).toBe("light");
    applyTheme(document, "light");
    expect(document.documentElement.dataset.theme).toBeUndefined();
  });
  it("parses stored palettes and still falls back to system", () => {
    expect(parseTheme("paper")).toBe("paper");
    expect(parseTheme("midnight")).toBe("midnight");
    expect(parseTheme("green")).toBe("green");
    expect(parseTheme("sepia")).toBe("system");
    expect(resolveTheme("contrast", true)).toBe("contrast");
  });
});

describe("dark palettes (ADR-0058)", () => {
  it.each(["evergreen", "oled", "dusk", "mocha"] as const)("%s is dark and marked with data-theme", (name) => {
    applyTheme(document, name);
    expect(document.documentElement.classList.contains("dark")).toBe(true);
    expect(document.documentElement.dataset.theme).toBe(name);
    expect(document.documentElement.style.colorScheme).toBe("dark");
    expect(parseTheme(name)).toBe(name);
  });
});
