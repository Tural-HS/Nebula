// Markdown for item descriptions and comments (ADR-0022). react-markdown
// builds React elements from a syntax tree: there is no innerHTML anywhere,
// raw HTML in the source is dropped, only an allow-listed set of elements is
// rendered and every URL goes through safeUrl. Images are never loaded (no
// tracking pixels, no requests to other hosts); they render as links.
import type { ReactNode } from "react";
import ReactMarkdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";

import { ta } from "../i18n-attach";
import { t } from "../i18n";
import { mentionPlugin, safeUrl } from "../lib/markdown";
import type { Person } from "../lib/mentions";

const ALLOWED = [
  "p", "br", "strong", "em", "del", "a", "code", "pre", "blockquote", "hr",
  "h1", "h2", "h3", "h4", "h5", "h6", "ul", "ol", "li", "input",
  "table", "thead", "tbody", "tr", "th", "td", "span", "img", "section", "sup",
];

function components(names: Map<string, string>): Components {
  return {
    a: ({ href, children }) =>
      !href ? (
        <span>{children}</span>
      ) : href.startsWith("#") ? (
        <a href={href} className="text-brand-700 underline dark:text-brand-100">{children}</a>
      ) : (
        <a href={href} target="_blank" rel="noopener noreferrer nofollow" className="text-brand-700 underline dark:text-brand-100">
          {children}
        </a>
      ),
    // Never fetch images: show the alt text as a link the reader may open.
    img: ({ src, alt }) => {
      const label = `🖼 ${alt || ta.image}`;
      const href = typeof src === "string" ? safeUrl(src) : "";
      return href ? (
        <a href={href} target="_blank" rel="noopener noreferrer nofollow" className="text-brand-700 underline dark:text-brand-100">
          {label}
        </a>
      ) : (
        <span>{label}</span>
      );
    },
    // Task list checkboxes are read-only.
    input: ({ type, checked }) =>
      type === "checkbox" ? <input type="checkbox" checked={Boolean(checked)} disabled readOnly className="mr-1.5 align-middle" /> : null,
    span: ({ node, children }) => {
      const id = node?.properties.dataMention;
      if (typeof id === "string") {
        return (
          <span className="rounded bg-brand-50 px-1 font-medium text-brand-700 dark:bg-brand-700/30 dark:text-brand-100">
            @{names.get(id) ?? t.unknownPerson}
          </span>
        );
      }
      return <span>{children}</span>;
    },
    table: ({ children }) => (
      <div className="overflow-x-auto">
        <table>{children}</table>
      </div>
    ),
  };
}

/** Renders stored Markdown (with <@uuid> mention tokens) safely. */
export function Markdown({ source, people = [], className }: { source: string; people?: readonly Person[]; className?: string }): ReactNode {
  const names = new Map(people.map((p) => [p.id.toLowerCase(), p.name]));
  return (
    <div className={"nebula-md text-sm break-words " + (className ?? "")}>
      <ReactMarkdown
        remarkPlugins={[remarkGfm, mentionPlugin]}
        skipHtml
        allowedElements={ALLOWED}
        unwrapDisallowed
        urlTransform={safeUrl}
        components={components(names)}
      >
        {source}
      </ReactMarkdown>
    </div>
  );
}
