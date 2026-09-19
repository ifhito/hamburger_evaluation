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

-- name: GetShopBurgerByNameWithStats :one
-- The shop's burger with the given exact name (the burger_name review
-- submission lookup, mirroring Rails' shop.burgers.find_by(name:)), with
-- its stats (NULLs when none calculated yet) — the find side of
-- find-or-create. Nothing enforces name uniqueness within a shop, so the
-- lowest id wins deterministically.
SELECT b.id, b.name,
       bs.review_count, bs.average_rating, bs.weighted_score, bs.confidence
FROM burgers b
JOIN shops_burgers sb ON sb.burger_id = b.id
LEFT JOIN burger_stats bs ON bs.burger_id = b.id
WHERE sb.shop_id = sqlc.arg(shop_id) AND b.name = sqlc.arg(name)
ORDER BY b.id
LIMIT 1;

-- name: DeleteShopBurger :exec
DELETE FROM shops_burgers
WHERE shop_id = $1 AND burger_id = $2;
