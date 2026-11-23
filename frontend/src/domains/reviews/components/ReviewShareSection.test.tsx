// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import "../../../lib/i18n";
import { byText, cleanup, click, mount, need } from "../../../test/dom";
import { ReviewShareSection } from "./ReviewShareSection";

afterEach(async () => {
  vi.restoreAllMocks();
  Reflect.deleteProperty(navigator, "share");
  await cleanup();
});

describe("ReviewShareSection の X 共有リンクと共有操作", () => {
  it("バーガー名・店名・評価・コメント・ハッシュタグとレビュー URL を含む X 投稿リンクを組み立てる", async () => {
    const view = await mount(
      <ReviewShareSection
        reviewId="rev-123"
        burgerName="ダブルチーズバーガー"
        shopName="Burger Mania 恵比寿"
        rating={5}
        ratingMax={5}
        comment="肉汁があふれて最高！"
      />,
    );

    const xLink = need(view.querySelector<HTMLAnchorElement>('a[href^="https://x.com/intent/post?"]'), "X link");
    expect(xLink.target).toBe("_blank");
    expect(xLink.rel).toBe("noopener noreferrer");

    const parsed = new URL(xLink.href);
    expect(parsed.searchParams.get("url")).toBe(`${window.location.origin}/reviews/rev-123`);
    const text = parsed.searchParams.get("text") ?? "";
    expect(text).toContain("🍔 ダブルチーズバーガー（Burger Mania 恵比寿） ★5/5");
    expect(text).toContain("肉汁があふれて最高！");
    expect(text).toContain("#BurgerStack");
  });

  it("長いコメントは共有文面で 80 文字に切り詰め、末尾に省略記号を付ける", async () => {
    const longComment = "あ".repeat(95);
    const view = await mount(
      <ReviewShareSection
        reviewId="rev-long"
        burgerName="クラシックバーガー"
        shopName={null}
        rating={4}
        ratingMax={5}
        comment={longComment}
      />,
    );

    const xLink = need(view.querySelector<HTMLAnchorElement>('a[href^="https://x.com/intent/post?"]'), "X link");
    const text = new URL(xLink.href).searchParams.get("text") ?? "";
    expect(text).toContain(`${"あ".repeat(80)}…`);
    expect(text).not.toContain("あ".repeat(81));
  });

  it("バーガー名・店名・コメントがいずれも長い日本語でも、URL 込みで X の 280 ウェイト上限を超えないよう切り詰める", async () => {
    const view = await mount(
      <ReviewShareSection
        reviewId="rev-weight"
        burgerName={"特選黒毛和牛ダブルチーズバーガー".repeat(5)}
        shopName={"東京クラフトバーガーショップ本店".repeat(5)}
        rating={5}
        ratingMax={5}
        comment={"肉汁とバンズの香ばしさが抜群でした。".repeat(8)}
      />,
    );

    const xLink = need(view.querySelector<HTMLAnchorElement>('a[href^="https://x.com/intent/post?"]'), "X link");
    const text = new URL(xLink.href).searchParams.get("text") ?? "";
    const textWeight = [...text].reduce((sum, ch) => sum + ((ch.codePointAt(0) ?? 0) <= 127 ? 1 : 2), 0);
    // URL 分(24 ウェイト)を足しても 280 ウェイト以内に収まること
    expect(textWeight + 24).toBeLessThanOrEqual(280);
    expect(text).toContain("#BurgerStack");
  });

  it("リンクをコピーできたときはボタンの文言を切り替え、失敗したときは手動コピー用の入力欄を出す", async () => {
    const writeText = vi.fn().mockResolvedValueOnce(undefined).mockRejectedValueOnce(new Error("denied"));
    Object.defineProperty(navigator, "clipboard", { value: { writeText }, configurable: true });

    const view = await mount(
      <ReviewShareSection reviewId="rev-copy" burgerName="チーズバーガー" shopName="Shop A" rating={4} comment={null} />,
    );

    const copyBtn = need(byText<HTMLButtonElement>(view, "button", "Copy link"), "Copy link");
    await click(copyBtn);
    expect(writeText).toHaveBeenCalledWith(`${window.location.origin}/reviews/rev-copy`);
    expect(copyBtn.textContent).toBe("Link copied");

    await click(copyBtn);
    const manualInput = need(view.querySelector<HTMLInputElement>("#review-share-link"), "manual copy input");
    expect(manualInput.value).toBe(`${window.location.origin}/reviews/rev-copy`);
  });

  it("端末が Web Share API に対応しているときだけ共有ボタンを出し、キャンセルされても例外にしない", async () => {
    const shareSpy = vi.fn().mockRejectedValueOnce(new DOMException("Aborted", "AbortError"));
    Object.defineProperty(navigator, "share", { value: shareSpy, configurable: true });

    const view = await mount(
      <ReviewShareSection reviewId="rev-native" burgerName="ベーコンバーガー" shopName="Shop B" rating={5} ratingMax={5} comment="うまい" />,
    );

    const nativeBtn = need(byText<HTMLButtonElement>(view, "button", "Share\u2026"), "Share button");
    await click(nativeBtn);

    expect(shareSpy).toHaveBeenCalledWith({
      title: "🍔 ベーコンバーガー（Shop B） ★5/5 | BurgerStack",
      text: "🍔 ベーコンバーガー（Shop B） ★5/5\nうまい\n#BurgerStack",
      url: `${window.location.origin}/reviews/rev-native`,
    });
  });
});
