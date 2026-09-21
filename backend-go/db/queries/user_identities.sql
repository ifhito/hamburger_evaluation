-- name: CreateUserIdentity :one
INSERT INTO user_identities (user_id, provider, provider_user_id, email)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: DiscardUserIdentity :execrows
DELETE FROM user_identities
WHERE user_id = $1 AND provider = $2;

-- name: DiscardUserIdentitiesByUser :exec
DELETE FROM user_identities
WHERE user_id = $1;

-- name: GetUserIdentityByProviderUserID :one
SELECT * FROM user_identities
WHERE provider = $1 AND provider_user_id = $2;

-- name: ListUserIdentitiesByUser :many
SELECT * FROM user_identities
WHERE user_id = $1
ORDER BY created_at, id;
