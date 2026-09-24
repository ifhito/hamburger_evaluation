import { useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useAuth } from "../../auth/AuthProvider";
import { useReview } from "../hooks/useReview";
import { useDeleteReview } from "../hooks/useReviewMutations";
import { useRatingRange } from "../hooks/useRatingRange";
import { formatDate, formatVisitedAt } from "../../../lib/date";
import { Button } from "../../../components/ui/Button";
import { LinkButton } from "../../../components/ui/LinkButton";
import { TextLink } from "../../../components/ui/TextLink";
import { RatingBurger } from "../../../components/ui/RatingBurger";
import { Loading, NotFound } from "../../../components/ui/states";
import { Layout } from "../../../components/Layout";
import styles from "./reviewDetail.module.css";

// レビュー詳細(design/redesign/review-detail.html)。GET /reviews/:id は、そのバーガーの他のレビューを
// 返さない(一覧は user_id でしか絞れない)ため、常にこの 1 件だけを表示する。一覧の中の 1 項目ではなく、
// このレビュー自体が主役のページなので、「レビュー」「新しい順」のような一覧の見出しは付けず、囲みのカードにも
// 入れない(評価・本文を、ページの本文としてそのまま大きく見せる)。
// 「レビューを書く」も、can_review がこの応答にないため置かない(design/redesign の実装メモが挙げる案のとおり)。
export default function ReviewDetailPage() {
  const { t } = useTranslation();
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { user, isLoading: authLoading } = useAuth();
  const ratingRange = useRatingRange();
  // can_edit は閲覧者ごとに違うので、認証状態が確定してから取得する
  const { data: review, isLoading, error } = useReview(id, user?.id ?? null, { enabled: !authLoading });
  const { destroy } = useDeleteReview();
  const [isDeleting, setIsDeleting] = useState(false);

  if (isLoading || authLoading) {
    return (
      <Layout>
        <Loading />
      </Layout>
    );
  }
  if (error || !review) {
    return (
      <Layout>
        <NotFound action={<LinkButton to="/reviews">{t("reviews.detail.backToReviewsAction")}</LinkButton>} />
      </Layout>
    );
  }

  const handleDelete = async () => {
    if (!confirm(t("reviews.detail.deleteConfirm"))) return;
    setIsDeleting(true);
    try {
      await destroy(review.id);
      void navigate("/reviews");
    } finally {
      setIsDeleting(false);
    }
  };

  return (
    <Layout>
      <div className={styles.container}>
        <TextLink to="/reviews">{t("reviews.detail.backToReviews")}</TextLink>

        <div className={styles.hero}>
          {review.photoUrl ? (
            <img src={review.photoUrl} alt={t("reviews.photo.alt")} className={styles.heroPhoto} />
          ) : (
            <div className={styles.heroNoPhoto}>{t("shops.detail.noPhoto")}</div>
          )}
        </div>

        <div className={styles.narrow}>
          <section className={styles.titleSection}>
            <h1 className={styles.name}>
              {review.burger ? (
                <Link to={`/burgers/${review.burger.id}`} className={styles.nameLink}>
                  {review.burger.name}
                </Link>
              ) : (
                t("shops.detail.unknown")
              )}
            </h1>
            {review.shop && <p className={styles.shop}>{review.shop.name}</p>}
          </section>

          <section className={styles.review}>
            <div className={styles.who}>
              <span className={styles.avatar} aria-hidden="true">
                {review.user ? [...review.user.username][0] : "?"}
              </span>
              <div className={styles.grow}>
                <b>{review.user?.username ?? t("shops.detail.unknown")}</b>
                <time dateTime={review.createdAt}>{formatDate(review.createdAt)}</time>
                {review.visitedAt && (
                  <time dateTime={review.visitedAt}>{t("reviews.detail.visitedOn", { date: formatVisitedAt(review.visitedAt) })}</time>
                )}
              </div>
            </div>
            {ratingRange === undefined ? <b className={styles.ratingOnly}>{review.rating}</b> : <RatingBurger value={review.rating} max={ratingRange.max} size="md" variant="stepped" />}
            {review.comment && <p className={styles.comment}>{review.comment}</p>}
            {review.canEdit && (
              <div className={styles.actions}>
                <LinkButton to={`/reviews/${review.id}/edit`}>{t("reviews.detail.edit")}</LinkButton>
                <Button variant="danger" isLoading={isDeleting} onClick={() => void handleDelete()}>
                  {t("reviews.detail.delete")}
                </Button>
              </div>
            )}
          </section>
        </div>
      </div>
    </Layout>
  );
}
