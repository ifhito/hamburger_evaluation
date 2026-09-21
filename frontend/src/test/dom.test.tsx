// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, mount, unmount } from "./dom";

afterEach(cleanup);

describe("dom(テスト用の道具)の unmount", () => {
  it("すでに外した(または、mount していない)要素を渡されても、ほかの mount を外さない", async () => {
    const a = await mount(<p>a</p>);
    const b = await mount(<p>b</p>);
    await unmount(a);

    await unmount(a); // 二重の unmount
    await unmount(document.createElement("div")); // mount していない要素

    expect(b.textContent).toBe("b");
    expect(document.body.contains(b)).toBe(true);
  });
});
