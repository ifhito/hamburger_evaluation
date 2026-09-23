import { useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useAuth } from "../../auth/AuthProvider";
import { useReview } from "../hooks/useReview";
import { useDeleteReview } from "../hooks/useReviewMutations";
import { useRatingRange } from "../hooks/useRatingRange";
import { formatDate } from "../../../lib/date";
import { Button } from "../../../components/ui/Button";
import { Card } from "../../../components/ui/Card";
import { LinkButton } from "../../../components/ui/LinkButton";
import { TextLink } from "../../../components/ui/TextLink";
import { RatingBurger } from "../../../components/ui/RatingBurger";
import { Loading, NotFound } from "../../../components/ui/states";
import { Layout } from "../../../components/Layout";
import styles from "./reviewDetail.module.css";

// レビュー詳細(design/redesign/review-detail.html)。GET /reviews/:id は、そのバーガーの他のレビューを
// 返さない(一覧は user_id でしか絞れない)ため、表示中の 1 件だけを並べる(デザインの見本にある、
// 他のレビューの行は出さない。「表示中のレビュー」の札も、比べる相手がいないため出さない)。
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
            <h1 className={styles.name}>{review.burger?.name ?? t("shops.detail.unknown")}</h1>
            {review.shop && <p className={styles.shop}>{review.shop.name}</p>}
          </section>

          <div className={styles.sectionHead}>
            <h2 className={styles.heading}>{t("reviews.detail.reviewsHeading")}</h2>
            <span className={styles.count}>{t("reviews.detail.newestFirst")}</span>
          </div>

          <Card current className={styles.item}>
            <div className={styles.who}>
              <span className={styles.avatar} aria-hidden="true">
                {review.user ? [...review.user.username][0] : "?"}
              </span>
              <div className={styles.grow}>
                <b>{review.user?.username ?? t("shops.detail.unknown")}</b>
                <time dateTime={review.createdAt}>{formatDate(review.createdAt)}</time>
              </div>
            </div>
            {ratingRange === undefined ? <b className={styles.ratingOnly}>{review.rating}</b> : <RatingBurger value={review.rating} max={ratingRange.max} size="sm" variant="stepped" />}
            <p className={styles.comment}>{review.comment}</p>
            {review.canEdit && (
              <div className={styles.actions}>
                <LinkButton to={`/reviews/${review.id}/edit`}>{t("reviews.detail.edit")}</LinkButton>
                <Button variant="danger" isLoading={isDeleting} onClick={() => void handleDelete()}>
                  {t("reviews.detail.delete")}
                </Button>
              </div>
            )}
          </Card>
        </div>
      </div>
    </Layout>
  );
}
