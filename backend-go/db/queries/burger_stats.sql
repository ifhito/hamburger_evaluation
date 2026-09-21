-- name: UpsertBurgerStats :one
INSERT INTO burger_stats (burger_id, review_count, average_rating, weighted_score, confidence, calculated_at)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (burger_id) DO UPDATE
SET review_count = EXCLUDED.review_count,
    average_rating = EXCLUDED.average_rating,
    weighted_score = EXCLUDED.weighted_score,
    confidence = EXCLUDED.confidence,
    calculated_at = EXCLUDED.calculated_at
RETURNING *;

-- name: LockBurgerForStats :one
-- burger ごとの burger_stats の再計算を直列化する。再計算は
-- 「全件を読んでから上書きする」処理なので、READ COMMITTED では、並行する
-- 2 つのトランザクションがそれぞれ、相手のコミット前の review が欠けた
-- スナップショットを読み、後の upsert が古い件数で統計を上書きしてしまう
-- 可能性がある（lost update）。burgers 行への FOR UPDATE により、
-- 2 つ目のトランザクションはここで 1 つ目がコミットするまでブロックされる。
-- その次の文は、その時点でコミット済みの review を見る。
SELECT id FROM burgers
WHERE id = $1
FOR UPDATE;

-- name: ListBurgerReviewFacts :many
-- 1 つの burger の統計の元になる kept な review。discard 済みの review と、
-- discard 済みの user の review の両方を除外する（issue #15 R4/AC4。story の
-- 決定に従い、Rails の burger.reviews.kept よりも意図的に厳しくしている）。
-- active な shop によるフィルタは行わない。統計はすべての kept な review を
-- 集計する（Rails と同様）。
SELECT r.rating, r.created_at, r.user_id
FROM reviews r
JOIN users u ON u.id = r.user_id
WHERE r.burger_id = $1
  AND r.discarded_at IS NULL
  AND u.discarded_at IS NULL
ORDER BY r.created_at, r.id;

-- name: ListReviewerRatings :many
-- reviewer trust の履歴：各 reviewer が「すべての」burger にわたってつけた
-- kept な rating（Rails の user.reviews.kept に対応する）。
SELECT r.user_id, r.rating
FROM reviews r
WHERE r.user_id = ANY(sqlc.arg(user_ids)::uuid[])
  AND r.discarded_at IS NULL
ORDER BY r.created_at, r.id;

-- name: GetBurgerStats :one
SELECT * FROM burger_stats
WHERE burger_id = $1;

-- name: DeleteBurgerStats :exec
DELETE FROM burger_stats
WHERE burger_id = $1;
