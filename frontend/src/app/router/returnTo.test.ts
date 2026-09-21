import { describe, it, expect } from "vitest";
import { appPathOrNull, returnPathFrom } from "./returnTo";

describe("returnPathFrom", () => {
  it("このアプリの中のパス(クエリつき)は、戻り先として取り出す", () => {
    expect(returnPathFrom({ from: "/oauth/authorize?client_id=app&state=x" })).toBe("/oauth/authorize?client_id=app&state=x");
    expect(returnPathFrom({ from: "/reviews" })).toBe("/reviews");
  });

  it("外部の URL や、// で始まる(スキームを省いた外部の)URL は、戻り先にしない", () => {
    expect(returnPathFrom({ from: "https://evil.example.com/" })).toBeNull();
    expect(returnPathFrom({ from: "//evil.example.com/" })).toBeNull();
    expect(returnPathFrom({ from: "/\\evil.example.com/" })).toBeNull();
    expect(returnPathFrom({ from: "javascript:alert(1)" })).toBeNull();
  });

  it("state がない・オブジェクトでない・from が文字列でないときは、戻り先なしとして扱う", () => {
    expect(returnPathFrom(null)).toBeNull();
    expect(returnPathFrom(undefined)).toBeNull();
    expect(returnPathFrom("/reviews")).toBeNull();
    expect(returnPathFrom({})).toBeNull();
    expect(returnPathFrom({ from: 1 })).toBeNull();
    expect(returnPathFrom({ from: "" })).toBeNull();
  });
});

describe("appPathOrNull", () => {
  it("このアプリの中のパスはそのまま返し、外部の URL・//・/\\・スキームつき・空は null にする", () => {
    expect(appPathOrNull("/oauth/authorize?client_id=app&state=x")).toBe("/oauth/authorize?client_id=app&state=x");
    for (const bad of ["", "https://evil.example.com/", "//evil.example.com/", "/\\evil.example.com/", "javascript:alert(1)", "reviews"]) {
      expect(appPathOrNull(bad), bad).toBeNull();
    }
  });
});

describe("appPathOrNull(値がない・文字列でないとき)", () => {
  it("undefined・null・文字列でない値は、落ちずに null にする(古い・想定外の応答の本文に備える)", () => {
    for (const bad of [undefined, null, 1, {}, []]) {
      expect(appPathOrNull(bad as unknown as string), String(bad)).toBeNull();
    }
  });
});
