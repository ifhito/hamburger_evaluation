// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import { MemoryRouter } from "react-router-dom";
import "../../../lib/i18n";
import { cleanup, mount } from "../../../test/dom";
import { BurgerRankingCard } from "./BurgerRankingCard";
import type { BurgerRanking } from "../api/types";

const burger = (over: Partial<BurgerRanking> = {}): BurgerRanking => ({
  id: "42",
  name: "テリヤキバーガー",
  photoUrl: null,
  shop: { id: "7", name: "バーガーラボ 中目黒" },
  averageRating: 4.8,
  weightedScore: 4.7,
  reviewCount: 8,
  ...over,
});

const show = (ui: React.ReactElement) => mount(<MemoryRouter>{ui}</MemoryRouter>);
afterEach(cleanup);

describe("BurgerRankingCard", () => {
  it("渡された順位をバッジに表示する", async () => {
    const page = await show(<BurgerRankingCard burger={burger()} rank={3} ratingMax={5} />);
    expect(page.textContent).toContain("3");
  });

  it("rank を渡さないと、順位バッジを描画しない", async () => {
    const page = await show(<BurgerRankingCard burger={burger()} ratingMax={5} />);
    expect(page.querySelector('[aria-label^="Rank"]')).toBeNull();
  });

  it("写真・バーガー名・店舗名からそれぞれの詳細へ向かう", async () => {
    const page = await show(<BurgerRankingCard burger={burger({ id: "42", name: "テリヤキバーガー" })} rank={1} ratingMax={5} />);
    const links = page.querySelectorAll("a");
    expect(links.length).toBe(3);
    expect(links[0].getAttribute("href")).toBe("/burgers/42");
    expect(links[1].textContent).toBe("テリヤキバーガー");
    expect(links[2].getAttribute("href")).toBe("/shops/7");
    expect(page.querySelector("a a")).toBeNull();
  });

  it("ショップ名とレビュー件数を表示する", async () => {
    const page = await show(<BurgerRankingCard burger={burger({ shop: { id: "7", name: "バーガーラボ 中目黒" }, reviewCount: 8 })} rank={1} ratingMax={5} />);
    expect(page.textContent).toContain("バーガーラボ 中目黒");
    expect(page.textContent).toContain("8 reviews");
  });

  describe("平均評価", () => {
    it("ratingMax が未取得でも、整数の平均を小数第 1 位まで表示する", async () => {
      const page = await show(<BurgerRankingCard burger={burger({ averageRating: 4 })} rank={1} ratingMax={undefined} />);
      expect(page.textContent).toContain("4.0");
    });

    it("ratingMax が取得済みなら、整数の平均も小数第 1 位まで表示する(RatingBurger 側)", async () => {
      const page = await show(<BurgerRankingCard burger={burger({ averageRating: 4 })} rank={1} ratingMax={5} />);
      expect(page.textContent).toContain("4.0");
    });
  });
});
