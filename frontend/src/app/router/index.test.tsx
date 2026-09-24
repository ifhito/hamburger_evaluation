// @vitest-environment jsdom
import { describe, expect, it } from "vitest";
import { router } from "./index";
import HomePage from "../../domains/home/pages/HomePage";

describe("router の /", () => {
  // / は、以前の /shops への転送(Navigate)ではなく、新しいトップページ(HomePage)を表示する。
  it("最上位で HomePage を表示する", () => {
    const top = router.routes.find((r) => r.path === "/");
    expect(top, "最上位に / がない").toBeDefined();
    if (!top || !("element" in top)) throw new Error("route element not found");
    expect(top.element).toMatchObject({ type: HomePage });
  });
});

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

describe("サービス紹介ページ", () => {
  it("サインインを求めずに公開ページを開ける", () => {
    expect(router.routes.find((route) => route.path === "/about")).toBeDefined();
    const guarded = router.routes.filter((route) => route.path === undefined).flatMap((route) => route.children ?? []);
    expect(guarded.some((route) => route.path === "/about")).toBe(false);
  });
});
