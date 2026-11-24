// English strings for workspace kinds: team templates (ADR-0033).
import type { Messages } from "../../i18n/locale";

export const tk = {
  kind: "Team type",
  kindHint: "Everything can be changed in Settings later.",
  names: {
    general: "General",
    finance: "Finance",
    it: "IT",
    hr: "HR",
  },
  descriptions: {
    general: "Statuses and item types only (Task, Bug). You set up the rest.",
    finance: "Payment, invoice, report; Amount, VAT and Counterparty fields; a second admin approves \"Paid\"; a \"Finance\" project with a \"Payment request\" form.",
    it: "Bug, task, request; Priority and Component fields; \"My work\" and \"Critical\" views and an IT access request form.",
    hr: "Onboarding and leave; a private \"HR\" project with leave request and new hire forms.",
  },
  calendarNote: "An Azerbaijani workspace also gets this year's official working-day calendar.",
  approvalNote: "Moving to \"Paid\" needs the approval of another admin or owner: invite at least one more admin.",
} satisfies Messages["kinds"];
