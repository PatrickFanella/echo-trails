import { FormEvent, useCallback, useEffect, useState } from "react";
import { openApiOperations } from "./generated/openapi-paths";
import "./App.css";

export type AdminRole = "moderator" | "admin";
export type AdminQueue = "reports" | "media" | "places" | "jobs" | "appeals" | "audit";

export interface AdminSession {
  role: AdminRole;
  csrfToken: string;
  expiresAt: string;
}

export interface AdminQueueItem {
  id: string;
  kind: string;
  status: string;
  reason?: string;
  target_type?: string;
  target_id?: string;
  created_at: string;
  metadata?: Record<string, unknown>;
}

interface QueuePage {
  items: AdminQueueItem[];
  next_cursor?: string;
}

export const queueLabels: Record<AdminQueue, string> = {
  reports: "Reports",
  media: "Media review",
  places: "Places",
  jobs: "Jobs",
  appeals: "Appeals",
  audit: "Audit history",
};

export function mutationHeaders(session: AdminSession, idempotencyKey = crypto.randomUUID()): HeadersInit {
  return { "Content-Type": "application/json", "X-CSRF-Token": session.csrfToken, "Idempotency-Key": idempotencyKey };
}

export function safePreview(item: AdminQueueItem): string {
  if (item.kind === "media_review") return "Private media preview requires an explicit signed-preview action.";
  return `${item.target_type || item.kind} · ${item.target_id || item.id}`;
}

export function adminMutationPath(queue: AdminQueue, targetId: string): string {
  switch (queue) {
    case "media": return openApiOperations.reviewAdminSafetyItem(targetId);
    case "places": return openApiOperations.reviewAdminPlace(targetId);
    case "jobs": return openApiOperations.manageAdminJob(targetId);
    case "appeals": return openApiOperations.resolveAdminAppeal(targetId);
    default: throw new Error(`${queue} does not support item mutations`);
  }
}

const apiBase = (import.meta.env.VITE_ECHOTRAILS_API_URL as string | undefined)?.replace(/\/$/, "") ?? "";

async function api(path: string, init: RequestInit = {}): Promise<Response> {
  return fetch(`${apiBase}${path}`, { ...init, credentials: "include" });
}

export function App() {
  const [session, setSession] = useState<AdminSession | null>(null);
  const [accessToken, setAccessToken] = useState("");
  const [queue, setQueue] = useState<AdminQueue>("reports");
  const [items, setItems] = useState<AdminQueueItem[]>([]);
  const [nextCursor, setNextCursor] = useState<string>();
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("Sign in with a moderator or administrator account.");
  const [inviteCode, setInviteCode] = useState("");
  const [inviteUses, setInviteUses] = useState(1);

  const loadQueue = useCallback(async (selected: AdminQueue, cursor?: string) => {
    if (!session) return;
    setBusy(true);
    try {
      const response = await api(`${openApiOperations.listAdminQueue(selected)}${cursor ? `?cursor=${encodeURIComponent(cursor)}` : ""}`);
      if (!response.ok) throw new Error(`Queue request failed (${response.status})`);
      const page = (await response.json()) as QueuePage;
      setItems((current) => cursor ? [...current, ...page.items] : page.items);
      setNextCursor(page.next_cursor);
      setMessage(`${page.items.length} ${queueLabels[selected].toLowerCase()} loaded.`);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Queue request failed.");
    } finally {
      setBusy(false);
    }
  }, [session]);

  useEffect(() => { void loadQueue(queue); }, [queue, loadQueue]);

  async function signIn(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    try {
      const response = await api(openApiOperations.createAdminSession, { method: "POST", headers: { Authorization: `Bearer ${accessToken.trim()}` } });
      if (!response.ok) throw new Error("This account does not have active moderator access.");
      const body = await response.json() as { role: AdminRole; csrf_token: string; expires_at: string };
      setSession({ role: body.role, csrfToken: body.csrf_token, expiresAt: body.expires_at });
      setAccessToken("");
      setMessage("Secure admin session opened.");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Sign-in failed.");
    } finally {
      setBusy(false);
    }
  }

  async function signOut() {
    if (!session) return;
    await api(openApiOperations.deleteAdminSession, { method: "DELETE", headers: mutationHeaders(session) });
    setSession(null);
    setItems([]);
    setMessage("Admin session closed.");
  }

  async function act(item: AdminQueueItem, action: string, extra: Record<string, unknown> = {}) {
    if (!session) return;
    const reasonCode = window.prompt("Reason code (required and audited)");
    if (!reasonCode || !window.confirm(`Confirm ${action} for ${safePreview(item)}?`)) return;
    const path = item.kind === "report" ? openApiOperations.applyModerationAction : adminMutationPath(queue, item.id);
    const method = item.kind === "report" ? "POST" : "PATCH";
    const body = item.kind === "report"
      ? { action, reason_code: reasonCode, target_type: item.target_type, target_id: item.target_id, report_id: item.id, confirmed: true }
      : { action, reason_code: reasonCode, confirmed: true, ...extra };
    setBusy(true);
    const response = await api(path, { method, headers: mutationHeaders(session), body: JSON.stringify(body) });
    setBusy(false);
    if (!response.ok) {
      setMessage(`Action failed (${response.status}).`);
      return;
    }
    setMessage(`${action} recorded in the audit history.`);
    await loadQueue(queue);
  }

  async function mergePlace(item: AdminQueueItem) {
    const destination = window.prompt("Canonical destination Place UUID");
    if (!destination) return;
    await act(item, "merge", { merge_into_place_id: destination.trim() });
  }

  async function editBoundary(item: AdminQueueItem) {
    const raw = window.prompt("Paste reviewed Polygon or MultiPolygon GeoJSON");
    if (!raw) return;
    let boundary: unknown;
    try { boundary = JSON.parse(raw); } catch { setMessage("Boundary must be valid GeoJSON."); return; }
    const radius = Number(window.prompt("Access buffer in meters (25–500)", "25"));
    if (!Number.isFinite(radius) || radius < 25 || radius > 500) { setMessage("Boundary buffer must be between 25 and 500 meters."); return; }
    await act(item, "set_boundary", { boundary_geojson: boundary, radius_meters: radius });
  }

  async function setPointRadius(item: AdminQueueItem) {
    const radius = Number(window.prompt("Reviewed point radius in meters (25–500)", "100"));
    if (!Number.isFinite(radius) || radius < 25 || radius > 500) { setMessage("Radius must be between 25 and 500 meters."); return; }
    await act(item, "set_radius", { radius_meters: radius });
  }

  async function createInvite(event: FormEvent) {
    event.preventDefault();
    if (!session || session.role !== "admin" || !inviteCode.trim()) return;
    const reasonCode = window.prompt("Reason code (required and audited)");
    if (!reasonCode || !window.confirm(`Create invite with ${inviteUses} use(s)?`)) return;
    setBusy(true);
    const response = await api(openApiOperations.createAdminInvite, {
      method: "POST", headers: mutationHeaders(session),
      body: JSON.stringify({ code: inviteCode.trim(), max_uses: inviteUses, reason_code: reasonCode, confirmed: true }),
    });
    setBusy(false);
    if (!response.ok) { setMessage(`Invite creation failed (${response.status}).`); return; }
    setInviteCode(""); setInviteUses(1); setMessage("Invite created and recorded in audit history.");
  }

  if (!session) {
    return <main className="shell"><section className="card" aria-labelledby="admin-heading">
      <p className="eyebrow">Invite-gated operations</p><h1 id="admin-heading">EchoTrails Admin</h1>
      <p className="summary">Moderation, Place review, appeals, delivery jobs, and immutable audit history.</p>
      <form className="login" onSubmit={signIn}>
        <label htmlFor="access-token">Moderator access token</label>
        <input id="access-token" type="password" autoComplete="off" required value={accessToken} onChange={(event) => setAccessToken(event.target.value)} />
        <button disabled={busy}>Open secure session</button>
      </form><p className="status" role="status">{message}</p>
    </section></main>;
  }

  return <main className="shell"><section className="card wide" aria-labelledby="admin-heading">
    <header className="toolbar"><div><p className="eyebrow">{session.role} console</p><h1 id="admin-heading">EchoTrails Admin</h1></div><button onClick={signOut}>Sign out</button></header>
    <p className="status" role="status" aria-live="polite">{message}</p>
    <nav aria-label="Admin queues" className="nav-list">
      {(Object.keys(queueLabels) as AdminQueue[]).map((entry) => <button className={queue === entry ? "pill selected" : "pill"} aria-current={queue === entry ? "page" : undefined} onClick={() => setQueue(entry)} key={entry}>{queueLabels[entry]}</button>)}
    </nav>
    {session.role === "admin" && <form className="invite-form" onSubmit={createInvite} aria-labelledby="invite-heading">
      <h2 id="invite-heading">Create beta invite</h2>
      <label htmlFor="invite-code">Invite code</label>
      <input id="invite-code" required value={inviteCode} onChange={(event) => setInviteCode(event.target.value)} />
      <label htmlFor="invite-uses">Maximum uses</label>
      <input id="invite-uses" type="number" min="1" max="1000" required value={inviteUses} onChange={(event) => setInviteUses(Number(event.target.value))} />
      <button disabled={busy || !inviteCode.trim()}>Create audited invite</button>
    </form>}
    <section className="panel" aria-labelledby="queue-heading"><h2 id="queue-heading">{queueLabels[queue]}</h2>
      {items.length === 0 && !busy ? <p>No items in this queue.</p> : <ul className="queue-list">{items.map((item) => <li key={item.id}>
        <div><strong>{item.reason || item.status}</strong><span>{safePreview(item)}</span><small>{new Date(item.created_at).toLocaleString()}</small></div>
        {queue === "reports" && item.target_type === "user" && <div className="actions"><button onClick={() => act(item, "warn")}>Warn</button><button onClick={() => act(item, "restrict_posting")}>Restrict posting</button><button className="danger" onClick={() => act(item, "suspend")}>Suspend</button><button className="danger" onClick={() => act(item, "ban")}>Ban</button><button onClick={() => act(item, "restore")}>Restore</button></div>}
        {queue === "reports" && (item.target_type === "echo" || item.target_type === "reply") && <div className="actions"><button onClick={() => act(item, "limit")}>Limit</button><button className="danger" onClick={() => act(item, "remove")}>Remove</button>{item.target_type === "echo" && <button onClick={() => act(item, "sensitive_label")}>Sensitive label</button>}<button onClick={() => act(item, "restore")}>Dismiss / restore</button></div>}
        {queue === "reports" && item.target_type === "place" && <div className="actions"><button className="danger" onClick={() => act(item, "place_restrict")}>Restrict Place</button></div>}
        {queue === "media" && <div className="actions">
          {item.kind === "media_review" && <button onClick={() => window.open(`${apiBase}${openApiOperations.getAdminMediaPreview(item.id)}`, "_blank", "noopener,noreferrer")}>Open signed private preview</button>}
          <button onClick={() => act(item, "approve")}>Approve</button>
          <button className="danger" onClick={() => act(item, "reject")}>Reject / limit</button>
        </div>}
        {queue === "places" && session.role === "admin" && <div className="actions"><button onClick={() => act(item, "approve")}>Approve</button><button onClick={() => act(item, "restrict")}>Restrict</button><button onClick={() => mergePlace(item)}>Merge</button><button onClick={() => editBoundary(item)}>Set boundary</button><button onClick={() => setPointRadius(item)}>Use point radius</button></div>}
        {queue === "appeals" && <div className="actions"><button onClick={() => act(item, "accept")}>Accept</button><button onClick={() => act(item, "reject")}>Reject</button></div>}
        {queue === "jobs" && session.role === "admin" && <div className="actions"><button onClick={() => act(item, "retry")}>Retry</button><button onClick={() => act(item, "cancel")}>Cancel</button></div>}
      </li>)}</ul>}
      {nextCursor && <button disabled={busy} onClick={() => loadQueue(queue, nextCursor)}>Load more</button>}
    </section>
  </section></main>;
}
