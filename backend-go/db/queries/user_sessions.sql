-- name: CreateUserSession :exec
-- 時刻は DB の now() を使い、アプリとの時計のずれを避ける。
INSERT INTO user_sessions (user_id, token_hash, expires_at)
VALUES (@user_id, @token_hash, now() + make_interval(secs => @ttl_seconds::float8));

-- name: GetActiveUserBySessionTokenHash :one
-- 有効期限内のセッションと、退会していない利用者を 1 回の JOIN で解決する。
SELECT u.id, u.email, u.username, u.bio, u.admin
FROM user_sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token_hash = @token_hash
  AND s.user_id = @user_id
  AND s.expires_at > now()
  AND u.discarded_at IS NULL;

-- name: DeleteUserSessionByTokenHash :exec
DELETE FROM user_sessions WHERE token_hash = $1;

-- name: DeleteUserSessionsByUser :exec
DELETE FROM user_sessions WHERE user_id = $1;

-- name: DeleteExpiredUserSessions :execrows
-- 期限切れのセッションを上限つきで削除する。ほかの処理が掴んでいる行は待たずに飛ばす。
DELETE FROM user_sessions
WHERE id IN (
    SELECT us.id FROM user_sessions us
    WHERE us.expires_at <= now()
    ORDER BY us.expires_at
    LIMIT $1
    FOR UPDATE SKIP LOCKED
);
