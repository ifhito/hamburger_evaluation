-- name: CreateUser :one
INSERT INTO users (email, username, password_digest, admin)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetUser :one
SELECT * FROM users
WHERE id = $1;

-- name: GetActiveUserByEmail :one
SELECT * FROM users
WHERE email = $1 AND discarded_at IS NULL;

-- name: GetActiveUserByID :one
SELECT * FROM users
WHERE id = $1 AND discarded_at IS NULL;

-- name: ListUsers :many
SELECT * FROM users
ORDER BY id
LIMIT $1 OFFSET $2;

-- name: UpdateUser :one
UPDATE users
SET email = $2,
    username = $3,
    password_digest = $4,
    admin = $5,
    discarded_at = $6,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteUser :exec
DELETE FROM users
WHERE id = $1;
