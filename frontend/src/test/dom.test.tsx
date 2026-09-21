// @vitest-environment jsdom
import { useEffect, useState } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, eventually, mount, unmount } from "./dom";

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

function Delayed() {
  const [n, setN] = useState(0);
  useEffect(() => {
    const id = setTimeout(() => setN(1), 50);
    return () => clearTimeout(id);
  }, []);
  return <p>value:{n}</p>;
}

describe("dom(テスト用の道具)の eventually", () => {
  it("待っている間に、タイマーで起きる描画の更新も、観測できる(act の中に閉じ込めない)", async () => {
    const page = await mount(<Delayed />);

    await eventually(() => expect(page.textContent).toBe("value:1"));
  });

  it("条件が満たされないまま、時間が過ぎたら、最後の失敗を投げる", async () => {
    const page = await mount(<p>never</p>);

    await expect(eventually(() => expect(page.textContent).toBe("something else"))).rejects.toThrow(/something else/);
  }, 10_000);
});
