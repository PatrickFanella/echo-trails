import { describe, expect, it } from "vitest";
import { renderToString } from "react-dom/server";
import { App, policyPages, publicBrowsingDisabledNotice, routePath, sharePreview } from "./App";

describe("Public web beta readiness surface", () => {
  it("renders share preview and TestFlight CTA", () => {
    const html = renderToString(<App />);

    expect(html).toContain("place-stories");
    expect(html).toContain("Request TestFlight invite");
    expect(html).toContain(sharePreview.title);
  });

  it("exposes required policy routes with disclosure text", () => {
    expect(policyPages.map((page) => page.route)).toEqual(["terms", "privacy", "disclosure"]);
    expect(routePath("terms")).toBe("/terms");
    expect(routePath("privacy")).toBe("/privacy");
    expect(routePath("disclosure")).toBe("/disclosure");
    expect(policyPages.every((page) => page.requiresLegalReview)).toBe(true);
  });

  it("does not claim full public browsing", () => {
    expect(publicBrowsingDisabledNotice()).toContain("Full public browsing is not available");
  });
});
