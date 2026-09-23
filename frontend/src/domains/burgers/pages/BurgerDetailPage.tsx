import { Fragment } from "react";
import { useParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useAuth } from "../../auth/AuthProvider";
import { useBurger } from "../hooks/useBurger";
import { useReviews } from "../../reviews/hooks/useReviews";
import { useRatingRange } from "../../reviews/hooks/useRatingRange";
import { ReviewListCard } from "../../reviews/components/ReviewListCard";
import { Alert } from "../../../components/ui/Alert";
import { Button } from "../../../components/ui/Button";
import { LinkButton } from "../../../components/ui/LinkButton";
import { TextLink } from "../../../components/ui/TextLink";
import { RatingSummary } from "../../../components/ui/RatingBurger";
import { EmptyState, Loading, NotFound } from "../../../components/ui/states";
import { Layout } from "../../../components/Layout";
import styles from "./burgerDetail.module.css";

// バーガー詳細(design/redesign/burger-detail.html)。GET /burgers/:id には can_review のような閲覧者ごとの値が
// ないため、「レビューを書く」は置かない(review-detail.html と同じ理由)。レビュー一覧は ReviewListCard を
// そのまま使う(バーガー名がこの画面の見出しと重なるが、issue の仕様どおり再利用する)。
// reviewCount は統計の再計算(非同期)由来で、useReviews の取得(同期)と前後することがあるため、
// レビュー一覧の表示は reviews 側の状態だけで決め、RatingSummary は reviewCount の有無だけで別に出し分ける。
export default function BurgerDetailPage() {
  const { t } = useTranslation();
  const { id } = useParams<{ id: string }>();
  const { user, isLoading: authLoading } = useAuth();
  const ratingRange = useRatingRange();
  // shops の一覧が閲覧者ごとに違うので、認証状態が確定してから取得する
  const { data: burger, isLoading, error } = useBurger(id, user?.id ?? null, { enabled: !authLoading });
  const {
    data: reviews,
    isLoading: reviewsLoading,
    error: reviewsError,
    hasNextPage,
    fetchNextPage,
    isFetchingNextPage,
  } = useReviews({ burgerId: id }, { enabled: burger !== undefined });

  if (isLoading || authLoading) {
    return (
      <Layout>
        <Loading />
      </Layout>
    );
  }
  if (error || !burger) {
    return (
      <Layout>
        <NotFound action={<LinkButton to="/reviews">{t("reviews.detail.backToReviewsAction")}</LinkButton>} />
      </Layout>
    );
  }

  return (
    <Layout>
      <div className={styles.narrow}>
        <TextLink to="/reviews">{t("reviews.detail.backToReviews")}</TextLink>

        <section className={styles.titleSection}>
          <h1 className={styles.name}>{burger.name}</h1>
          {burger.shops.length > 0 && (
            <p className={styles.shop}>
              {t("burgers.detail.servedAt")}{" "}
              {burger.shops.map((shop, i) => (
                <Fragment key={shop.id}>
                  {i > 0 && " · "}
                  <TextLink to={`/shops/${shop.id}`}>{shop.name}</TextLink>
                </Fragment>
              ))}
            </p>
          )}
        </section>

        {burger.reviewCount !== null && (
          <RatingSummary value={burger.averageRating} max={ratingRange?.max} count={burger.reviewCount} />
        )}

        <div className={styles.sectionHead}>
          <h2 className={styles.heading}>{t("reviews.detail.reviewsHeading")}</h2>
          {reviews && reviews.length > 0 && <span className={styles.count}>{t("reviews.detail.newestFirst")}</span>}
        </div>
        {reviewsLoading && <Loading />}
        {reviewsError && <Alert message={t("reviews.list.loadError")} />}
        {reviews && reviews.length === 0 && <EmptyState />}
        <div className={styles.grid}>
          {reviews?.map((review) => (
            <ReviewListCard key={review.id} review={review} ratingMax={ratingRange?.max} />
          ))}
        </div>
        {hasNextPage && (
          <div className={styles.loadMore}>
            <Button type="button" variant="secondary" isLoading={isFetchingNextPage} onClick={fetchNextPage}>
              {t("reviews.list.loadMore")}
            </Button>
          </div>
        )}
      </div>
    </Layout>
  );
}
