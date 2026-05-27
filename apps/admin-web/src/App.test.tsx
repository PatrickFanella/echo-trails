import { describe, expect, it } from "vitest";
import { renderToString } from "react-dom/server";
import { App } from "./App";

describe("Admin web placeholder", () => {
  it("renders the moderation shell copy", () => {
    const html = renderToString(<App />);

    expect(html).toContain("EchoTrails Admin");
    expect(html).toContain("moderation and operations");
  });
});
