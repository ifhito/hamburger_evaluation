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
