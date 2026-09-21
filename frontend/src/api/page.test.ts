import { describe, it, expect } from "vitest";
import type { AxiosResponse } from "axios";
import { mergePages, toPage, type Page } from "./page";

function res(data: unknown, headers: Record<string, string> = {}): AxiosResponse<{ id: number }[]> {
  return { data, headers } as AxiosResponse<{ id: number }[]>;
}

function page(ids: number[], hasMore = false): Page<{ id: number }> {
  return { items: ids.map((id) => ({ id })), hasMore };
}

describe("toPage", () => {
  it("配列と X-Has-More: true を、続きありのページにする", () => {
    expect(toPage(res([{ id: 1 }], { "x-has-more": "true" }))).toEqual({ items: [{ id: 1 }], hasMore: true });
  });

  it("X-Has-More: false を、最終ページにする", () => {
    expect(toPage(res([], { "x-has-more": "false" }))).toEqual({ items: [], hasMore: false });
  });

  it("配列でない本文は失敗させる", () => {
    expect(() => toPage(res({ error: "x" }, { "x-has-more": "false" }))).toThrow("expected array");
  });

  it("X-Has-More がない(proxy に落とされた)ときは、最終ページと決めつけず失敗させる", () => {
    expect(() => toPage(res([{ id: 1 }]))).toThrow("X-Has-More");
  });

  it("X-Has-More が true / false 以外でも失敗させる", () => {
    expect(() => toPage(res([{ id: 1 }], { "x-has-more": "yes" }))).toThrow("X-Has-More");
  });
});

describe("mergePages", () => {
  it("ページをつないで 1 つの一覧にする", () => {
    expect(mergePages([page([1, 2, 3], true), page([4, 5])]).map((r) => r.id)).toEqual([1, 2, 3, 4, 5]);
  });

  it("前ページ末尾が次ページに再登場しても、先頭出現の位置で 1 件にする", () => {
    expect(mergePages([page([1, 2, 3], true), page([3, 4])]).map((r) => r.id)).toEqual([1, 2, 3, 4]);
  });

  it("ページがなければ空になる", () => {
    expect(mergePages([])).toEqual([]);
  });
});
