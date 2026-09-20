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

-- name: ListShopsForModeration :many
-- Admin moderation list: every shop with its creator, newest first
-- (id desc breaks created_at ties for a deterministic order).
-- status_code is the smallint status filter, NULL for all statuses; the
-- string-to-smallint mapping lives in the repository.
SELECT s.id, s.name, s.status, s.moderation_note, s.creator_id,
       u.username AS creator_username
FROM shops s
LEFT JOIN users u ON u.id = s.creator_id
WHERE sqlc.narg(status_code)::smallint IS NULL
   OR s.status = sqlc.narg(status_code)::smallint
ORDER BY s.created_at DESC, s.id DESC;

-- name: GetShopWithCreator :one
SELECT s.id, s.name, s.status, s.moderation_note, s.creator_id,
       u.username AS creator_username
FROM shops s
LEFT JOIN users u ON u.id = s.creator_id
WHERE s.id = $1;

-- name: ListShopReviews :many
-- The shop's non-discarded reviews of non-discarded users, newest first;
-- the u.discarded_at filter hides discarded users' (still kept) reviews
-- from the shop detail (S8).
SELECT r.id, r.rating, r.comment, r.created_at,
       u.id AS user_id, u.username AS user_username,
       b.id AS burger_id, b.name AS burger_name,
       bs.review_count, bs.average_rating, bs.weighted_score, bs.confidence
FROM reviews r
JOIN shops_burgers sb ON sb.burger_id = r.burger_id
JOIN burgers b ON b.id = r.burger_id
JOIN users u ON u.id = r.user_id
LEFT JOIN burger_stats bs ON bs.burger_id = b.id
WHERE sb.shop_id = $1 AND r.discarded_at IS NULL AND u.discarded_at IS NULL
ORDER BY r.created_at DESC, r.id DESC;

-- name: UpdateShop :one
UPDATE shops
SET name = $2,
    status = $3,
    moderation_note = $4,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpdateShopName :one
-- Column-scoped rename: touches only name so a concurrent status change
-- (approve/reject) is never reverted from a stale snapshot.
UPDATE shops
SET name = $2,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpdateShopStatus :one
-- Column-scoped moderation transition: touches only status and
-- moderation_note so a concurrent rename is never reverted from a stale
-- snapshot.
UPDATE shops
SET status = $2,
    moderation_note = $3,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteShop :exec
DELETE FROM shops
WHERE id = $1;
