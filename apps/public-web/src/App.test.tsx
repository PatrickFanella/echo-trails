import { describe, expect, it } from "vitest";
import { renderToString } from "react-dom/server";
import { App } from "./App";

describe("Public web placeholder", () => {
  it("renders the public preview shell copy", () => {
    const html = renderToString(<App />);

    expect(html).toContain("place-stories");
    expect(html).toContain("public preview web");
  });
});
