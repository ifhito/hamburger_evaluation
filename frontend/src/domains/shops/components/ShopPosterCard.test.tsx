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
