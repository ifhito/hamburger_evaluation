import type { TFunction } from "i18next";
import type { Review } from "../domains/reviews/api/types";

// ShopReviewCard(ショップ詳細)と ReviewListCard(レビュー一覧)が共有する、カード全体の <a> の
// aria-label(バーガー名 + 投稿者名)。2 つの画面で同じ組み立てなので、ここに 1 か所にまとめる
// (reviewCard.module.css が見た目を共有するのと同じやり方)。
export function shopReviewAriaLabel(review: Review, t: TFunction): string {
  const username = review.user?.username ?? t("shops.detail.unknown");
  return review.burger
    ? t("shops.detail.reviewAriaLabel", { burger: review.burger.name, user: username })
    : t("shops.detail.reviewAriaLabelNoBurger", { user: username });
}
