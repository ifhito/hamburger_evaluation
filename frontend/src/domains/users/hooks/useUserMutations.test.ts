// @vitest-environment jsdom
import { createElement, act } from "react";
import useSWR, { SWRConfig } from "swr";
import { afterEach, describe, it, expect, vi } from "vitest";
import { cleanup, mount } from "../../../test/dom";
import { userApiClient } from "../api/userApiClient";
import { isUserKey, useDeleteUser, useUpdateUser } from "./useUserMutations";

afterEach(cleanup);

describe("isUserKey", () => {
  it("useUser の配列キーに一致する", () => {
    expect(isUserKey(["/users", "0b0e3a5c-8d54-4c1a-9f33-2a9d6f1c7e10", "5d2c8a91-3f47-4e6b-8c15-7a0e9b4d2f63"])).toBe(true);
    expect(isUserKey(["/users", "0b0e3a5c-8d54-4c1a-9f33-2a9d6f1c7e10", null])).toBe(true);
  });

  it("廃止した一覧のキーや、他ドメインのキーには一致しない", () => {
    expect(isUserKey("/users")).toBe(false);
    expect(isUserKey(["/reviews"])).toBe(false);
    expect(isUserKey("/reviews?page=1")).toBe(false);
    expect(isUserKey(null)).toBe(false);
  });
});

describe("ユーザー更新・退会時のキャッシュ更新", () => {
  it("更新時はプロフィールとレビューを再検証し、退会時は削除済みユーザーの再取得(404)を飛ばさずに値を空にする", async () => {
    const putSpy = vi.spyOn(userApiClient, "put").mockResolvedValue({
      data: { id: "u1", username: "new", email: "a@example.com", canModerate: false },
    } as never);
    const deleteSpy = vi.spyOn(userApiClient, "delete").mockResolvedValue({ data: {} } as never);
    try {
      let userFetches = 0;
      let reviewFetches = 0;
      let userData: string | undefined;
      let updateFn!: ReturnType<typeof useUpdateUser>["update"];
      let destroyFn!: ReturnType<typeof useDeleteUser>["destroy"];

      function Harness() {
        const u = useSWR(["/users", "u1", "u1"], async () => `user-${++userFetches}`);
        useSWR(["/reviews", "r1", "u1"], async () => `rev-${++reviewFetches}`);
        userData = u.data;
        updateFn = useUpdateUser("u1").update;
        destroyFn = useDeleteUser().destroy;
        return null;
      }

      await mount(
        createElement(
          SWRConfig,
          { value: { provider: () => new Map(), dedupingInterval: 0 } },
          createElement(Harness),
        ),
      );
      expect(userFetches).toBe(1);
      expect(reviewFetches).toBe(1);
      expect(userData).toBe("user-1");

      await act(async () => {
        await updateFn({ username: "new" });
      });
      expect(userFetches).toBe(2);
      expect(reviewFetches).toBe(2);
      expect(userData).toBe("user-2");

      await act(async () => {
        await destroyFn("u1");
      });
      // 再検証なしで空になるため GET は増えない
      expect(userFetches).toBe(2);
      expect(userData).toBeUndefined();
    } finally {
      putSpy.mockRestore();
      deleteSpy.mockRestore();
    }
  });
});
