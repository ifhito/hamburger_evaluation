-- name: CreateLoginHandoff :exec
-- 時刻は DB の now() を使い、アプリとの時計のずれを避ける。
INSERT INTO login_handoffs (code_hash, outcome, user_id, return_to, expires_at)
VALUES (@code_hash, @outcome, sqlc.narg('user_id'), @return_to, now() + make_interval(secs => @ttl_seconds::float8));

-- name: LockLoginHandoffByCodeHash :one
-- 期限内のコードの行を排他ロックする(中身は返さず、ロックできたことだけを id で示す)。コードを使う
-- トランザクションの先頭で使うので、同じコードでの並行する交換は直列になり、2 人目は行が消えているのを見る。
SELECT id FROM login_handoffs
WHERE code_hash = $1 AND expires_at > now()
FOR UPDATE;

-- name: GetLoginHandoffByCodeHash :one
-- 期限内のコードの中身を読む(ロックはしない。同じトランザクションで先にロックしているので、
-- ここで読んだ行は、使い終わるまで変わらない)。
SELECT id, outcome, user_id, return_to FROM login_handoffs
WHERE code_hash = $1 AND expires_at > now();

-- name: DeleteLoginHandoff :exec
DELETE FROM login_handoffs WHERE id = $1;

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
