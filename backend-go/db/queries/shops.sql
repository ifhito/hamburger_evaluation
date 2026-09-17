-- name: CreateShop :one
INSERT INTO shops (name, status, moderation_note, creator_id)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetShop :one
SELECT * FROM shops
WHERE id = $1;

-- name: ListShops :many
SELECT * FROM shops
ORDER BY id
LIMIT $1 OFFSET $2;

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
