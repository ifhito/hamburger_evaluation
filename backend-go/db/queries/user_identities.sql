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

-- name: GetActiveUserByEmailIgnoreCase :one
-- 外部のサービスでの新規登録のとき、同じメールのアカウントがすでにあるかを調べる。大文字小文字は
-- 区別しない(「Alice@」と「alice@」を別のアカウントとして作らないため)。退会済みは含めない。
SELECT * FROM users
WHERE lower(email) = lower($1) AND discarded_at IS NULL
ORDER BY created_at
LIMIT 1;
