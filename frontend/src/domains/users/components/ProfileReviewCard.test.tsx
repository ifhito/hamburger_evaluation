import { describe, it, expect } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter } from "react-router-dom";
import "../../../lib/i18n";
import type { Review } from "../../reviews/api/types";
import { ProfileReviewCard } from "./ProfileReviewCard";

const review: Review = {
  id: "r1",
  rating: 4,
  comment: "Great patty",
  photoUrl: null,
  createdAt: "2026-09-21T00:00:00Z",
  visitedAt: null,
  user: { id: "u1", username: "alice" },
  burger: { id: "b1", name: "Teriyaki", averageRating: 4, reviewCount: 3, weightedScore: 4, confidence: 1 },
};
const html = (r: Review, ratingMax: number | "unknown" = 5) =>
  renderToStaticMarkup(
    <MemoryRouter>
      <ProfileReviewCard review={r} ratingMax={ratingMax === "unknown" ? undefined : ratingMax} />
    </MemoryRouter>,
  );

describe("ProfileReviewCard(プロフィールのレビューのカード)", () => {
  it("写真があるとき: 写真を出す。ないとき: 「No photo」の面を出す", () => {
    expect(html({ ...review, photoUrl: "/photos/a.jpg" })).toContain('src="/photos/a.jpg"');
    expect(html({ ...review, photoUrl: "/photos/a.jpg" })).not.toContain("No photo");

    expect(html(review)).toContain("No photo");
    expect(html(review)).not.toContain("<img");
  });

  it("評価の最大値が分かるとき: 水位のバーガーと数字(読み上げは「Rating 4 out of 5」)を出す。分からない間は、数字だけを出す", () => {
    expect(html(review)).toContain('aria-label="Rating 4 out of 5"');

    const waiting = html(review, "unknown");
    expect(waiting).not.toContain("<svg");
    expect(waiting).toContain(">4<");
  });

  it("バーガーの名前・コメントに HTML があっても、文字として出す(解釈しない)", () => {
    const markup = html({ ...review, comment: "<img src=x onerror=alert(1)>", burger: { ...review.burger!, name: "<b>Big</b>" } });

    expect(markup).not.toContain("<img src=x");
    expect(markup).toContain("&lt;img src=x onerror=alert(1)&gt;");
    expect(markup).toContain("&lt;b&gt;Big&lt;/b&gt;");
  });

  it("バーガーの情報がないレビューでも、壊れずに、日付とレビューへのリンクを出す", () => {
    const markup = html({ ...review, burger: null });

    expect(markup).not.toContain("<h3");
    expect(markup).toContain('href="/reviews/r1"');
    expect(markup).toContain('dateTime="2026-09-21T00:00:00Z"');
  });

  it("写真からレビュー詳細へ、バーガー名からバーガー詳細へ移動できる", () => {
    const markup = html(review);

    expect(markup).not.toContain("詳しく見る");
    expect(markup).not.toContain("View");
    expect(markup).toContain('href="/reviews/r1"');
    expect(markup).toContain('href="/burgers/b1"');
  });

  it("comment が空のときは、空のコメント欄を出さない(評価だけのレビュー)", () => {
    const markup = html({ ...review, comment: "" });

    expect(markup).not.toMatch(/<p[^>]*class="[^"]*comment[^"]*"[^>]*>/);
  });

  it("リンクの aria-label に、日付を差し替えた文字列が入る(タイムゾーンで変わる書式そのものは固定しない)", () => {
    const withBurger = html(review).match(/aria-label="([^"]+)"/)?.[1];
    expect(withBurger).toContain("Teriyaki");
    expect(withBurger).not.toContain("{{");

    const withoutBurger = html({ ...review, burger: null }).match(/aria-label="([^"]+)"/)?.[1];
    expect(withoutBurger).toBeTruthy();
    expect(withoutBurger).not.toContain("{{");
  });
});

describe("ProfileReviewCard の実食日(visitedAt)", () => {
  it("visitedAt があるときは、実食日を出す", () => {
    const markup = html({ ...review, visitedAt: "2026-09-10" });
    expect(markup).toContain("Visited Sep 10, 2026");
  });

  it("visitedAt が null のときは、実食日を出さない", () => {
    const markup = html(review);
    expect(markup).not.toContain("Visited ");
  });
});


describe("プロフィールの関連店舗", () => {
  it("レビューの店舗名を、正しい店舗詳細へのリンクとして表示する", () => {
    const markup = html({ ...review, shop: { id: "shop-b", name: "別のバーガー店" } });
    expect(markup).toContain('href="/shops/shop-b"');
    expect(markup).toContain("別のバーガー店");
  });
  it("店舗情報がない場合も評価を残し、壊れた店舗リンクを作らない", () => {
    const markup = html({ ...review, shop: null });
    expect(markup).not.toContain('href="/shops/');
    expect(markup).toContain('href="/reviews/r1"');
  });
});
