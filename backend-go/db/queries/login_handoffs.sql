-- name: CreateLoginHandoff :exec
-- 時刻は DB の now() を使い、アプリとの時計のずれを避ける。
INSERT INTO login_handoffs (code_hash, outcome, user_id, return_to, expires_at)
VALUES (@code_hash, @outcome, sqlc.narg('user_id'), @return_to, now() + make_interval(secs => @ttl_seconds::float8));

-- name: DeleteLoginHandoffByCodeHash :one
-- 期限内のコードの中身を、削除しながら返す(1 つの文なので、同じコードを並行して使っても、
-- 成功するのは 1 回だけである)。
DELETE FROM login_handoffs
WHERE code_hash = $1 AND expires_at > now()
RETURNING id, outcome, user_id, return_to;

-- name: DeleteExpiredLoginHandoffs :execrows
-- 期限切れの行を上限つきで削除する。ほかの掃除や使用が掴んでいる行は待たずに飛ばす。
DELETE FROM login_handoffs
WHERE id IN (
    SELECT lh.id FROM login_handoffs lh
    WHERE lh.expires_at <= now()
    ORDER BY lh.expires_at
    LIMIT $1
    FOR UPDATE SKIP LOCKED
);
