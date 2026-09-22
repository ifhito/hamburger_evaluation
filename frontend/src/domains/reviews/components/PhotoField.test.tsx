// @vitest-environment jsdom
import { act } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import "../../../lib/i18n";
import { byText, cleanup, click, mount, need } from "../../../test/dom";
import { PhotoField } from "./PhotoField";

// 実際のブラウザの画像デコード(createImageBitmap・canvas)は jsdom にないので、shrinkPhoto を差し替える。
// 差し替え自体の正しさ(小さくするかどうかの判断)は lib/photoResize.test.ts が確かめる。ここでは、
// PhotoField が「選ぶ → 小さくしている間 → 付いた」の見た目を正しく切り替え、変える/外す操作が働くことだけを見る。
vi.mock("../../../lib/photoResize", () => ({
  shrinkPhoto: vi.fn(async (file: File) => new File([file], "shrunk.jpg", { type: "image/jpeg" })),
}));

const limits = { maxEdge: 1600, maxBytes: 100 };
const file = (name: string, size: number) => {
  const f = new File([new Uint8Array(size)], name, { type: "image/jpeg" });
  return f;
};

afterEach(cleanup);

describe("PhotoField", () => {
  it("最初は「写真を追加」の空の見た目で、写真を選ぶボタンがある", async () => {
    const page = await mount(<PhotoField photo={null} onChange={() => {}} limits={limits} />);
    expect(page.textContent).toContain("Add a photo");
    expect(byText(page, "button", "Choose a photo")).toBeDefined();
  });

  it("写真を選ぶと、小さくしてから onChange に渡し、小さくした前後の大きさを出す", async () => {
    const onChange = vi.fn();
    const onShrinkingChange = vi.fn();
    const page = await mount(<PhotoField photo={null} onChange={onChange} limits={limits} onShrinkingChange={onShrinkingChange} />);
    const input = need(page.querySelector<HTMLInputElement>('input[type="file"]'), "file input");

    // jsdom は DataTransfer を実装していないので、files を直接差し替える(ブラウザの操作の代わり)。
    const selected = file("teriyaki.png", 200);
    Object.defineProperty(input, "files", { value: [selected], configurable: true });
    await act(async () => {
      input.dispatchEvent(new Event("change", { bubbles: true }));
    });

    expect(onShrinkingChange).toHaveBeenCalledWith(true);
    expect(onShrinkingChange).toHaveBeenCalledWith(false);
    expect(onChange).toHaveBeenCalledTimes(1);
    expect(onChange.mock.calls[0][0].name).toBe("shrunk.jpg");
  });

  it("続けて 2 枚選んだときは、あとに選んだ方が勝つ(先に選んだ方の小さくする処理が遅れて終わっても上書きしない)", async () => {
    const { shrinkPhoto } = await import("../../../lib/photoResize");
    const deferred = new Map<string, { resolve: (f: File) => void }>();
    vi.mocked(shrinkPhoto).mockImplementation(
      (f: File) =>
        new Promise<File>((resolve) => {
          deferred.set(f.name, { resolve: (shrunk) => resolve(shrunk) });
        }),
    );

    const onChange = vi.fn();
    const page = await mount(<PhotoField photo={null} onChange={onChange} limits={limits} />);
    const input = need(page.querySelector<HTMLInputElement>('input[type="file"]'), "file input");

    const selectFile = async (name: string) => {
      Object.defineProperty(input, "files", { value: [file(name, 10)], configurable: true });
      await act(async () => {
        input.dispatchEvent(new Event("change", { bubbles: true }));
      });
    };

    await selectFile("first.png");
    await selectFile("second.png");

    // 先に選んだ方(first)が、あとから終わる。
    const firstShrunk = new File(["a"], "first-shrunk.jpg", { type: "image/jpeg" });
    const secondShrunk = new File(["b"], "second-shrunk.jpg", { type: "image/jpeg" });
    await act(async () => {
      need(deferred.get("second.png"), "second.png の処理").resolve(secondShrunk);
    });
    await act(async () => {
      need(deferred.get("first.png"), "first.png の処理").resolve(firstShrunk);
    });

    expect(onChange).toHaveBeenCalledTimes(1);
    expect(onChange).toHaveBeenCalledWith(secondShrunk);
  });

  it("付いた状態は、写真のプレビューと「変える」「外す」を出す。「外す」で onChange(null) が呼ばれる", async () => {
    const onChange = vi.fn();
    const attached = file("shrunk.jpg", 50);
    const page = await mount(<PhotoField photo={attached} onChange={onChange} limits={limits} allowRemove />);

    expect(byText(page, "button", "Change photo")).toBeDefined();
    const removeBtn = need(byText<HTMLButtonElement>(page, "button", "Remove"), "Remove");
    await click(removeBtn);
    expect(onChange).toHaveBeenCalledWith(null);
  });

  it("編集の、いまの写真(existingPhotoUrl)は「変える」だけを出し、外す操作は出ない", async () => {
    const page = await mount(
      <PhotoField photo={null} onChange={() => {}} limits={limits} existingPhotoUrl="https://example.com/a.jpg" existingPhotoCaption="Posted on Sep 21, 2026" />,
    );
    expect(page.textContent).toContain("Posted on Sep 21, 2026");
    expect(byText(page, "button", "Change photo")).toBeDefined();
    expect(byText(page, "button", "Remove")).toBeUndefined();
  });
});
