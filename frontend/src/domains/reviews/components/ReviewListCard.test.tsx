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
  it("「詳しく見る」の文字を出さず、カード全体がレビュー詳細への 1 つのリンクになる", () => {
    const markup = html(review);

    expect(markup).not.toContain("詳しく見る");
    expect(markup).not.toContain("Read more");
    expect(markup.match(/<a /g)?.length).toBe(1);
    expect(markup).toContain('href="/reviews/r1"');
  });

  it("リンクに、空でない aria-label が付く", () => {
    const markup = html(review);

    expect(markup).toMatch(/aria-label="[^"]+"/);
  });

  it("バーガーの情報がないレビューでも、壊れずに、空でない aria-label を出す", () => {
    const markup = html({ ...review, burger: null });

    expect(markup.match(/<a /g)?.length).toBe(1);
    expect(markup).toContain('href="/reviews/r1"');
    expect(markup).toMatch(/aria-label="[^"]+"/);
  });
});
