// @vitest-environment jsdom
import { act, useState } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, click, mount } from "../../test/dom";
import { Photo } from "./Photo";

afterEach(cleanup);

describe("写真の代替表示", () => {
  it("写真の取得に失敗しても代替表示を残し、写真URLが変わると新しい写真を表示する", async () => {
    function Example() {
      const [src, setSrc] = useState("/old.jpg");
      return <><Photo src={src} alt="バーガー" className="photo" fallbackClassName="empty" fallback="写真なし" /><button onClick={() => setSrc("/new.jpg")}>写真変更</button></>;
    }
    const page = await mount(<Example />);
    await act(async () => page.querySelector("img")!.dispatchEvent(new Event("error")));
    expect(page.textContent).toContain("写真なし");
    expect(page.querySelector("img")).toBeNull();
    await click(page.querySelector("button")!);
    expect(page.querySelector("img")?.getAttribute("src")).toBe("/new.jpg");
    expect(page.textContent).not.toContain("写真なし");
  });
});
