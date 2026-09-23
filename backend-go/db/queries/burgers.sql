-- name: CreateBurger :one
INSERT INTO burgers (name)
VALUES ($1)
RETURNING *;

-- name: GetBurger :one
SELECT * FROM burgers
WHERE id = $1;

-- name: ListBurgers :many
SELECT * FROM burgers
ORDER BY created_at, id
LIMIT $1 OFFSET $2;

-- name: UpdateBurger :one
UPDATE burgers
SET name = $2,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteBurger :exec
DELETE FROM burgers
WHERE id = $1;

-- name: ListBurgerRankings :many
-- GET /burgers 向けの公開ランキング。対象は、review が 1 件以上ある(burger_stats を持つ)、かつ
-- 少なくとも 1 つの active な shop(status 1)に紐づく burger で、weighted_score の降順(同値は
-- id の昇順で決着)で返す。1 つの burger が複数の active な shop に紐づくときの代表 shop は、
-- 作成の古い順(created_at 昇順、同時刻は id 昇順)の先頭を採る(DISTINCT ON)。これは、
-- domain.ReviewShopFor が匿名の viewer に対して選ぶショップ(見えるショップの先頭)と同じ選び方
-- である(この一覧には viewer がなく、常に匿名と同じ扱いになるため)。次のページの有無を知るために
-- limit+1 件を取得する。
WITH representative_shop AS (
    SELECT DISTINCT ON (sb.burger_id)
        sb.burger_id, s.id AS shop_id, s.name AS shop_name
    FROM shops_burgers sb
    JOIN shops s ON s.id = sb.shop_id
    WHERE s.status = 1
    ORDER BY sb.burger_id, s.created_at, s.id
)
SELECT b.id, b.name,
       rs.shop_id, rs.shop_name,
       bs.average_rating, bs.weighted_score, bs.review_count
FROM burgers b
JOIN burger_stats bs ON bs.burger_id = b.id
JOIN representative_shop rs ON rs.burger_id = b.id
ORDER BY bs.weighted_score DESC, b.id ASC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);
