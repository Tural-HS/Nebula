import { describe, expect, it } from "vitest";

import type { AppNotification } from "../api/notifications";
import { tn } from "../i18n-notifications";
import { badgeText, notificationHref, notificationText } from "./notifications";

const base: AppNotification = {
  id: 1,
  kind: "mention",
  read: false,
  created_at: "2026-11-23T12:00:00Z",
  workspace_slug: "acme",
  workspace_name: "Acme",
  project_key: "QA",
  item_number: 12,
  item_key: "QA-12",
  item_title: "Giriş xətası",
  actor_id: "u1",
  actor_name: "Alice",
};

describe("notificationText", () => {
  it.each([
    [{ kind: "mention" }, "Alice sizi qeyd etdi: QA-12 Giriş xətası"],
    [{ kind: "assigned" }, "Alice tərəfindən sizə təyin edildi: QA-12 Giriş xətası"],
    [{ kind: "assigned", actor_id: null, actor_name: null }, "Sizə təyin edildi: QA-12 Giriş xətası"],
    [{ kind: "overdue", actor_id: null, actor_name: null }, "Son tarixi keçdi: QA-12 Giriş xətası"],
    [{ kind: "comment" }, "Alice şərh yazdı: QA-12 Giriş xətası"],
    [{ kind: "comment", actor_name: null }, "Bir istifadəçi şərh yazdı: QA-12 Giriş xətası"],
    [{ kind: "approval_requested" }, "Alice təsdiqinizi gözləyir: QA-12 Giriş xətası"],
    [{ kind: "approval_applied" }, "Təsdiq sorğunuz qəbul edildi: QA-12 Giriş xətası"],
    [{ kind: "approval_rejected" }, "Alice təsdiq sorğunuzu rədd etdi: QA-12 Giriş xətası"],
    [{ kind: "approval_canceled" }, "Alice təsdiq sorğunuzu ləğv etdi: QA-12 Giriş xətası"],
    // Watcher kinds (ADR-0028)
    [{ kind: "status_changed" }, "Alice statusu dəyişdi: QA-12 Giriş xətası"],
    [{ kind: "assignee_changed" }, "Alice icraçını dəyişdi: QA-12 Giriş xətası"],
    [{ kind: "due_changed" }, "Alice son tarixi dəyişdi: QA-12 Giriş xətası"],
    [{ kind: "link_added", actor_name: null }, "Bir istifadəçi əlaqə əlavə etdi: QA-12 Giriş xətası"],
  ] as const)("%o", (patch, want) => {
    expect(notificationText({ ...base, ...patch })).toBe(want);
  });
});

describe("notificationHref", () => {
  it("deep-links to the item drawer and encodes segments", () => {
    expect(notificationHref(base)).toBe("/w/acme/p/QA?item=12");
    expect(notificationHref({ ...base, workspace_slug: "a/b" })).toBe("/w/a%2Fb/p/QA?item=12");
  });
});

describe("badge", () => {
  it.each([
    [0, false, null],
    [1, false, "1"],
    [99, false, "99"],
    [100, false, "99+"],
    [100, true, "99+"],
  ] as const)("%i capped=%s", (count, capped, want) => {
    expect(badgeText(count, capped)).toBe(want);
  });
  it("labels the button with the count", () => {
    expect(tn.bellLabel(0, false)).toBe("Bildirişlər");
    expect(tn.bellLabel(3, false)).toBe("Bildirişlər, 3 oxunmamış");
    expect(tn.bellLabel(100, true)).toBe("Bildirişlər, 99-dan çox oxunmamış");
  });
});
