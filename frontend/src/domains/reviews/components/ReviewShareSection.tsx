import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { copyToClipboard } from "../../../lib/clipboard";
import { Button } from "../../../components/ui/Button";
import styles from "./reviewShareSection.module.css";

const COPY_FEEDBACK_RESET_MS = 1800;
const SHARE_COMMENT_PREVIEW_CHARS = 80;
// X の投稿上限(280 ウェイト)から URL 分(24 ウェイト)と余白を引いた本文の予算。
const X_TEXT_WEIGHT_BUDGET = 248;
const X_SUBJECT_WITH_COMMENT_MAX_WEIGHT = 140;

interface Props {
  reviewId: string;
  burgerName: string | null;
  shopName: string | null;
  rating: number;
  ratingMax?: number;
  comment: string | null;
  // 投稿完了直後に遷移してきたときだけ上部の強調バナーとして表示する。
  highlighted?: boolean;
  onDismissHighlight?: () => void;
}

function codePointWeight(ch: string): number {
  return (ch.codePointAt(0) ?? 0) <= 127 ? 1 : 2;
}

function measureXWeight(text: string): number {
  let total = 0;
  for (const ch of text) total += codePointWeight(ch);
  return total;
}

function truncateForShare(text: string, maxWeight: number, maxChars?: number): string {
  if (!text || maxWeight <= 0) return "";
  const chars = [...text];
  const withinCharCap = maxChars === undefined || chars.length <= maxChars;
  if (withinCharCap && measureXWeight(text) <= maxWeight) return text;

  const ellipsis = "…";
  const budget = maxWeight - measureXWeight(ellipsis);
  if (budget <= 0) return "";

  let usedWeight = 0;
  let count = 0;
  const kept: string[] = [];
  for (const ch of chars) {
    if (maxChars !== undefined && count >= maxChars) break;
    const w = codePointWeight(ch);
    if (usedWeight + w > budget) break;
    kept.push(ch);
    usedWeight += w;
    count += 1;
  }
  return kept.length > 0 ? `${kept.join("")}${ellipsis}` : "";
}

// レビューの共有導線(X の Web Intent・端末標準の共有シート・URL コピー)。
// OGP タグは Cloudflare Worker が /reviews/:id に対して返すため、公開 URL を共有するだけで写真つきカードが出る。
export function ReviewShareSection({
  reviewId,
  burgerName,
  shopName,
  rating,
  ratingMax,
  comment,
  highlighted = false,
  onDismissHighlight,
}: Props) {
  const { t } = useTranslation();
  const [copyState, setCopyState] = useState<{ forReviewId: string; status: "idle" | "copied" | "manual" }>({
    forReviewId: reviewId,
    status: "idle",
  });
  const state = copyState.forReviewId === reviewId ? copyState.status : "idle";
  const shareUrl = `${window.location.origin}/reviews/${encodeURIComponent(reviewId)}`;

  useEffect(() => {
    if (state !== "copied") return;
    const timer = window.setTimeout(() => setCopyState({ forReviewId: reviewId, status: "idle" }), COPY_FEEDBACK_RESET_MS);
    return () => window.clearTimeout(timer);
  }, [state, reviewId]);

  const cleanBurger = burgerName?.trim() ?? "";
  const cleanShop = shopName?.trim() ?? "";
  let rawSubject = `🍔 ${t("reviews.share.fallbackSubject")}`;
  if (cleanBurger && cleanShop) {
    rawSubject = `🍔 ${cleanBurger}（${cleanShop}）`;
  } else if (cleanBurger) {
    rawSubject = `🍔 ${cleanBurger}`;
  } else if (cleanShop) {
    rawSubject = `🍔 ${cleanShop}`;
  }

  const ratingLabel = ratingMax !== undefined ? `★${rating}/${ratingMax}` : `★${rating}`;
  const ratingSuffix = ` ${ratingLabel}`;
  const hashtagLine = "#BurgerStack";
  const normalizedComment = comment ? comment.replace(/\s+/g, " ").trim() : "";

  const maxSubjectWeight = X_TEXT_WEIGHT_BUDGET - measureXWeight(ratingSuffix) - 1 - measureXWeight(hashtagLine);
  const subjectBudget = normalizedComment
    ? Math.min(X_SUBJECT_WITH_COMMENT_MAX_WEIGHT, maxSubjectWeight)
    : maxSubjectWeight;
  const subject = truncateForShare(rawSubject, subjectBudget);
  const headlineLine = `${subject}${ratingSuffix}`;
  const remainingCommentWeight =
    X_TEXT_WEIGHT_BUDGET - measureXWeight(headlineLine) - 1 - measureXWeight(hashtagLine) - 1;
  const snippet = normalizedComment
    ? truncateForShare(normalizedComment, remainingCommentWeight, SHARE_COMMENT_PREVIEW_CHARS)
    : "";

  const shareLines = [headlineLine, ...(snippet ? [snippet] : []), hashtagLine];
  const shareText = shareLines.join("\n");
  const xIntentParams = new URLSearchParams({ text: shareText, url: shareUrl });
  const xIntentUrl = `https://x.com/intent/post?${xIntentParams.toString()}`;

  const supportsNativeShare = typeof navigator !== "undefined" && typeof navigator.share === "function";

  const handleNativeShare = async () => {
    if (typeof navigator === "undefined" || typeof navigator.share !== "function") return;
    try {
      await navigator.share({
        title: `${headlineLine} | BurgerStack`,
        text: shareText,
        url: shareUrl,
      });
    } catch {
      // 利用者が共有シートを閉じた場合(AbortError)などはエラー表示にしない。
    }
  };

  const handleCopy = async () => {
    setCopyState({ forReviewId: reviewId, status: (await copyToClipboard(shareUrl)) ? "copied" : "manual" });
  };

  const manualInputId = highlighted ? "review-share-link-banner" : "review-share-link";

  const actionButtons = (
    <>
      <div className={styles.actions}>
        <a href={xIntentUrl} target="_blank" rel="noopener noreferrer" className={styles.xButton}>
          <span className={styles.xMark} aria-hidden="true">
            𝕏
          </span>
          <span>{t("reviews.share.postOnX")}</span>
        </a>
        {supportsNativeShare && (
          <Button type="button" variant="secondary" onClick={() => void handleNativeShare()}>
            {t("reviews.share.nativeShare")}
          </Button>
        )}
        <Button type="button" variant="secondary" aria-live="polite" onClick={() => void handleCopy()}>
          {state === "copied" ? t("reviews.share.linkCopied") : t("reviews.share.copyLink")}
        </Button>
      </div>
      {state === "manual" && (
        <div className={styles.manual}>
          <label htmlFor={manualInputId} className={styles.manualLabel}>
            {t("reviews.share.copyLinkManually")}
          </label>
          <input
            id={manualInputId}
            className={styles.linkInput}
            readOnly
            value={shareUrl}
            onFocus={(e) => e.currentTarget.select()}
          />
        </div>
      )}
    </>
  );

  if (highlighted) {
    return (
      <section className={styles.banner} aria-label={t("reviews.share.postedTitle")}>
        <i className={styles.band} aria-hidden="true" />
        <div className={styles.bannerHeader}>
          <div className={styles.bannerText}>
            <b className={styles.bannerTitle}>{t("reviews.share.postedTitle")}</b>
            <p className={styles.bannerBody}>{t("reviews.share.postedBody")}</p>
          </div>
          {onDismissHighlight && (
            <button type="button" className={styles.dismissBtn} onClick={onDismissHighlight}>
              {t("reviews.share.dismissNotice")}
            </button>
          )}
        </div>
        {actionButtons}
      </section>
    );
  }

  return <div className={styles.inlineBar}>{actionButtons}</div>;
}
