// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useState } from "react";
import { SWRConfig } from "swr";
import { cleanup, click, eventually, mount } from "../../../test/dom";
import { burgerApiClient } from "../api/burgerApiClient";
import type { BurgerRanking } from "../api/types";
import { useBurgerRanking } from "./useBurgerRanking";
import { useBurgers } from "./useBurgers";

vi.mock("../api/burgerApiClient", () => ({ burgerApiClient: { get: vi.fn() } }));

const burgers: BurgerRanking[] = [{
  id: "burger-1", name: "チーズバーガー", shop: { id: "shop-1", name: "バーガー店" },
  averageRating: 4, weightedScore: 3.9, reviewCount: 5,
}];

function Ranking() {
  const { data } = useBurgerRanking();
  return <div data-testid="ranking">{data?.slice(0, 6).map((burger) => burger.name).join(",")}</div>;
}

function List() {
  const { data, hasNextPage } = useBurgers();
  return <div data-testid="list">{data?.map((burger) => burger.name).join(",")}{hasNextPage && "続きあり"}</div>;
}

function Screens({ initialRanking }: { initialRanking: boolean }) {
  const [ranking, setRanking] = useState(initialRanking);
  return <>
    <button onClick={() => setRanking(!ranking)}>移動</button>
    {ranking ? <Ranking /> : <List />}
  </>;
}

beforeEach(() => {
  vi.mocked(burgerApiClient.get).mockReset();
  vi.mocked(burgerApiClient.get).mockResolvedValue({ data: burgers, headers: { "x-has-more": "true" } });
});
afterEach(cleanup);

describe("ランキングと一覧のキャッシュ共有", () => {
  it.each([
    { name: "一覧からホームへ移動してもランキングを表示できる", initialRanking: false },
    { name: "ホームから一覧へ移動してもバーガーと続きを表示できる", initialRanking: true },
  ])("$name", async ({ initialRanking }) => {
    const page = await mount(
      <SWRConfig value={{ provider: () => new Map(), revalidateOnFocus: false, shouldRetryOnError: false }}>
        <Screens initialRanking={initialRanking} />
      </SWRConfig>,
    );
    await eventually(() => expect(page.textContent).toContain("チーズバーガー"));
    // ホームへの遷移では再取得を保留し、キャッシュだけでも抜粋を描画できることを確かめる。
    if (!initialRanking) vi.mocked(burgerApiClient.get).mockReturnValue(new Promise(() => undefined));
    await click(page.querySelector("button")!);
    const target = initialRanking ? "list" : "ranking";
    await eventually(() => expect(page.querySelector(`[data-testid="${target}"]`)?.textContent).toContain("チーズバーガー"));
    if (initialRanking) expect(page.textContent).toContain("続きあり");
  });
});
