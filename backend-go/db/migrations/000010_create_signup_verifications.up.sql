-- 確認待ちの signup(S16)。users とは別のテーブルにするので、確認前の signup が
-- users を汚さない(users のスキーマは変えない)。token_hash は確認トークンの
-- SHA-256 で、平文のトークンは保存しない。password_digest は bcrypt 済みで、平文は保存しない。
CREATE TABLE signup_verifications (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email text NOT NULL,
    username text NOT NULL,
    password_digest text NOT NULL,
    token_hash text NOT NULL,
    expires_at timestamptz NOT NULL,
    last_sent_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT signup_verifications_token_hash_key UNIQUE (token_hash)
);

-- email は入力どおりに保存し(users.email と同じ)、大文字小文字を区別しない一意性で、
-- 同じ email の確認待ちを 1 行に保つ(再 signup は upsert で置き換える)。
CREATE UNIQUE INDEX idx_signup_verifications_email_lower ON signup_verifications (lower(email));
-- 期限切れの行の掃除と、有効期限の判定のため。
CREATE INDEX idx_signup_verifications_expires_at ON signup_verifications (expires_at);
