CREATE TABLE users (
    id uuid DEFAULT gen_random_uuid() PRIMARY KEY,
    email text NOT NULL UNIQUE,
    username text NOT NULL,
    -- 自己紹介文（bio は biography の略）。空文字は未設定を表す。
    bio text NOT NULL DEFAULT '',
    password_digest text NOT NULL,
    admin boolean NOT NULL DEFAULT false,
    discarded_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    -- 文字数の上限(Unicode のコードポイント数。char_length と同じ数え方)。
    -- 値は internal/domain の定数と同じでなければならない(食い違いは db/migrations_test.go が検出する)。
    CONSTRAINT users_username_max_length CHECK (char_length(username) <= 50),
    CONSTRAINT users_bio_max_length CHECK (char_length(bio) <= 500),
    CONSTRAINT users_email_max_length CHECK (char_length(email) <= 254)
);
