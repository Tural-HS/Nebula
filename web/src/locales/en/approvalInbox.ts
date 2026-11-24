// English strings for the approval inbox, re-approval and deadlines (ADR-0034).
import type { Messages } from "../../i18n/locale";
import { plural } from "../../i18n/plural";

const requests = (n: number) => `${String(n)} ${plural("en", n, { one: "request", other: "requests" })}`;

export const tai = {
  nav: "Pending approvals",
  title: "Pending approvals",
  hint: "Pending approval requests from every project of the workspace you can see, oldest first.",
  tabs: { mine: "Waiting for my decision", all: "All pending" },
  empty: { mine: "Nothing is waiting for your decision.", all: "No approval requests are pending." },
  badge: (n: number) => `${requests(n)} waiting for your decision`,
  badgeOverdue: (n: number) => `${requests(n)} past the deadline`,
  requestedBy: (name: string) => `Requested by ${name}`,
  age: (days: number) => (days <= 0 ? "today" : `${String(days)} ${plural("en", days, { one: "day", other: "days" })} ago`),
  due: (date: string) => `Due ${date}`,
  overdue: (date: string) => `Overdue (due ${date})`,
  yourTurn: "Waiting for your decision",
  open: "Open",
  more: "Show more",
  invalidated: "The approved content changed afterwards: this approval no longer holds.",
  reopened: "Requested again because approved content changed.",
  notify: {
    overdue: "An approval request is past its deadline",
    invalidated: (actor: string) => `${actor} changed approved content; it needs approval again`,
  },
  act: {
    invalidated: "changed approved content: the approval no longer holds and the item went back for approval",
    overdue: "the approval deadline passed",
  },
  errors: {
    reapproval_required: "Changing approved content needs a new approval: edit this item on its own.",
  } as Record<string, string>,
  settings: {
    reapprove: "Changes after approval require new approval",
    reapproveHint: "When watched content changes, the item returns to the status it was approved from and a new request opens; whoever changed it cannot approve it.",
    watched: "Watched content",
    watchAll: "Everything: title, description and all fields",
    titleField: "Title",
    descriptionField: "Description",
    due: "Decision deadline, working days",
    dueHint: "Counted on the working-day calendar. Leave empty for no deadline.",
    dueRange: "Must be between 1 and 30.",
    summaryDue: (n: number) => `${String(n)} ${plural("en", n, { one: "working day", other: "working days" })}`,
    summaryReapprove: "re-approve on change",
  },
  reapproveWarn: (what: string) => `Note: changing ${what} sends this item back for approval.`,
  watchAllShort: "the title, description or fields",
} satisfies Messages["approvalInbox"];
