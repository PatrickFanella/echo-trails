import { describe, expect, it, vi } from "vitest";
import { renderToString } from "react-dom/server";
import { App, adminMutationPath, mutationHeaders, safePreview } from "./App";

describe("EchoTrails admin client", () => {
  it("renders a secure role-gated login instead of fixture data", () => {
    const html = renderToString(<App />);
    expect(html).toContain("Moderator access token");
    expect(html).toContain("Open secure session");
    expect(html).not.toContain("Non-mutating fixture workflow");
  });

  it("attaches CSRF and idempotency protection to mutations", () => {
    vi.stubGlobal("crypto", { randomUUID: () => "request-1" });
    expect(mutationHeaders({ role: "admin", csrfToken: "csrf", expiresAt: "later" })).toMatchObject({
      "X-CSRF-Token": "csrf",
      "Idempotency-Key": "request-1",
    });
  });

  it("does not expose media in queue summaries", () => {
    expect(safePreview({ id: "review", kind: "media_review", status: "needs_review", created_at: "2026-01-01" }))
      .toContain("requires an explicit signed-preview action");
  });

  it("uses generated operation routes for privileged queue mutations", () => {
    expect(adminMutationPath("media", "asset/unsafe")).toBe("/admin/media/asset%2Funsafe");
    expect(adminMutationPath("places", "place-1")).toBe("/admin/places/place-1");
    expect(adminMutationPath("appeals", "appeal-1")).toBe("/admin/appeals/appeal-1");
    expect(adminMutationPath("jobs", "job-1")).toBe("/admin/jobs/job-1");
  });
});
