import type { ReactNode } from "react";
import { useParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useAuth } from "../../auth/AuthProvider";
import { useShopDetail } from "../hooks/useShops";
import { useRatingRange } from "../../reviews/hooks/useRatingRange";
import { Badge } from "../../../components/ui/Badge";
import { LinkButton } from "../../../components/ui/LinkButton";
import { TextLink } from "../../../components/ui/TextLink";
import { RatingSummary } from "../../../components/ui/RatingBurger";
import { EmptyState, Loading, NotFound } from "../../../components/ui/states";
import { Layout } from "../../../components/Layout";
import { ShopReviewCard } from "../components/ShopReviewCard";
import styles from "./shopDetail.module.css";

// ショップ詳細(design/redesign/shop-detail.html)。「レビューを書く」を出すかは backend が返す can_review だけで
// 決める(閲覧者が誰かを frontend が判断しない)。審査待ち・却下の札と理由は、返された状態をそのまま出す。
export default function ShopDetailPage() {
  const { t } = useTranslation();
  const { id } = useParams<{ id: string }>();
  const { user, isLoading: authLoading } = useAuth();
  const ratingRange = useRatingRange();
  // canReview は閲覧者ごとに違うので、認証状態が確定してから取得する
  const { data: shop, isLoading, error } = useShopDetail(id, user?.id ?? null, { enabled: !authLoading });

  if (isLoading || authLoading) {
    return (
      <Layout>
        <Loading />
      </Layout>
    );
  }
  if (error || !shop) {
    return (
      <Layout>
        <NotFound action={<LinkButton to="/shops">{t("shops.detail.backToShopsAction")}</LinkButton>} />
      </Layout>
    );
  }

  // 「レビューを書く」は can_review が true のときだけ。サインインしていなければ、サインインへの案内に替える
  // (デザインの案。can_review は匿名では常に false なので、その場合だけ出る)。
  const cta: ReactNode = shop.canReview ? (
    <LinkButton variant="primary" to={`/reviews/new?shop_id=${shop.id}`}>
      {t("shops.detail.writeReview")}
    </LinkButton>
  ) : !user ? (
    <TextLink to="/signin">{t("shops.detail.signInToReview")}</TextLink>
  ) : null;

  return (
    <Layout>
      <div className={styles.container}>
        <TextLink to="/shops">{t("shops.detail.backToShops")}</TextLink>

        <div className={styles.hero}>
          {shop.photoUrl ? (
            <img src={shop.photoUrl} alt="" className={styles.heroPhoto} />
          ) : (
            <div className={styles.heroNoPhoto}>{t("shops.detail.noPhotoYet")}</div>
          )}
        </div>

        <section className={styles.titleSection}>
          {shop.status !== "active" && <Badge>{t(`shops.statusBadge.${shop.status}`)}</Badge>}
          {shop.closedAt && <Badge>{t("shops.closedBadge")}</Badge>}
          <h1 className={styles.name}>{shop.name}</h1>
        </section>

        {shop.status === "pending" && (
          <div className={styles.notice}>
            <b>{t("shops.detail.pendingNoticeTitle")}</b>
            <span>{t("shops.detail.pendingNoticeBody")}</span>
          </div>
        )}
        {shop.status === "rejected" && (
          <div className={styles.notice}>
            <b>{t("shops.detail.rejectedNoticeTitle")}</b>
            {shop.moderationNote && <span>{t("shops.detail.rejectedNoticeReason", { reason: shop.moderationNote })}</span>}
          </div>
        )}

        {shop.status !== "rejected" &&
          (shop.reviewCount > 0 ? (
            <>
              <RatingSummary value={shop.averageRating} max={ratingRange?.max} count={shop.reviewCount} action={cta} />
              <div className={styles.sectionHead}>
                <h2 className={styles.heading}>{t("shops.detail.reviewsHeading")}</h2>
                <span className={styles.count}>{t("shops.detail.newestFirst")}</span>
              </div>
              <div className={styles.grid}>
                {shop.reviews.map((review) => (
                  <ShopReviewCard key={review.id} review={review} ratingMax={ratingRange?.max} />
                ))}
              </div>
            </>
          ) : (
            <EmptyState action={cta} />
          ))}

        {shop.status === "rejected" && (
          <p className={styles.backBelow}>
            <TextLink to="/shops">{t("shops.detail.backToShops")}</TextLink>
          </p>
        )}
      </div>
    </Layout>
  );
}
