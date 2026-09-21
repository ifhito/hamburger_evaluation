import { useParams, Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useAuth } from "../../auth/AuthProvider";
import { useUser } from "../hooks/useUser";
import { useReviews } from "../../reviews/hooks/useReviews";
import { formatDate } from "../../../lib/date";
import { formatRating } from "../../../lib/rating";
import { Button } from "../../../components/Button";
import { ErrorMessage } from "../../../components/ErrorMessage";
import { Layout } from "../../../components/Layout";
import styles from "./userDetail.module.css";

export default function UserDetailPage() {
  const { t } = useTranslation();
  const { id } = useParams<{ id: string }>();
  const { user: authUser, isLoading: authLoading } = useAuth();

  // 認証状態の復元前は authUser が null でも token は localStorage にあり得る。閲覧者が確定してから取得する
  const {
    data: user,
    isLoading: userLoading,
    error: userError,
  } = useUser(id, authUser?.id ?? null, { enabled: !authLoading });
  // id の形式の判定は持たない(不正な id は API の 404 でエラー表示になる)。
  // user_id を外して呼ぶと全件のフィードが他人のレビューとして出てしまうため、ユーザーが取得できてから呼ぶ
  const {
    data: userReviews,
    isLoading: reviewsLoading,
    error: reviewsError,
    hasNextPage,
    fetchNextPage,
    isFetchingNextPage,
  } = useReviews({ userId: id }, { enabled: id !== undefined && user !== undefined });
  const isOwner = user !== undefined && authUser?.id === user.id;

  return (
    <Layout title={user ? t("users.detail.namedProfile", { name: user.username }) : t("users.detail.profileTitle")}>
      {(userLoading || reviewsLoading) && (
        <p className={styles.muted}>{t("users.detail.loading")}</p>
      )}
      {userError && <ErrorMessage message={t("users.detail.loadError")} />}

      {user && (
        <div className={styles.profile}>
          <div className={styles.profileCard}>
            <h2 className={styles.username}>{user.username}</h2>
            {/* email は API が本人の閲覧時だけ返す。isOwner ではなく API の返却有無で出し分ける */}
            {user.email && <p className={styles.email}>{user.email}</p>}
          </div>
          {isOwner && (
            <Link to={`/users/${user.id}/edit`} className={styles.editLink}>
              {t("users.detail.editProfile")}
            </Link>
          )}
        </div>
      )}

      <h2 className={styles.reviewsHeading}>{t("users.detail.reviewsHeading")}</h2>
      {reviewsError && <ErrorMessage message={t("users.detail.reviewsLoadError")} />}
      {userReviews && userReviews.length === 0 && (
        <p className={styles.muted}>{t("users.detail.noReviews")}</p>
      )}
      <div className={styles.reviewList}>
        {userReviews?.map((review) => (
          <div key={review.id} className={styles.reviewCard}>
            <div className={styles.reviewHeader}>
              <span>{formatRating(review.rating)}</span>
              <span className={styles.reviewDate}>{formatDate(review.createdAt)}</span>
            </div>
            <p className={styles.reviewComment}>{review.comment}</p>
            <Link to={`/reviews/${review.id}`} className={styles.reviewComment}>
              {t("users.detail.viewLink")}
            </Link>
          </div>
        ))}
      </div>
      {hasNextPage && (
        <div className={styles.loadMore}>
          <Button type="button" variant="secondary" isLoading={isFetchingNextPage} onClick={fetchNextPage}>
            {t("users.detail.loadMore")}
          </Button>
        </div>
      )}
    </Layout>
  );
}
