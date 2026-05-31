import "./App.css";

export type AdminRole = "viewer" | "moderator" | "place_admin" | "admin";

export type AdminRoute = "overview" | "moderation" | "places" | "audit";

export interface AdminSession {
  userId: string;
  displayName: string;
  roles: AdminRole[];
}

export interface AdminNavItem {
  route: AdminRoute;
  label: string;
  requiredRoles: AdminRole[];
}

export interface ModerationCase {
  id: string;
  targetType: "echo" | "reply" | "user" | "media";
  targetId: string;
  reporterId: string;
  reason: string;
  status: "pending_review" | "needs_context" | "action_ready";
  recommendedAction: "limit" | "remove" | "dismiss" | "escalate";
  preview: string;
}

export interface ModerationActionIntent {
  caseId: string;
  action: "limit" | "remove" | "dismiss" | "escalate";
  requiresSecondReview: boolean;
  auditLabel: string;
}

export interface AdminPlaceRecord {
  id: string;
  name: string;
  state: "active" | "pending_review" | "restricted";
  boundarySensitivity: "normal" | "sensitive";
  requestedOverride: boolean;
  reviewIntent: "approve" | "restrict" | "needs_boundary_review";
}

export const adminNavigation: AdminNavItem[] = [
  { route: "overview", label: "Overview", requiredRoles: ["viewer", "moderator", "place_admin", "admin"] },
  { route: "moderation", label: "Moderation", requiredRoles: ["moderator", "admin"] },
  { route: "places", label: "Places", requiredRoles: ["place_admin", "admin"] },
  { route: "audit", label: "Audit", requiredRoles: ["admin"] },
];

export const defaultAdminSession: AdminSession = {
  userId: "admin-fixture-1",
  displayName: "Moderator Fixture",
  roles: ["moderator", "place_admin"],
};

export const moderationCases: ModerationCase[] = [
  {
    id: "case-report-1",
    targetType: "echo",
    targetId: "echo-active-1",
    reporterId: "user-reporter-1",
    reason: "Safety concern",
    status: "pending_review",
    recommendedAction: "limit",
    preview: "Sensitive Echo preview hidden until moderator opens case.",
  },
  {
    id: "case-media-1",
    targetType: "media",
    targetId: "ast_0001",
    reporterId: "system-moderation",
    reason: "Media requires review",
    status: "needs_context",
    recommendedAction: "escalate",
    preview: "Media preview suppressed in queue.",
  },
];

export const adminPlaces: AdminPlaceRecord[] = [
  {
    id: "place-active-1",
    name: "City Museum",
    state: "active",
    boundarySensitivity: "normal",
    requestedOverride: false,
    reviewIntent: "approve",
  },
  {
    id: "place-restricted-1",
    name: "Private Club",
    state: "restricted",
    boundarySensitivity: "sensitive",
    requestedOverride: true,
    reviewIntent: "needs_boundary_review",
  },
];

export function canAccess(session: AdminSession, item: AdminNavItem): boolean {
  return item.requiredRoles.some((role) => session.roles.includes(role));
}

export function visibleNavigation(session: AdminSession): AdminNavItem[] {
  return adminNavigation.filter((item) => canAccess(session, item));
}

export function buildModerationIntent(item: ModerationCase): ModerationActionIntent {
  return {
    caseId: item.id,
    action: item.recommendedAction,
    requiresSecondReview: item.recommendedAction === "remove" || item.recommendedAction === "escalate",
    auditLabel: `${item.recommendedAction}:${item.targetType}:${item.targetId}`,
  };
}

export function placeReviewSummary(place: AdminPlaceRecord): string {
  if (place.boundarySensitivity === "sensitive" || place.requestedOverride) {
    return `${place.name}: boundary review required`;
  }
  return `${place.name}: ${place.reviewIntent}`;
}

export function App({ session = defaultAdminSession }: { session?: AdminSession }) {
  const visibleItems = visibleNavigation(session);
  const moderationAllowed = visibleItems.some((item) => item.route === "moderation");
  const placesAllowed = visibleItems.some((item) => item.route === "places");

  return (
    <main className="shell">
      <section className="card" aria-labelledby="admin-heading">
        <p className="eyebrow">Phase 4 admin shell</p>
        <h1 id="admin-heading">EchoTrails Admin</h1>
        <p className="summary">
          Authenticated admin shell with deterministic role guards, moderation queue placeholders, and Places review intents.
        </p>
        <p className="status">Non-mutating fixture workflow</p>
        <nav aria-label="Admin sections" className="nav-list">
          {visibleItems.map((item) => (
            <span className="pill" key={item.route}>{item.label}</span>
          ))}
        </nav>
        <section aria-labelledby="moderation-heading" className="panel">
          <h2 id="moderation-heading">Moderation queue</h2>
          {moderationAllowed ? (
            <ul>
              {moderationCases.map((item) => (
                <li key={item.id}>{item.reason} · {buildModerationIntent(item).auditLabel}</li>
              ))}
            </ul>
          ) : (
            <p className="locked">Role required: moderator or admin.</p>
          )}
        </section>
        <section aria-labelledby="places-heading" className="panel">
          <h2 id="places-heading">Places review</h2>
          {placesAllowed ? (
            <ul>
              {adminPlaces.map((place) => (
                <li key={place.id}>{placeReviewSummary(place)}</li>
              ))}
            </ul>
          ) : (
            <p className="locked">Role required: place admin or admin.</p>
          )}
        </section>
      </section>
    </main>
  );
}
