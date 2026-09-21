-- メール送信の記録。送信の履歴と冪等キーだけを持つ。本文・確認トークン・パスワードは
-- 保存しない。同じ idempotency_key の要求は 1 行にしかならないので、メールは 1 通しか出ない。
-- status は pending(記録した。送信前)→ sent / failed。failed のときは、失敗の種類
-- (temporary=再試行で直りうる / permanent=直らない)と、切り詰めた理由(last_error)を持つ。
CREATE TABLE mail_deliveries (
    id uuid DEFAULT gen_random_uuid() PRIMARY KEY,
    kind text NOT NULL,
    recipient text NOT NULL,
    idempotency_key text NOT NULL,
    status text NOT NULL DEFAULT 'pending',
    failure_kind text,
    attempts integer NOT NULL DEFAULT 0,
    last_error text,
    created_at timestamptz NOT NULL DEFAULT now(),
    sent_at timestamptz,
    CONSTRAINT mail_deliveries_idempotency_key_key UNIQUE (idempotency_key),
    CONSTRAINT mail_deliveries_kind_check CHECK (kind IN ('signup_confirmation', 'already_registered')),
    CONSTRAINT mail_deliveries_status_check CHECK (status IN ('pending', 'sent', 'failed')),
    CONSTRAINT mail_deliveries_failure_kind_check CHECK (failure_kind IN ('temporary', 'permanent')),
    CONSTRAINT mail_deliveries_attempts_check CHECK (attempts >= 0),
    CONSTRAINT mail_deliveries_recipient_length_check CHECK (char_length(recipient) <= 254),
    CONSTRAINT mail_deliveries_idempotency_key_length_check CHECK (char_length(idempotency_key) <= 400),
    CONSTRAINT mail_deliveries_last_error_length_check CHECK (char_length(last_error) <= 200),
    -- 結果の列は status と食い違わない: sent だけが sent_at を持ち、failed だけが failure_kind を持つ。
    CONSTRAINT mail_deliveries_sent_at_check CHECK ((status = 'sent') = (sent_at IS NOT NULL)),
    CONSTRAINT mail_deliveries_failure_kind_status_check CHECK ((status = 'failed') = (failure_kind IS NOT NULL))
);
