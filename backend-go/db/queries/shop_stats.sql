-- name: LockShopForStats :one
-- shop ごとの shop_stats の再計算を直列化する。再計算は「全件を読んでから上書きする」処理なので、
-- READ COMMITTED では、並行する 2 つのトランザクション(別のバーガーの依頼を処理する 2 つのワーカーなど)が、
-- それぞれ、相手のコミット前のレビューが欠けたスナップショットを読み、後の upsert が古い集計で上書きして
-- しまう可能性がある(lost update)。shops 行へのロックにより、2 つ目のトランザクションは、ここで 1 つ目が
-- コミットするまでブロックされ、その次の文は、その時点でコミット済みのレビューを見る。
-- FOR NO KEY UPDATE にしているのは、shop_stats・shops_burgers の外部キーの検査が取る FOR KEY SHARE を
-- 止めないためである(ショップの名前・状態の変更とは、短い間、互いに待つ)。
-- 複数のショップを 1 つのトランザクションでロックするときは、shop の id の昇順にすること。
-- ショップがないとき(消えた)は、行がない(ErrNoRows)。
SELECT id FROM shops
WHERE id = $1
FOR NO KEY UPDATE;

-- name: ListShopReviewFacts :many
-- 1 つの shop の集計の元になる kept な review。discard 済みの review と、discard 済みの user の review の
-- 両方を除外する(ListShopReviews、shop 詳細に出るレビューと同じ範囲)。バーガーは複数のショップに
-- 紐づけられるので、shops_burgers を通して、そのショップのすべてのバーガーのレビューをまとめる。
-- 集計の値(件数・平均・写真の選び方)は、ここでは求めない(domain.CalculateShopStat が求める)。user_id は、
-- 投稿者の過去の評価(レビューの重みに使う)を、ListReviewerRatings で読むための値である。
SELECT r.id, r.user_id, r.rating, r.photo_key, r.created_at
FROM reviews r
JOIN shops_burgers sb ON sb.burger_id = r.burger_id
JOIN users u ON u.id = r.user_id
WHERE sb.shop_id = $1
  AND r.discarded_at IS NULL
  AND u.discarded_at IS NULL
ORDER BY r.created_at, r.id;

-- name: ListBurgerShopIDs :many
-- バーガーが紐づくショップの id(重複なし)を、昇順に返す。バーガーの統計を計算し直したあと、
-- 影響するショップの集計も計算し直す対象を知るために使う。昇順は、複数のショップを続けてロックする
-- 再計算が、別の処理と逆の順序になって互いを待ち合わない(デッドロックしない)ための保証である。
SELECT sb.shop_id
FROM shops_burgers sb
WHERE sb.burger_id = $1
ORDER BY sb.shop_id;

-- name: UpsertShopStats :exec
INSERT INTO shop_stats (shop_id, review_count, average_rating, photo_key, calculated_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (shop_id) DO UPDATE
SET review_count = EXCLUDED.review_count,
    average_rating = EXCLUDED.average_rating,
    photo_key = EXCLUDED.photo_key,
    calculated_at = EXCLUDED.calculated_at;
