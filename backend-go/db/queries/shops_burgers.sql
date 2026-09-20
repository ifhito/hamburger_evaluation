-- name: CreateShopBurger :exec
INSERT INTO shops_burgers (shop_id, burger_id)
VALUES ($1, $2);

-- name: ListShopBurgersByShop :many
SELECT * FROM shops_burgers
WHERE shop_id = $1
ORDER BY burger_id;

-- name: GetShopBurgerWithStats :one
-- The burger only when it is linked to the shop via shops_burgers, with
-- its stats (NULLs when none calculated yet) — the review submission
-- existence check and response payload in one query.
SELECT b.id, b.name,
       bs.review_count, bs.average_rating, bs.weighted_score, bs.confidence
FROM burgers b
JOIN shops_burgers sb ON sb.burger_id = b.id
LEFT JOIN burger_stats bs ON bs.burger_id = b.id
WHERE sb.shop_id = sqlc.arg(shop_id) AND b.id = sqlc.arg(burger_id);

-- name: DeleteShopBurger :exec
DELETE FROM shops_burgers
WHERE shop_id = $1 AND burger_id = $2;
