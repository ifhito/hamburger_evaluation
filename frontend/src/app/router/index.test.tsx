// @vitest-environment jsdom
import { describe, expect, it } from "vitest";
import { router } from "./index";

// Google でのサインインの結果の受け皿は、公開の route(ゲスト専用にしない)。ゲスト専用の route の下に置くと、ログイン中の
// 利用者が結び付けを終えて戻ったときに、交換の前に /reviews へ移動させられ、1 回限りのコードが使われずに終わる。
describe("router の /auth/google/complete", () => {
  it("公開の route として、最上位に登録されている(ゲスト専用・要ログインの route の下ではない)", () => {
    const top = router.routes.find((r) => r.path === "/auth/google/complete");
    expect(top, "最上位に /auth/google/complete がない").toBeDefined();

    const guarded = router.routes.filter((r) => r.path === undefined).flatMap((r) => r.children ?? []);
    expect(guarded.some((r) => r.path === "/auth/google/complete")).toBe(false);
  });
});
