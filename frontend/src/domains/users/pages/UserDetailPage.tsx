import { useParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useAuth } from "../../auth/AuthProvider";
import { useUser } from "../hooks/useUser";
import { useReviews } from "../../reviews/hooks/useReviews";
import { useRatingRange } from "../../reviews/hooks/useRatingRange";
import { Alert } from "../../../components/ui/Alert";
import { Button } from "../../../components/ui/Button";
import { LinkButton } from "../../../components/ui/LinkButton";
import { EmptyState, Loading } from "../../../components/ui/states";
import { Layout } from "../../../components/Layout";
import { ShareLinkButton } from "../components/ShareLinkButton";
import { ProfileReviewCard } from "../components/ProfileReviewCard";
import { ConnectedApps } from "../../oauth/components/ConnectedApps";
import { GoogleConnection } from "../../auth/components/GoogleConnection";
import styles from "./userDetail.module.css";

export default function UserDetailPage() {
  const { t } = useTranslation();
  const { id } = useParams<{ id: string }>();
  const { user: authUser, isLoading: authLoading } = useAuth();
  const ratingRange = useRatingRange();

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
  } = useReviews({ userId: id }, authUser?.id ?? null, { enabled: id !== undefined && user !== undefined });

  return (
    // 名前は、プロフィールの見出し(h1)として、下の本体に出す。取得できるまでの間だけ、画面の見出しを出す
    <Layout title={user ? undefined : t("users.detail.profileTitle")}>
      {userLoading && <Loading />}
      {userError && <Alert message={t("users.detail.loadError")} />}

      {user && (
        <section className={styles.profile}>
          <span className={styles.avatar} aria-hidden="true">
            {[...user.username][0]}
          </span>
          <div className={styles.main}>
            <h1 className={styles.username}>{user.username}</h1>
            {/* 自己紹介文は利用者が自由に書くので、HTML として解釈せず、改行だけを保った文字として描画する */}
            {user.bio && <p className={styles.bio}>{user.bio}</p>}
            <div className={styles.actions}>
              <ShareLinkButton userId={user.id} />
              {user.canEdit && <><LinkButton to={`/users/${user.id}/edit`}>{t("users.detail.editProfile")}</LinkButton><LinkButton to="/signout">{t("nav.signout")}</LinkButton></>}
            </div>
          </div>
        </section>
      )}

      {/* Google の連携と、許可したアプリの一覧は、本人のプロフィールにだけ出す(本人かどうかは backend が返す canEdit で決める) */}
      {user?.canEdit && authUser && <GoogleConnection viewerId={authUser.id} />}
      {user?.canEdit && authUser && <ConnectedApps viewerId={authUser.id} />}

      {user && (
        <section>
          <div className={styles.sectionHead}>
            <h2 className={styles.heading}>{t("users.detail.reviewsHeading")}</h2>
            {userReviews && userReviews.length > 0 && <span className={styles.count}>{t("users.detail.newestFirst")}</span>}
          </div>
          {reviewsLoading && <Loading />}
          {reviewsError && <Alert message={t("users.detail.reviewsLoadError")} />}
          {userReviews && userReviews.length === 0 && (user.canEdit ? <EmptyState title={t("users.detail.emptyTitle")} /> : <p className={styles.muted}>{t("users.detail.noReviews")}</p>)}
          <div className={styles.reviewList}>
            {userReviews?.map((review) => (
              <ProfileReviewCard key={review.id} review={review} ratingMax={ratingRange?.max} />
            ))}
          </div>
          {hasNextPage && (
            <div className={styles.loadMore}>
              <Button type="button" variant="secondary" isLoading={isFetchingNextPage} onClick={fetchNextPage}>
                {t("users.detail.loadMore")}
              </Button>
            </div>
          )}
        </section>
      )}
    </Layout>
  );
}
