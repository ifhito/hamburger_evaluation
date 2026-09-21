-- name: UpsertOAuthGrant :one
-- 利用者とアプリの組の許可を保存する。すでにあれば、許可の範囲を、既存の範囲と新しい範囲を合わせた
-- ものにし(狭めない)、表示名を最新にする。範囲の並びは、呼び出し側(domain)が整えて渡す。
INSERT INTO oauth_grants (user_id, client_id, client_name, scopes)
VALUES (@user_id, @client_id, @client_name, @scopes::text[])
ON CONFLICT (user_id, client_id) DO UPDATE
SET client_name = EXCLUDED.client_name,
    scopes = ARRAY(
        SELECT DISTINCT s FROM unnest(oauth_grants.scopes || EXCLUDED.scopes) AS s ORDER BY s
    ),
    updated_at = now()
RETURNING id;

-- name: DeleteOAuthGrant :execrows
-- 利用者本人の許可だけを削除する(別の利用者の id を渡しても、0 行になる)。発行済みのトークンは、
-- 外部キーの連鎖削除で同時に消える。
DELETE FROM oauth_grants WHERE id = @id AND user_id = @user_id;

-- name: DeleteOAuthGrantsByUser :exec
-- 利用者のすべての許可を削除する(退会のとき)。発行済みのトークンは、外部キーの連鎖削除で同時に消える。
DELETE FROM oauth_grants WHERE user_id = @user_id;

-- name: GetOAuthGrantByUserAndClient :one
SELECT id, user_id, client_id, client_name, scopes, created_at, updated_at
FROM oauth_grants
WHERE user_id = @user_id AND client_id = @client_id;

-- name: ListOAuthGrantsByUser :many
-- 利用者が許可したアプリを、最近使ったものから順に返す(同時刻は id で決める)。
SELECT id, user_id, client_id, client_name, scopes, created_at, updated_at
FROM oauth_grants
WHERE user_id = @user_id
ORDER BY updated_at DESC, id;
