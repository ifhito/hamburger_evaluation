import { Link, useParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useAuth } from "../../auth/AuthProvider";
import { useShopDetail } from "../hooks/useShops";
import { Button } from "../../../components/Button";
import { Layout } from "../../../components/Layout";
import styles from "./shopDetail.module.css";

export default function ShopDetailPage() {
  const { t } = useTranslation();
  const { id } = useParams<{ id: string }>();
  const { user } = useAuth();
  const { data: shop, isLoading, error } = useShopDetail(Number(id));

  if (isLoading) return <Layout><p className={styles.muted}>{t("shops.detail.loading")}</p></Layout>;
  if (error || !shop) return <Layout><p className={styles.muted}>{t("shops.detail.notFound")}</p></Layout>;

  return (
    <Layout title={shop.name}>
      <div className={styles.container}>
        {user && (
          <div>
            <Link to={`/reviews/new?shop_id=${shop.id}`}>
              <Button type="button">{t("shops.detail.writeReview")}</Button>
            </Link>
          </div>
        )}
        <section>
          <h2 className={styles.reviewsHeading}>{t("shops.detail.reviewsHeading")}</h2>
          {shop.reviews.length === 0 && (
            <p className={styles.muted}>{t("shops.detail.noReviews")}</p>
          )}
          <ul className={styles.reviewList}>
            {shop.reviews.map((review) => (
              <li key={review.id} className={styles.reviewCard}>
                <div className={styles.reviewCardHeader}>
                  <strong>{review.burger?.name ?? "—"}</strong>
                  <span>
                    {"★".repeat(review.rating)}
                    {"☆".repeat(5 - review.rating)}
                  </span>
                </div>
                <p className={styles.reviewComment}>{review.comment}</p>
                <small className={styles.reviewAuthor}>
                  by {review.user?.username ?? t("shops.detail.unknown")}
                </small>
              </li>
            ))}
          </ul>
        </section>
        <Link to="/shops" className={styles.backLink}>
          {t("shops.detail.backToShops")}
        </Link>
      </div>
    </Layout>
  );
}
