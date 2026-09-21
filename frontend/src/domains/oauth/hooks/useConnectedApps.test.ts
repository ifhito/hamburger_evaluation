import { describe, it, expect } from "vitest";
import { ApiError } from "../../../api/client/buildApiClient";
import { connectedAppsKey, isFeatureDisabled } from "./useConnectedApps";

const aliceId = "0b0e3a5c-8d54-4c1a-9f33-2a9d6f1c7e10";
const bobId = "5d2c8a91-3f47-4e6b-8c15-7a0e9b4d2f63";

describe("connectedAppsKey", () => {
  it("閲覧者が確定していれば、閲覧者の id を含むキーを返す", () => {
    expect(connectedAppsKey(aliceId)).toEqual(["/oauth/grants", aliceId]);
  });

  it("閲覧者が確定していなければ null を返し、取得しない", () => {
    expect(connectedAppsKey(null)).toBeNull();
  });

  it("利用者が違えば別のキーになり、前の利用者の一覧を再利用しない", () => {
    expect(connectedAppsKey(aliceId)).not.toEqual(connectedAppsKey(bobId));
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
