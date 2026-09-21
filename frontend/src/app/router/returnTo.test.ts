import { describe, it, expect } from "vitest";
import { returnPathFrom } from "./returnTo";

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
