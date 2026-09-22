// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import { MemoryRouter } from "react-router-dom";
import "../../../lib/i18n";
import { cleanup, mount } from "../../../test/dom";
import { ShopPosterCard } from "./ShopPosterCard";
import type { Shop } from "../api/types";

const shop = (over: Partial<Shop>): Shop => ({
  id: "1",
  name: "Shake Shack",
  status: "active",
  photoUrl: null,
  averageRating: null,
  reviewCount: 0,
  ...over,
});

const show = (ui: React.ReactElement) => mount(<MemoryRouter>{ui}</MemoryRouter>);
afterEach(cleanup);

describe("ShopPosterCard の平均評価", () => {
  // /meta(rating.max)が、まだ取得できていない間の見た目。整数の平均でも、小数第 1 位まで表示する
  // (JSON は 4.0 を 4 と送ってくるため、そのまま出すと、平均が整数のショップだけ小数点が消える)。
  it("ratingMax が未取得でも、整数の平均を小数第 1 位まで表示する", async () => {
    const page = await show(<ShopPosterCard shop={shop({ averageRating: 4, reviewCount: 56 })} ratingMax={undefined} />);
    expect(page.textContent).toContain("4.0");
  });

  it("ratingMax が未取得でも、小数の平均はそのまま表示する", async () => {
    const page = await show(<ShopPosterCard shop={shop({ averageRating: 4.3, reviewCount: 3 })} ratingMax={undefined} />);
    expect(page.textContent).toContain("4.3");
  });

  it("ratingMax が取得済みなら、整数の平均も小数第 1 位まで表示する(RatingBurger 側)", async () => {
    const page = await show(<ShopPosterCard shop={shop({ averageRating: 4, reviewCount: 56 })} ratingMax={5} />);
    expect(page.textContent).toContain("4.0");
  });
});

describe("ShopPosterCard のクリック判定", () => {
  // カードのどこを押しても、ショップ詳細に移動できる(stretched link)。実際のリンクは店名の 1 つだけ
  // (読み上げで、同じ行き先を二重に伝えないため)。
  it("リンクは 1 つだけで、カード全体(写真を含む)を覆う", async () => {
    const page = await show(<ShopPosterCard shop={shop({ id: "42", photoUrl: "https://example.com/p.jpg" })} ratingMax={5} />);
    const links = page.querySelectorAll("a");
    expect(links.length).toBe(1);
    expect(links[0].getAttribute("href")).toBe("/shops/42");
  });
});
