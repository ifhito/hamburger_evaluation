-- name: UpsertBurgerStatsRecalcRequest :exec
-- バーガーの統計の再計算を依頼する。すでに依頼があれば、version を進め、失敗の記録を消す
-- (新しい書き込みが入ったので、最初からやり直す)。書き込みと同じトランザクションから呼ぶ。
INSERT INTO burger_stats_recalc_requests (burger_id)
VALUES ($1)
ON CONFLICT (burger_id) DO UPDATE
SET version = nextval('burger_stats_recalc_requests_version_seq'),
    attempts = 0,
    next_attempt_at = NULL,
    last_error = NULL,
    updated_at = now();

-- name: ListDueBurgerStatsRecalcRequests :many
-- 再計算の時期が来ている依頼を、上限件数だけ返す。next_attempt_at が null か、now 以前で、
-- 失敗の回数が上限に達していないもの(上限に達したものは打ち切りで、行は残る)。
SELECT burger_id, version, attempts
FROM burger_stats_recalc_requests
WHERE attempts < sqlc.arg(max_attempts)::int
  AND (next_attempt_at IS NULL OR next_attempt_at <= sqlc.arg(now)::timestamptz)
ORDER BY next_attempt_at NULLS FIRST, burger_id
LIMIT sqlc.arg(batch)::int;

-- name: DeleteBurgerStatsRecalcRequestIfVersion :execrows
-- 再計算が終わった依頼を消す。取り出したときの version と一致する場合だけ消す(比較つき削除)。
-- 再計算の最中に新しい書き込みが入ると version が進むので、その依頼は消えずに残り、
-- 次の再計算で最新の状態になる。
DELETE FROM burger_stats_recalc_requests
WHERE burger_id = $1 AND version = $2;

-- name: RecordBurgerStatsRecalcRequestFailure :execrows
-- 再計算の失敗を記録し、次の再試行の時刻を進める。取り出したときの version と一致する場合だけ
-- 更新する(その間に新しい書き込みが入っていれば、その依頼は最初からやり直すので触らない)。
UPDATE burger_stats_recalc_requests
SET attempts = attempts + 1,
    next_attempt_at = $3,
    last_error = $4,
    updated_at = now()
WHERE burger_id = $1 AND version = $2;
