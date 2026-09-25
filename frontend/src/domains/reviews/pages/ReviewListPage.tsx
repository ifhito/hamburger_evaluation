import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useReviews } from "../hooks/useReviews";
import { useAuth } from "../../auth/AuthProvider";
import { useRatingRange } from "../hooks/useRatingRange";
import { Alert } from "../../../components/ui/Alert";
import { Button } from "../../../components/ui/Button";
import { LinkButton } from "../../../components/ui/LinkButton";
import { EmptyState, Loading } from "../../../components/ui/states";
import { Layout } from "../../../components/Layout";
import { ReviewListCard } from "../components/ReviewListCard";
import styles from "./reviewList.module.css";

// レビュー一覧(design/redesign/reviews.html)。「レビューを書く」は、必ずショップを選ぶ必要があるため、
// ショップ一覧へ送る(サインインしている人だけに出す。API の項目ではなく、サインインの状態で決める)。
export default function ReviewListPage() {
  const { t } = useTranslation();
  const { user, isLoading: authLoading } = useAuth();
  const ratingRange = useRatingRange();
  const [sort, setSort] = useState("newest");
  const [keyword, setKeyword] = useState("");
  const [ratingFilter, setRatingFilter] = useState<number | undefined>(undefined);
  const { data: reviews, isLoading, error, hasNextPage, fetchNextPage, isFetchingNextPage } = useReviews(
    { rating: ratingFilter, keyword: keyword || undefined, sort },
    user?.id ?? null,
    { enabled: !authLoading },
  );
  const ratingOptions = ratingRange ? Array.from({ length: ratingRange.max - ratingRange.min + 1 }, (_, i) => ratingRange.max - i) : [];

  return (
    <Layout>
      <div className={styles.head}>
        <h1 className={styles.heading}>{t("reviews.list.title")}</h1>
      </div>
      <div className={styles.toolbar}>
        <select className={styles.sort} aria-label={t("lists.sortLabel")} value={sort} onChange={(e) => setSort(e.target.value)}>
          <option value="newest">{t("lists.newest")}</option>
          <option value="rating">{t("lists.rating")}</option>
        </select>
        <input
          aria-label={t("reviews.list.searchLabel")}
          className={styles.search}
          type="text"
          value={keyword}
          onChange={(e) => setKeyword(e.target.value)}
          placeholder={t("reviews.list.searchPlaceholder")}
        />
        <div className={styles.selectWrap}>
          <select
            aria-label={t("reviews.list.ratingFilterLabel")}
            className={styles.select}
            value={ratingFilter ?? ""}
            onChange={(e) => setRatingFilter(e.target.value ? Number(e.target.value) : undefined)}
          >
            <option value="">{t("reviews.list.allRatings")}</option>
            {ratingOptions.map((r) => (
              <option key={r} value={r}>
                {t("reviews.list.ratingOption", { value: r })}
              </option>
            ))}
          </select>
          <span className={styles.caret} aria-hidden="true">
            ▾
          </span>
        </div>
        {user && (
          <LinkButton variant="primary" to="/shops">
            {t("reviews.list.newReview")}
          </LinkButton>
        )}
      </div>

      {(isLoading || authLoading) && <Loading />}
      {error && <Alert message={t("reviews.list.loadError")} />}
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
    </Layout>
  );
}
