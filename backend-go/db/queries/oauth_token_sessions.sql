-- name: InsertOAuthTokenSession :exec
INSERT INTO oauth_token_sessions (kind, signature, request_id, grant_id, user_id, client_id, active, request, expires_at)
VALUES (@kind, @signature, @request_id, @grant_id, @user_id, @client_id, @active, @request, @expires_at);

-- name: GetOAuthTokenSession :one
SELECT kind, signature, request_id, grant_id, user_id, client_id, active, request, expires_at
FROM oauth_token_sessions
WHERE kind = @kind AND signature = @signature;

-- name: DeactivateOAuthTokenSession :execrows
-- 有効な記録だけを無効にする。判定と更新が 1 つの文なので、並行して使おうとしても、成功するのは 1 つだけ
-- (0 行なら、呼び出し側が、存在しないのか、すでに無効なのかを、読み取りで区別する)。
UPDATE oauth_token_sessions SET active = false
WHERE kind = @kind AND signature = @signature AND active;

-- name: RotateOAuthRefreshToken :one
-- 有効な更新トークンを入れ替え済み(無効)にし、成功したときだけ、同じ系列のアクセストークンを削除する。
-- 1 つの文なので、判定・無効化・削除は分けられない(0 行なら、何も変えていない)。
WITH rotated AS (
    UPDATE oauth_token_sessions AS r SET active = false
    WHERE r.kind = 'refresh_token' AND r.signature = @signature AND r.request_id = @request_id AND r.active
    RETURNING r.id
), dropped AS (
    DELETE FROM oauth_token_sessions AS a
    WHERE a.kind = 'access_token' AND a.request_id = @request_id AND EXISTS (SELECT 1 FROM rotated)
    RETURNING a.id
)
SELECT count(*)::bigint AS rotated_count FROM rotated;

-- name: DeactivateOAuthTokenSessionsByRequest :exec
UPDATE oauth_token_sessions SET active = false
WHERE request_id = @request_id AND kind = @kind AND active;

-- name: DeleteOAuthTokenSession :exec
DELETE FROM oauth_token_sessions WHERE kind = @kind AND signature = @signature;

-- name: DeleteOAuthTokenSessionsByRequest :exec
DELETE FROM oauth_token_sessions WHERE request_id = @request_id AND kind = @kind;

-- name: DeleteExpiredOAuthTokenSessions :execrows
-- 期限切れの記録を、上限つきで削除する。ほかの処理が掴んでいる行は待たずに飛ばす。
DELETE FROM oauth_token_sessions
WHERE id IN (
    SELECT s.id FROM oauth_token_sessions s
    WHERE s.expires_at <= now()
    ORDER BY s.expires_at
    LIMIT $1
    FOR UPDATE SKIP LOCKED
);
