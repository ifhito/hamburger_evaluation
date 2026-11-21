-- 退会済み(discarded_at IS NOT NULL)の利用者が同じメールアドレスで再登録できるように、
-- users のメール一意制約を「退会していない行だけに対する大文字小文字を区別しない一意索引」へ置き換える。
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_email_key;
DROP INDEX IF EXISTS users_email_lower_key;
CREATE UNIQUE INDEX users_email_lower_key ON users (lower(email)) WHERE discarded_at IS NULL;

-- ログイン・確認メール・外部サービス認証で発行したセッショントークンの有効性をサーバー側で管理する。
-- 平文トークンは保存せず SHA-256(64 文字の 16 進文字列)だけを保持し、ログアウトや退会で即座に失効させる。
CREATE TABLE IF NOT EXISTS user_sessions (
    id uuid DEFAULT gen_random_uuid() PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash text NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT user_sessions_token_hash_key UNIQUE (token_hash),
    CONSTRAINT user_sessions_token_hash_length CHECK (char_length(token_hash) = 64)
);

CREATE INDEX IF NOT EXISTS idx_user_sessions_user_id ON user_sessions (user_id);
CREATE INDEX IF NOT EXISTS idx_user_sessions_expires_at ON user_sessions (expires_at);
