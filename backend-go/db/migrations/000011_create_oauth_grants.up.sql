-- OAuth の許可の記録。利用者が、あるアプリ(AI アプリなどのクライアント)に、どの範囲を許可したかを、
-- 利用者とアプリの組ごとに 1 行で持つ。許可を取り消すには、この行を消す。この行から発行された
-- トークン(oauth_token_sessions)は、外部キーの連鎖削除で、同時に消える。
-- client_id は、アプリが自分の説明を公開している URL、または固定で登録したアプリの識別子で、
-- アプリの登録は、この表では持たない(client_name は許可したときの表示名の控え)。
CREATE TABLE oauth_grants (
    id uuid DEFAULT gen_random_uuid() PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    client_id text NOT NULL,
    client_name text NOT NULL,
    scopes text[] NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    -- 文字数・個数の上限。値は internal/domain の定数と同じでなければならない
    -- (食い違いは db/migrations_test.go が検出する)。
    CONSTRAINT oauth_grants_user_client_key UNIQUE (user_id, client_id),
    CONSTRAINT oauth_grants_client_id_max_length CHECK (char_length(client_id) <= 2048),
    CONSTRAINT oauth_grants_client_id_not_empty CHECK (char_length(client_id) >= 1),
    CONSTRAINT oauth_grants_client_name_max_length CHECK (char_length(client_name) <= 100),
    CONSTRAINT oauth_grants_client_name_not_empty CHECK (char_length(client_name) >= 1),
    CONSTRAINT oauth_grants_scopes_not_empty CHECK (cardinality(scopes) >= 1)
);
