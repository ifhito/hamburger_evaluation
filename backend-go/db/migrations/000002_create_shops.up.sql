-- status: 0=pending, 1=active, 2=rejected
CREATE TABLE shops (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name text NOT NULL,
    status smallint NOT NULL CHECK (status IN (0, 1, 2)),
    moderation_note text,
    creator_id uuid REFERENCES users (id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    -- S21: 文字数の上限(Unicode のコードポイント数。char_length と同じ数え方)。
    -- 値は internal/domain の定数と同じでなければならない(食い違いは db/migrations_test.go が検出する)。
    CONSTRAINT shops_name_max_length CHECK (char_length(name) <= 100),
    CONSTRAINT shops_moderation_note_max_length CHECK (char_length(moderation_note) <= 500)
);

CREATE INDEX idx_shops_status ON shops (status);
CREATE INDEX idx_shops_creator_id ON shops (creator_id);
