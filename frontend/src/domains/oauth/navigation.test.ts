import { describe, it, expect } from "vitest";
import { isNavigable } from "./navigation";

describe("isNavigable", () => {
  it("https と http の戻り先は、開いてよい", () => {
    expect(isNavigable("https://app.example.com/callback?code=abc&state=x")).toBe(true);
    expect(isNavigable("http://127.0.0.1:53000/callback?code=abc")).toBe(true);
  });

  it("javascript: や data: など、通信ではないスキームは、ページの中でコードが動きうるので開かない", () => {
    expect(isNavigable("javascript:alert(1)")).toBe(false);
    expect(isNavigable("data:text/html,<script>alert(1)</script>")).toBe(false);
    expect(isNavigable("myapp://callback?code=abc")).toBe(false);
  });

  it("URL として解釈できない文字列や空文字は、開かない", () => {
    expect(isNavigable("")).toBe(false);
    expect(isNavigable("/relative/path")).toBe(false);
    expect(isNavigable("not a url")).toBe(false);
  });
});
