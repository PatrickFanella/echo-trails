import { describe, expect, it } from "vitest";
import { renderToString } from "react-dom/server";
import { App, adminPlaces, buildModerationIntent, canAccess, moderationCases, placeReviewSummary, visibleNavigation } from "./App";

describe("Admin web Phase 4 shell", () => {
  it("renders guarded moderation and Places workflow sections", () => {
    const html = renderToString(<App />);

    expect(html).toContain("EchoTrails Admin");
    expect(html).toContain("Moderation queue");
    expect(html).toContain("Places review");
    expect(html).toContain("Non-mutating fixture workflow");
  });

  it("guards navigation by role", () => {
    const viewer = { userId: "viewer", displayName: "Viewer", roles: ["viewer" as const] };
    const nav = visibleNavigation(viewer).map((item) => item.route);

    expect(nav).toEqual(["overview"]);
    expect(canAccess(viewer, { route: "audit", label: "Audit", requiredRoles: ["admin"] })).toBe(false);
  });

  it("guards protected panels, not just navigation", () => {
    const viewer = { userId: "viewer", displayName: "Viewer", roles: ["viewer" as const] };
    const admin = { userId: "admin", displayName: "Admin", roles: ["admin" as const] };

    expect(renderToString(<App session={viewer} />)).toContain("Role required: moderator or admin.");
    expect(renderToString(<App session={viewer} />)).toContain("Role required: place admin or admin.");
    expect(visibleNavigation(admin).map((item) => item.route)).toEqual(["overview", "moderation", "places", "audit"]);
  });

  it("creates moderation action intents without mutating cases", () => {
    const [first, second] = moderationCases;

    expect(buildModerationIntent(first)).toEqual({
      caseId: "case-report-1",
      action: "limit",
      requiresSecondReview: false,
      auditLabel: "limit:echo:echo-active-1",
    });
    expect(buildModerationIntent(second).requiresSecondReview).toBe(true);
  });

  it("flags sensitive Places for boundary review", () => {
    expect(adminPlaces).toHaveLength(2);
    expect(placeReviewSummary(adminPlaces[1])).toContain("boundary review required");
  });
});
