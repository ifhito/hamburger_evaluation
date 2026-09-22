-- name: UpsertShopStatsRecalcRequest :exec
-- ショップの集計の再計算を依頼する。すでに依頼があれば、version を進め、失敗の記録を消す(新しい書き込みが
-- 入ったので、最初からやり直す)。バーガーの統計の再計算と同じトランザクションから呼ぶ。
INSERT INTO shop_stats_recalc_requests (shop_id)
VALUES ($1)
ON CONFLICT (shop_id) DO UPDATE
SET version = nextval('shop_stats_recalc_requests_version_seq'),
    attempts = 0,
    next_attempt_at = NULL,
    last_error = NULL,
    updated_at = now();

-- name: ListDueShopStatsRecalcRequests :many
-- 再計算の時期が来ている依頼を、上限件数だけ返す。next_attempt_at が null か、now 以前で、
-- 失敗の回数が上限に達していないもの(上限に達したものは打ち切りで、行は残る)。
SELECT shop_id, version, attempts
FROM shop_stats_recalc_requests
WHERE attempts < sqlc.arg(max_attempts)::int
  AND (next_attempt_at IS NULL OR next_attempt_at <= sqlc.arg(now)::timestamptz)
ORDER BY next_attempt_at NULLS FIRST, shop_id
LIMIT sqlc.arg(batch)::int;

-- name: DeleteShopStatsRecalcRequestIfVersion :execrows
-- 再計算が終わった依頼を消す。取り出したときの version と一致する場合だけ消す(比較つき削除)。
-- 再計算の最中に新しい依頼が入ると version が進むので、その依頼は消えずに残り、次の再計算で最新の状態になる。
DELETE FROM shop_stats_recalc_requests
WHERE shop_id = $1 AND version = $2;

-- name: RecordShopStatsRecalcRequestFailure :execrows
-- 再計算の失敗を記録し、次の再試行の時刻を進める。取り出したときの version と一致する場合だけ更新する
-- (その間に新しい依頼が入っていれば、その依頼は最初からやり直すので触らない)。
UPDATE shop_stats_recalc_requests
SET attempts = attempts + 1,
    next_attempt_at = $3,
    last_error = $4,
    updated_at = now()
WHERE shop_id = $1 AND version = $2;
