-- name: UpsertSignupVerification :one
-- 同じ email(大文字小文字を区別しない)の確認待ちを、最新の入力で置き換える。
-- 前回の送信から resend_interval_seconds 以内なら何も変えず、行を返さない(呼び出し側は
-- 「送信を見送った」と扱う)。判定と書き込みが 1 文なので、並行する signup でも
-- 送信の間隔は破られない。置き換えるたびに generation が 1 増える(確認メールの冪等キーに使う)。
-- 時刻はすべて DB の now() を使い、アプリとの時計のずれを避ける。
INSERT INTO signup_verifications (email, username, password_digest, token_hash, expires_at, last_sent_at)
VALUES (
    @email,
    @username,
    @password_digest,
    @token_hash,
    now() + make_interval(secs => @ttl_seconds::float8),
    now()
)
ON CONFLICT (lower(email)) DO UPDATE
SET email = EXCLUDED.email,
    username = EXCLUDED.username,
    password_digest = EXCLUDED.password_digest,
    token_hash = EXCLUDED.token_hash,
    expires_at = EXCLUDED.expires_at,
    last_sent_at = now(),
    generation = signup_verifications.generation + 1
WHERE signup_verifications.last_sent_at <= now() - make_interval(secs => @resend_interval_seconds::float8)
RETURNING id, generation;

-- name: LockSignupVerificationByTokenHash :one
-- 期限内の確認待ちの行をロックして返す。確認の transaction の先頭で使うので、
-- 同じトークンでの並行する確認は直列になり、2 人目は行が消えているのを見る。
SELECT * FROM signup_verifications
WHERE token_hash = $1 AND expires_at > now()
FOR UPDATE;

-- name: DeleteSignupVerification :exec
DELETE FROM signup_verifications WHERE id = $1;

-- name: DeleteExpiredSignupVerifications :execrows
-- 期限切れの行を上限つきで削除する。ほかの掃除や確認が掴んでいる行は待たずに飛ばす。
DELETE FROM signup_verifications
WHERE id IN (
    SELECT sv.id FROM signup_verifications sv
    WHERE sv.expires_at <= now()
    ORDER BY sv.expires_at
    LIMIT $1
    FOR UPDATE SKIP LOCKED
);
