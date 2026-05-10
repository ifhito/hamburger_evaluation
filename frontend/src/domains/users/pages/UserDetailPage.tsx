import { useParams, Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useAuth } from "../../auth/AuthProvider";
import { useUsers } from "../hooks/useUsers";
import { useReviews } from "../../reviews/hooks/useReviews";
import { formatDate } from "../../../lib/date";
import { ErrorMessage } from "../../../components/ErrorMessage";
import { Layout } from "../../../components/Layout";
import styles from "./userDetail.module.css";

export default function UserDetailPage() {
  const { t } = useTranslation();
  const { id } = useParams<{ id: string }>();
  const { user: authUser } = useAuth();
  const { data: users, isLoading: usersLoading, error: usersError } = useUsers();
  const { data: allReviews, isLoading: reviewsLoading } = useReviews();

  const userId = Number(id);
  const user = users?.find((u) => u.id === userId);
  const userReviews = allReviews?.filter((r) => r.user?.id === userId);
  const isOwner = authUser?.id === userId;

  return (
    <Layout title={user ? t("users.detail.namedProfile", { name: user.username }) : t("users.detail.profileTitle")}>
      {(usersLoading || reviewsLoading) && (
        <p className={styles.muted}>{t("users.detail.loading")}</p>
      )}
      {usersError && <ErrorMessage message={t("users.detail.loadError")} />}

      {user && (
        <div className={styles.profile}>
          <div className={styles.profileCard}>
            <h2 className={styles.username}>{user.username}</h2>
            <p className={styles.email}>{user.email}</p>
          </div>
          {isOwner && (
            <Link to={`/users/${userId}/edit`} className={styles.editLink}>
              {t("users.detail.editProfile")}
            </Link>
          )}
        </div>
      )}

      <h2 className={styles.reviewsHeading}>{t("users.detail.reviewsHeading")}</h2>
      {userReviews && userReviews.length === 0 && (
        <p className={styles.muted}>{t("users.detail.noReviews")}</p>
      )}
      <div className={styles.reviewList}>
        {userReviews?.map((review) => (
          <div key={review.id} className={styles.reviewCard}>
            <div className={styles.reviewHeader}>
              <span>
                {"★".repeat(review.rating)}
                {"☆".repeat(5 - review.rating)}
              </span>
              <span className={styles.reviewDate}>{formatDate(review.createdAt)}</span>
            </div>
            <p className={styles.reviewComment}>{review.comment}</p>
            <Link to={`/reviews/${review.id}`} className={styles.reviewComment}>
              {t("users.detail.viewLink")}
            </Link>
          </div>
        ))}
      </div>
    </Layout>
  );
}
