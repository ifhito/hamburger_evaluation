CREATE TABLE reviews (
    id uuid DEFAULT gen_random_uuid() PRIMARY KEY,
    rating smallint NOT NULL CHECK (rating BETWEEN 1 AND 5),
    comment text,
    user_id uuid NOT NULL REFERENCES users (id),
    burger_id uuid NOT NULL REFERENCES burgers (id),
    discarded_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    -- 文字数の上限(Unicode のコードポイント数。char_length と同じ数え方)。
    -- 値は internal/domain の定数と同じでなければならない(食い違いは db/migrations_test.go が検出する)。
    CONSTRAINT reviews_comment_max_length CHECK (char_length(comment) <= 2000)
);

CREATE INDEX idx_reviews_user_id ON reviews (user_id);
CREATE INDEX idx_reviews_burger_id ON reviews (burger_id);
