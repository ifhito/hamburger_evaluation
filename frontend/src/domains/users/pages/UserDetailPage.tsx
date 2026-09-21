import { useParams, Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useAuth } from "../../auth/AuthProvider";
import { isValidUserId, useUser } from "../hooks/useUser";
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

  const userId = Number(id);
  // /users/abc(NaN)や 0 以下・安全でない整数など不正な id では、user も reviews も取得しない
  const isValidId = isValidUserId(userId);
  // 認証状態の復元前は authUser が null でも token は localStorage にあり得る。閲覧者が確定してから取得する
  const {
    data: user,
    isLoading: userLoading,
    error: userError,
  } = useUser(userId, authUser?.id ?? null, { enabled: !authLoading });
  // 不正な id のまま呼ぶと user_id=NaN が 422 になり、レビューの読み込みエラーが余計に出る。
  // かといって user_id を外して呼ぶと全件のフィードが他人のレビューとして出てしまうため、不正な id では取得自体を止める
  const {
    data: userReviews,
    isLoading: reviewsLoading,
    error: reviewsError,
    hasNextPage,
    fetchNextPage,
    isFetchingNextPage,
  } = useReviews({ userId }, { enabled: isValidId });

  return (
    <Layout title={user ? t("users.detail.namedProfile", { name: user.username }) : t("users.detail.profileTitle")}>
      {(userLoading || reviewsLoading) && (
        <p className={styles.muted}>{t("users.detail.loading")}</p>
      )}
      {(!isValidId || userError) && <ErrorMessage message={t("users.detail.loadError")} />}

      {user && (
        <div className={styles.profile}>
          <div className={styles.profileCard}>
            <h2 className={styles.username}>{user.username}</h2>
            {/* email は API が本人の閲覧時だけ返す。閲覧者の比較ではなく、API の返却有無で出し分ける */}
            {user.email && <p className={styles.email}>{user.email}</p>}
          </div>
          {user.canEdit && (
            <Link to={`/users/${userId}/edit`} className={styles.editLink}>
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
