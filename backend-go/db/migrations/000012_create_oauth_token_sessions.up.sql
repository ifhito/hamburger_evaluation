-- OAuth で発行したトークンの記録(認可コード・アクセストークン・更新トークン・認可コードに結び付けた
-- PKCE の情報)。トークンの文字列そのものは保存せず、照合に使う署名(signature)だけを保存する。
-- 同じ認可から発行されたものは、同じ request_id(系列)を持ち、認可コードや更新トークンの再利用を
-- 検知したとき、系列ごと無効にする。active が false の行は、使用済み・入れ替え済みで、再利用の
-- 検知のために残す。request は、認可ライブラリが認可の内容を復元するための保存形式(JSON)である。
CREATE TABLE oauth_token_sessions (
    id uuid DEFAULT gen_random_uuid() PRIMARY KEY,
    kind text NOT NULL,
    signature text NOT NULL,
    request_id uuid NOT NULL,
    grant_id uuid NOT NULL REFERENCES oauth_grants (id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    client_id text NOT NULL,
    active boolean NOT NULL DEFAULT true,
    request jsonb NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT oauth_token_sessions_kind_signature_key UNIQUE (kind, signature),
    CONSTRAINT oauth_token_sessions_kind_check CHECK (kind IN ('authorization_code', 'access_token', 'refresh_token', 'pkce')),
    CONSTRAINT oauth_token_sessions_signature_length CHECK (char_length(signature) BETWEEN 1 AND 200)
);

-- 系列ごとの取り消しと、許可ごとの連鎖削除、期限切れの掃除のため。
CREATE INDEX idx_oauth_token_sessions_request_id ON oauth_token_sessions (request_id);
CREATE INDEX idx_oauth_token_sessions_grant_id ON oauth_token_sessions (grant_id);
CREATE INDEX idx_oauth_token_sessions_expires_at ON oauth_token_sessions (expires_at);
