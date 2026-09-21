-- 外部のサービスでのサインインの結果を、画面(SPA)へ渡すコードの中身。コードは短命(60 秒)で、1 回しか
-- 使えない(使うと行が消える)。code_hash はコードの SHA-256 で、平文のコードは保存しない。JWT は
-- ここに保存しない(コードを使った時点で、新しく発行する)。
CREATE TABLE login_handoffs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code_hash text NOT NULL,
    -- 結果の種類。値は internal/domain の LoginHandoffOutcome と同じでなければならない。
    outcome text NOT NULL,
    -- 結果が利用者を伴うとき(signed_in・linked・link_intent)だけ入る。
    user_id uuid REFERENCES users (id) ON DELETE CASCADE,
    -- 手続きのあとに戻る先(アプリの中のパス。空は既定の画面)。長さと形は、保存の前に domain(SanitizeReturnTo)が保証する。
    return_to text NOT NULL DEFAULT '',
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT login_handoffs_code_hash_key UNIQUE (code_hash),
    CONSTRAINT login_handoffs_outcome_check CHECK (outcome IN
        ('signed_in', 'linked', 'link_intent', 'account_exists', 'identity_taken', 'already_linked', 'failed')),
    CONSTRAINT login_handoffs_user_check CHECK
        ((outcome IN ('signed_in', 'linked', 'link_intent')) = (user_id IS NOT NULL))
);

-- 期限切れの行の掃除と、期限の判定のため。
CREATE INDEX idx_login_handoffs_expires_at ON login_handoffs (expires_at);
-- 外部キー(利用者の削除で連鎖して消す)の検索のため。
CREATE INDEX idx_login_handoffs_user_id ON login_handoffs (user_id);
