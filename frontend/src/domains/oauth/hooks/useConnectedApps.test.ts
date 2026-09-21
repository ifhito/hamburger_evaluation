import { describe, it, expect } from "vitest";
import { ApiError } from "../../../api/client/buildApiClient";
import type { Page } from "../../../api/page";
import type { ConnectedApp } from "../api/types";
import { getKey, isFeatureDisabled } from "./useConnectedApps";

const aliceId = "0b0e3a5c-8d54-4c1a-9f33-2a9d6f1c7e10";

function app(id: string): ConnectedApp {
  return { id, clientId: "app", clientName: "アプリ", scopes: [], createdAt: "2030-01-01T00:00:00Z", updatedAt: "2030-01-01T00:00:00Z" };
}

// ページの件数はテストの関心ではない。続きがあるかは hasMore(backend の判断)だけで決まる
function page(hasMore: boolean, count = 1): Page<ConnectedApp> {
  return { items: Array.from({ length: count }, (_, i) => app(`00000000-0000-4000-8000-${String(i + 1).padStart(12, "0")}`)), hasMore };
}

describe("getKey", () => {
  it("先頭ページ(previous が null)は、1 ページ目の URL を返す(1 ページの件数は送らない)", () => {
    expect(getKey(aliceId)(0, null)).toBe("/oauth/grants?page=1");
  });

  it("前ページに続きがある(hasMore が true)なら、次ページの URL を返す", () => {
    expect(getKey(aliceId)(1, page(true))).toBe("/oauth/grants?page=2");
  });

  it("前ページが最終(hasMore が false)なら、件数によらず null を返して読み込みを止める", () => {
    expect(getKey(aliceId)(1, page(false, 20))).toBeNull();
    expect(getKey(aliceId)(1, page(false, 0))).toBeNull();
  });

  it("件数が少なくても、hasMore が true なら続きを読み込む(件数から最終ページを推測しない)", () => {
    expect(getKey(aliceId)(1, page(true, 1))).toBe("/oauth/grants?page=2");
  });

  it("閲覧者が確定していなければ、どのページも null を返し、取得しない", () => {
    expect(getKey(null)(0, null)).toBeNull();
    expect(getKey(null)(1, page(true))).toBeNull();
  });
});

describe("isFeatureDisabled", () => {
  it("404(認可サーバーが無効で、窓口が未登録)は、機能が使えない印として扱う", () => {
    expect(isFeatureDisabled(new ApiError(["not found"], 404))).toBe(true);
  });

  it("401 や 500 などの失敗は、機能が無効なのではなく、エラーとして扱う", () => {
    expect(isFeatureDisabled(new ApiError(["Unauthorized"], 401))).toBe(false);
    expect(isFeatureDisabled(new ApiError(["boom"], 500))).toBe(false);
    expect(isFeatureDisabled(new Error("network"))).toBe(false);
    expect(isFeatureDisabled(undefined)).toBe(false);
  });
});
