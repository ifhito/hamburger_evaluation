import { describe, it, expect } from "vitest";
import { burgerKey } from "./useBurger";

const viewer3 = "00000000-0000-4000-8000-000000000003";
const viewer4 = "00000000-0000-4000-8000-000000000004";
const burger2 = "00000000-0000-4000-8000-000000000002";

describe("burgerKey", () => {
  it("id と viewerId を含む配列キーを返す", () => {
    expect(burgerKey(burger2, viewer3)).toEqual(["/burgers", burger2, viewer3]);
    expect(burgerKey(burger2, null)).toEqual(["/burgers", burger2, null]);
  });

  it("閲覧者が違えば(未ログイン含む)別のキーになる(shops が閲覧者ごとに違うため)", () => {
    expect(burgerKey(burger2, viewer3)).not.toEqual(burgerKey(burger2, null));
    expect(burgerKey(burger2, viewer3)).not.toEqual(burgerKey(burger2, viewer4));
  });

  it("enabled が false、または id が空なら null を返す", () => {
    expect(burgerKey(burger2, viewer3, false)).toBeNull();
    expect(burgerKey(undefined, viewer3)).toBeNull();
    expect(burgerKey("", viewer3)).toBeNull();
  });
});
