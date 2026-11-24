import { describe, expect, it } from "vitest";

import { ApiError } from "../api/client";
import { approvalErrorCode, approvalErrorText, isLocked, policyFor, policyProblem, progress } from "./approvals";

describe("approvals", () => {
  it("locks everything an approver signs off on while a request is pending", () => {
    const pending = { approval: { id: "r", to_status: { id: "s", name: "Done", category: "done" as const }, requested_by: "u", approvals: 0, min_approvals: 1 } };
    expect(isLocked(pending)).toBe(true);
    expect(isLocked({ approval: null })).toBe(false);
    expect(isLocked({})).toBe(false);
  });

  it("finds the policy of a status", () => {
    const p = { id: "p", status_id: "done", min_approvals: 1, approver_roles: [], approver_user_ids: ["u"] };
    expect(policyFor([p], "done")).toBe(p);
    expect(policyFor([p], "todo")).toBeUndefined();
    expect(policyFor(undefined, "done")).toBeUndefined();
  });

  it("explains policies the server would refuse", () => {
    expect(policyProblem({ min_approvals: 0, approver_roles: ["member"], approver_user_ids: [] })).toBe("range");
    expect(policyProblem({ min_approvals: 6, approver_roles: ["member"], approver_user_ids: [] })).toBe("range");
    expect(policyProblem({ min_approvals: 1, approver_roles: [], approver_user_ids: [] })).toBe("needApprovers");
    expect(policyProblem({ min_approvals: 2, approver_roles: [], approver_user_ids: ["a", "a"] })).toBe("tooFew");
    expect(policyProblem({ min_approvals: 2, approver_roles: [], approver_user_ids: ["a", "b"] })).toBeNull();
    expect(policyProblem({ min_approvals: 5, approver_roles: ["admin"], approver_user_ids: [] })).toBeNull();
  });

  it("clamps progress", () => {
    expect(progress(1, 2)).toEqual({ done: 1, min: 2, ratio: 0.5 });
    expect(progress(3, 2).done).toBe(2);
    expect(progress(-1, 2).done).toBe(0);
  });

  it("recognizes approval error codes", () => {
    expect(approvalErrorCode(new ApiError(409, "approval_pending", "x"))).toBe("approval_pending");
    expect(approvalErrorCode(new ApiError(403, "self_approval", "x"))).toBe("self_approval");
    expect(approvalErrorCode(new ApiError(409, "version_conflict", "x"))).toBeNull();
    expect(approvalErrorCode(new Error("x"))).toBeNull();
    expect(approvalErrorText(new ApiError(403, "self_approval", "raw"))).toBe("Öz sorğunuzu təsdiqləyə və ya rədd edə bilməzsiniz.");
    expect(approvalErrorText(new ApiError(422, "validation_failed", "raw"))).toBe("raw");
  });
});
