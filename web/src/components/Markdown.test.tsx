// @vitest-environment jsdom
import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { t } from "../i18n";
import { safeUrl } from "../lib/markdown";
import { Markdown } from "./Markdown";

afterEach(cleanup);

const ALICE = "0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b";

function html(source: string): HTMLElement {
  return render(<Markdown source={source} people={[{ id: ALICE, name: "Alice" }]} />).container;
}

// Classic XSS payloads (OWASP cheat sheet, Markdown-specific vectors). None
// may yield a script, an event handler, a dangerous URL or a loaded resource.
const VECTORS = [
  "<script>alert(1)</script>",
  "<img src=x onerror=alert(1)>",
  "<svg/onload=alert(1)>",
  "<iframe src=\"javascript:alert(1)\"></iframe>",
  "<a href=\"javascript:alert(1)\">x</a>",
  "<details open ontoggle=alert(1)>",
  "<style>body{display:none}</style>",
  "<form action=\"https://evil.test\"><button>x</button></form>",
  "[click](javascript:alert(1))",
  "[click](JaVaScRiPt:alert(1))",
  "[click](java&#x09;script:alert(1))",
  "[click](&#106;avascript:alert(1))",
  "[click](javascript&#58;alert(1))",
  "[click]( javascript:alert(1))",
  "[click](vbscript:msgbox(1))",
  "[click](data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==)",
  "[click](file:///etc/passwd)",
  "[click](//evil.test/x)",
  "[a](<javascript:alert(1)>)",
  "[ref]\n\n[ref]: javascript:alert(1)",
  "<javascript:alert(1)>",
  "![x](https://evil.test/pixel.gif)",
  "![x](javascript:alert(1))",
  "![x\"onerror=\"alert(1)](x)",
  "[x](https://ok.test \"title\" onmouseover=alert(1))",
  "`<script>alert(1)</script>`",
  "```html\n<script>alert(1)</script>\n```",
  "<!-- <script>alert(1)</script> -->",
  "<<script>script>alert(1)<</script>/script>",
  "| a |\n|---|\n| <img src=x onerror=alert(1)> |",
  "- [x] <b onclick=alert(1)>done</b>",
];

describe("Markdown sanitization", () => {
  for (const v of VECTORS) {
    it(`neutralizes ${JSON.stringify(v).slice(0, 60)}`, () => {
      const root = html(v);
      expect(root.querySelector("script, iframe, svg, style, form, img, object, embed, details, button, b")).toBeNull();
      for (const el of root.querySelectorAll("*")) {
        for (const attr of el.attributes) {
          expect(attr.name.startsWith("on"), `${el.tagName} ${attr.name}`).toBe(false);
          expect(attr.name).not.toBe("style");
        }
      }
      for (const a of root.querySelectorAll("a")) {
        const href = a.getAttribute("href") ?? "";
        expect(href === "" || /^(https?:|mailto:|#|\/(?![/\\]))/i.test(href), href).toBe(true);
      }
    });
  }

  it("drops raw HTML but keeps the text around it", () => {
    const root = html("before <b>bold</b> after");
    expect(root.querySelector("b")).toBeNull();
    expect(root.textContent).toContain("before");
    expect(root.textContent).toContain("after");
  });

  it("renders external images as links, never as <img>", () => {
    const root = html("![diagram](https://example.test/d.png)");
    expect(root.querySelector("img")).toBeNull();
    const a = root.querySelector("a");
    expect(a?.getAttribute("href")).toBe("https://example.test/d.png");
    expect(a?.textContent).toContain("diagram");
  });
});

describe("Markdown rendering", () => {
  it("renders the GFM subset", () => {
    const root = html("# Title\n\n## Sub\n\n- one\n- **two**\n\n1. first\n\n`code` and ~~gone~~\n\n```\nblock\n```\n\n> quote\n\n| a | b |\n|---|---|\n| 1 | 2 |");
    expect(root.querySelector("h1")?.textContent).toBe("Title");
    expect(root.querySelector("h2")).not.toBeNull();
    expect(root.querySelectorAll("ul li")).toHaveLength(2);
    expect(root.querySelector("ol li")?.textContent).toBe("first");
    expect(root.querySelector("strong")?.textContent).toBe("two");
    expect(root.querySelector("p code")?.textContent).toBe("code");
    expect(root.querySelector("del")?.textContent).toBe("gone");
    expect(root.querySelector("pre code")?.textContent).toBe("block\n");
    expect(root.querySelector("blockquote")).not.toBeNull();
    expect(root.querySelectorAll("table td")).toHaveLength(2);
  });

  it("shows task lists read-only", () => {
    const boxes = html("- [x] done\n- [ ] todo").querySelectorAll<HTMLInputElement>("input[type=checkbox]");
    expect(boxes).toHaveLength(2);
    expect(boxes[0]?.checked).toBe(true);
    expect(boxes[1]?.checked).toBe(false);
    for (const b of boxes) expect(b.disabled).toBe(true);
  });

  it("opens links in a new tab without referrer or opener", () => {
    const a = html("[docs](https://example.test/docs) and https://auto.example.test").querySelectorAll("a");
    expect(a).toHaveLength(2);
    for (const link of a) {
      expect(link.getAttribute("target")).toBe("_blank");
      expect(link.getAttribute("rel")).toBe("noopener noreferrer nofollow");
    }
  });

  it("renders mentions in prose, keeps them literal in code", () => {
    const root = html(`Hi <@${ALICE}>, see \`<@${ALICE}>\` and <@00000000-0000-7000-8000-000000000000>`);
    const chips = [...root.querySelectorAll("span")].filter((s) => s.textContent.startsWith("@"));
    expect(chips.map((s) => s.textContent)).toEqual(["@Alice", "@" + t.unknownPerson]);
    expect(root.querySelector("code")?.textContent).toBe(`<@${ALICE}>`);
    expect(root.innerHTML).not.toContain("data-mention=\"<");
  });

  it("keeps mentions working inside formatting", () => {
    const root = html(`**<@${ALICE}>** _and_ - list`);
    expect(root.querySelector("strong span")?.textContent).toBe("@Alice");
  });
});

describe("safeUrl", () => {
  it.each([
    ["https://example.test/x", "https://example.test/x"],
    ["http://example.test", "http://example.test"],
    ["mailto:a@example.test", "mailto:a@example.test"],
    ["/w/acme/p/QA", "/w/acme/p/QA"],
    ["#section", "#section"],
    ["javascript:alert(1)", ""],
    ["  JAVASCRIPT:alert(1)", ""],
    ["data:text/html,x", ""],
    ["//evil.test", ""],
    ["/\\evil.test", ""],
    ["relative/path", ""],
    ["ftp://example.test", ""],
  ])("%s -> %s", (input, want) => {
    expect(safeUrl(input)).toBe(want);
  });
});
