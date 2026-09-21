-- name: CreateMailDelivery :one
-- 送信の記録を pending で作る。同じ idempotency_key がすでにあれば何もせず、行を返さない
-- (呼び出し側は「同じ要求はすでに扱った」と判断し、メールを送らない)。
INSERT INTO mail_deliveries (kind, recipient, idempotency_key)
VALUES ($1, $2, $3)
ON CONFLICT (idempotency_key) DO NOTHING
RETURNING id;

-- name: UpdateMailDeliverySent :exec
UPDATE mail_deliveries
SET status = 'sent',
    attempts = attempts + 1,
    sent_at = now(),
    failure_kind = NULL,
    last_error = NULL
WHERE id = $1;

-- name: UpdateMailDeliveryFailed :exec
UPDATE mail_deliveries
SET status = 'failed',
    attempts = attempts + 1,
    failure_kind = $2,
    last_error = $3
WHERE id = $1;
