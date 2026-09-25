// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { act, useState } from "react";
import { SWRConfig } from "swr";
import { cleanup, click, eventually, mount } from "../test/dom";
import { useBurgers } from "../domains/burgers/hooks/useBurgers";
import { useShops } from "../domains/shops/hooks/useShops";
import { useReviews } from "../domains/reviews/hooks/useReviews";
import { burgerApiClient } from "../domains/burgers/api/burgerApiClient";
import { shopApiClient } from "../domains/shops/api/shopApiClient";
import { reviewApiClient } from "../domains/reviews/api/reviewApiClient";
vi.mock("../domains/burgers/api/burgerApiClient", () => ({ burgerApiClient: { get: vi.fn() } }));
vi.mock("../domains/shops/api/shopApiClient", () => ({ shopApiClient: { get: vi.fn() } }));
vi.mock("../domains/reviews/api/reviewApiClient", () => ({ reviewApiClient: { get: vi.fn() } }));
afterEach(cleanup);
type Params = { keyword: string; sort: string };
const cases = [
  { name: "バーガー", api: burgerApiClient, useResults: (p: Params) => useBurgers(p) },
  { name: "店舗", api: shopApiClient, useResults: (p: Params) => useShops(p, "viewer") },
  { name: "レビュー", api: reviewApiClient, useResults: (p: Params) => useReviews(p, "viewer") },
];
for (const entry of cases) {
 describe(`${entry.name}の検索と並び替え`, () => {
  it("条件変更で先頭へ戻り、古い応答を混在させず同じ条件で続きを取得する", async () => {
   const get = vi.mocked(entry.api.get); get.mockReset();
   let finishOld: (() => void) | undefined;
   get.mockImplementation(async (url: string) => {
    const q = new URL(url, "https://example.com").searchParams;
    const keyword = q.get("keyword") ?? ""; const page = q.get("page");
    if (keyword === "古い") await new Promise<void>((resolve) => { finishOld = resolve; });
    return { data: [{ id: `${keyword || "全件"}-${page}` }], headers: { "x-has-more": page === "1" ? "true" : "false" } };
   });
   function Probe() {
    const [params, setParams] = useState({ keyword: "", sort: "newest" });
    const result = entry.useResults(params);
    return <>
     <div data-testid="results">{result.data?.map((row) => row.id).join(",")}</div>
     <button onClick={result.fetchNextPage}>続き</button>
     <button onClick={() => setParams({ keyword: "古い", sort: "newest" })}>古い検索</button>
     <button onClick={() => setParams({ keyword: "新しい", sort: "name" })}>条件変更</button>
     <button onClick={() => setParams({ keyword: "", sort: "newest" })}>解除</button>
     <button onClick={() => setParams({ keyword: "新しい", sort: "newest" })}>順序変更</button>
    </>;
   }
   const page = await mount(<SWRConfig value={{ provider: () => new Map(), revalidateOnFocus: false, dedupingInterval: 0, shouldRetryOnError: false }}><Probe /></SWRConfig>);
   const results = () => page.querySelector('[data-testid="results"]')?.textContent;
   await eventually(() => expect(results()).toBe("全件-1"));
   expect(get).toHaveBeenCalledTimes(1);
   await click(page.querySelectorAll("button")[0]);
   await eventually(() => expect(results()).toBe("全件-1,全件-2"));
   await click(page.querySelectorAll("button")[1]);
   await eventually(() => expect(finishOld).toBeDefined());
   await click(page.querySelectorAll("button")[2]);
   await eventually(() => expect(results()).toBe("新しい-1"));
   await act(async () => { finishOld!(); });
   await eventually(() => expect(results()).toBe("新しい-1"));
   await click(page.querySelectorAll("button")[0]);
   await eventually(() => expect(results()).toBe("新しい-1,新しい-2"));
   expect(get.mock.calls.some(([url]) => {
    const q = new URL(url, "https://example.com").searchParams;
    return q.get("keyword") === "新しい" && q.get("sort") === "name" && q.get("page") === "2";
   })).toBe(true);
   await click(page.querySelectorAll("button")[4]);
   await eventually(() => expect(results()).toBe("新しい-1"));
   expect(get.mock.calls[get.mock.calls.length - 1]?.[0]).toContain("sort=newest&page=1");
   await click(page.querySelectorAll("button")[3]);
   await eventually(() => expect(results()).toBe("全件-1"));
  });
 });
}
