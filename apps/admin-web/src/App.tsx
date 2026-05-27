import "./App.css";

export function App() {
  return (
    <main className="shell">
      <section className="card" aria-labelledby="admin-heading">
        <p className="eyebrow">Admin placeholder</p>
        <h1 id="admin-heading">EchoTrails Admin</h1>
        <p className="summary">
          A scoped React/Vite shell for future moderation and operations workflows:
          users, Places, Echo review, appeals, media jobs, and audit trails.
        </p>
        <p className="status">Phase 0 scaffold only</p>
      </section>
    </main>
  );
}
