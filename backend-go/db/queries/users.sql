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

-- name: ListActiveUsers :many
-- user の一覧：kept な user すべて、id の昇順。LIMIT/OFFSET はない。
-- Rails parity：一覧は kept な user をすべて、ページネーションなしで返す。
SELECT * FROM users
WHERE discarded_at IS NULL
ORDER BY id;

-- name: UpdateUserUsername :one
-- 列を限定したプロフィール更新：username だけを更新し、user がまだ kept な
-- 間だけ更新するので、並行する email/password の変更や soft delete が古い
-- スナップショットによって元に戻されることはない。
UPDATE users
SET username = $2,
    updated_at = now()
WHERE id = $1 AND discarded_at IS NULL
RETURNING *;

-- name: UpdateUserEmail :one
-- 列を限定したプロフィール更新：email だけを更新する（理由は
-- UpdateUserUsername を参照）。
UPDATE users
SET email = $2,
    updated_at = now()
WHERE id = $1 AND discarded_at IS NULL
RETURNING *;

-- name: UpdateUserPasswordDigest :one
-- 列を限定したプロフィール更新：password_digest だけを更新する（理由は
-- UpdateUserUsername を参照）。
UPDATE users
SET password_digest = $2,
    updated_at = now()
WHERE id = $1 AND discarded_at IS NULL
RETURNING *;

-- name: DiscardUser :one
-- 列を限定した soft delete：discarded_at にタイムスタンプを設定するだけで、
-- しかも 1 回だけ行う。すでに discard 済みの user はどの行にもマッチせず、
-- not found として現れる（DiscardReview に対応する）。
UPDATE users
SET discarded_at = now(),
    updated_at = now()
WHERE id = $1 AND discarded_at IS NULL
RETURNING id;
