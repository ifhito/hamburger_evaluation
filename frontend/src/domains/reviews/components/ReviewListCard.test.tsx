import { describe, it, expect } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter } from "react-router-dom";
import "../../../lib/i18n";
import type { Review } from "../api/types";
import { ReviewListCard } from "./ReviewListCard";

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
const html = (r: Review) =>
  renderToStaticMarkup(
    <MemoryRouter>
      <ReviewListCard review={r} ratingMax={5} />
    </MemoryRouter>,
  );

describe("ReviewListCard(レビュー一覧のカード)", () => {
  it("写真・バーガー名・投稿者名からそれぞれの詳細へ移動できる", () => {
    const markup = html(review);

    expect(markup).not.toContain("詳しく見る");
    expect(markup).not.toContain("Read more");
    expect(markup).toContain('href="/users/u1"');
    expect(markup).toContain('href="/reviews/r1"');
    expect(markup).toContain("<article");
  });

  it("リンクの aria-label に、バーガー名と投稿者名が入る", () => {
    const markup = html(review);
    const ariaLabel = markup.match(/aria-label="([^"]+)"/)?.[1];

    expect(ariaLabel).toContain("Teriyaki");
    expect(ariaLabel).toContain("alice");
    expect(ariaLabel).not.toContain("{{");
  });

  it("バーガーの情報がないレビューでも、壊れずに、投稿者名の入った aria-label を出す", () => {
    const markup = html({ ...review, burger: null });
    const ariaLabel = markup.match(/aria-label="([^"]+)"/)?.[1];

    expect(markup).not.toContain('href="/burgers/');
    expect(markup).toContain('href="/reviews/r1"');
    expect(ariaLabel).toContain("alice");
    expect(ariaLabel).not.toContain("{{");
  });

  it("comment が空のときは、空のコメント欄を出さない(評価だけのレビュー)", () => {
    const markup = html({ ...review, comment: "" });

    expect(markup).not.toMatch(/<p[^>]*class="[^"]*comment[^"]*"[^>]*>/);
  });
});

describe("ReviewListCard の実食日(visitedAt)", () => {
  it("visitedAt があるときは、実食日を出す", () => {
    const markup = html({ ...review, visitedAt: "2026-09-10" });
    expect(markup).toContain("Visited Sep 10, 2026");
  });

  it("visitedAt が null のときは、実食日を出さない", () => {
    const markup = html(review);
    expect(markup).not.toContain("Visited ");
  });
});
