-- name: CreateShop :one
INSERT INTO shops (name, status, moderation_note, creator_id)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetShop :one
SELECT * FROM shops
WHERE id = $1;

-- name: ListShops :many
-- The WHERE clause below is the SQL translation of domain.ShopVisibility
-- (view all / active / own); the rule itself lives in the domain package.
-- status 1 = active. name_pattern is a pre-escaped ILIKE pattern (or NULL
-- for no keyword filter); comparing creator_id with a NULL viewer_id is
-- never true, which is exactly the anonymous case.
SELECT id, name, status, moderation_note, creator_id FROM shops
WHERE (sqlc.arg(view_all)::boolean
       OR status = 1
       OR creator_id = sqlc.narg(viewer_id)::bigint)
  AND (sqlc.narg(name_pattern)::text IS NULL OR name ILIKE sqlc.narg(name_pattern)::text)
ORDER BY name, id
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: GetShopWithCreator :one
SELECT s.id, s.name, s.status, s.moderation_note, s.creator_id,
       u.username AS creator_username
FROM shops s
LEFT JOIN users u ON u.id = s.creator_id
WHERE s.id = $1;

-- name: ListShopReviews :many
SELECT r.id, r.rating, r.comment, r.created_at,
       u.id AS user_id, u.username AS user_username,
       b.id AS burger_id, b.name AS burger_name,
       bs.review_count, bs.average_rating, bs.weighted_score, bs.confidence
FROM reviews r
JOIN shops_burgers sb ON sb.burger_id = r.burger_id
JOIN burgers b ON b.id = r.burger_id
JOIN users u ON u.id = r.user_id
LEFT JOIN burger_stats bs ON bs.burger_id = b.id
WHERE sb.shop_id = $1 AND r.discarded_at IS NULL
ORDER BY r.created_at DESC, r.id DESC;

-- name: UpdateShop :one
UPDATE shops
SET name = $2,
    status = $3,
    moderation_note = $4,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteShop :exec
DELETE FROM shops
WHERE id = $1;
