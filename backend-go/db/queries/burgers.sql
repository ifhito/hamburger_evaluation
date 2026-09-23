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

-- name: GetBurgerWithStats :one
-- burger 1 件を、保存された統計(burger_stats。LEFT JOIN。まだ計算されていない、または削除で 0 件に戻った
-- burger は review_count/average_rating/weighted_score が NULL または 0)とともに返す。存在しない id は
-- 0 行になる(呼び出し側が domain.ErrBurgerNotFound に対応付ける)。
SELECT b.id, b.name,
       bs.review_count, bs.average_rating, bs.weighted_score
FROM burgers b
LEFT JOIN burger_stats bs ON bs.burger_id = b.id
WHERE b.id = $1;
