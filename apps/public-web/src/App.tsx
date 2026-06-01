import "./App.css";

export type PublicRoute = "share-preview" | "testflight" | "terms" | "privacy" | "disclosure";

export interface PolicyPage {
  route: PublicRoute;
  title: string;
  summary: string;
  requiresLegalReview: boolean;
}

export const policyPages: PolicyPage[] = [
  {
    route: "terms",
    title: "Terms of Use",
    summary: "Template beta terms for place-story publishing, moderation, and account rules. Lawyer review required before public launch.",
    requiresLegalReview: true,
  },
  {
    route: "privacy",
    title: "Privacy Policy",
    summary: "Template privacy notice covering location proof, public coordinates, account data, media, analytics opt-out, export, and deletion requests.",
    requiresLegalReview: true,
  },
  {
    route: "disclosure",
    title: "Hosting and Safety Disclosure",
    summary: "EchoTrails is a beta place-stories service. Self-hosting, moderation, backup, and uptime guarantees are disclosed before invite access.",
    requiresLegalReview: true,
  },
];

export const sharePreview = {
  title: "A story is waiting at City Museum",
  description: "EchoTrails lets people discover short Echoes tied to real Places.",
  canonicalPath: "/places/place-active-1",
};

export function routePath(route: PublicRoute): string {
  switch (route) {
    case "share-preview":
      return sharePreview.canonicalPath;
    case "testflight":
      return "/beta";
    case "terms":
      return "/terms";
    case "privacy":
      return "/privacy";
    case "disclosure":
      return "/disclosure";
  }
}

export function publicBrowsingDisabledNotice(): string {
  return "Full public browsing is not available during beta; use an invite or TestFlight link.";
}

export function App() {
  return (
    <main className="landing">
      <section className="hero" aria-labelledby="public-heading">
        <p className="kicker">Beta public preview</p>
        <h1 id="public-heading">EchoTrails place-stories</h1>
        <p className="lede">
          Share previews, legal pages, and a TestFlight call to action for people discovering Echoes tied to real Places.
        </p>
        <a className="cta" href={routePath("testflight")}>Request TestFlight invite</a>
        <p className="badge">{publicBrowsingDisabledNotice()}</p>
        <section className="preview" aria-labelledby="share-preview-heading">
          <p className="kicker">Share preview</p>
          <h2 id="share-preview-heading">{sharePreview.title}</h2>
          <p>{sharePreview.description}</p>
        </section>
        <section className="policy-grid" aria-label="Policy pages">
          {policyPages.map((page) => (
            <article className="policy-card" key={page.route}>
              <h2>{page.title}</h2>
              <p>{page.summary}</p>
              <p className="review-note">Lawyer review required</p>
            </article>
          ))}
        </section>
      </section>
    </main>
  );
}
