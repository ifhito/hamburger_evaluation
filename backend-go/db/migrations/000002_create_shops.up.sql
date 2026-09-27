-- status: 0=pending, 1=active, 2=rejected
CREATE TABLE shops (
    id uuid DEFAULT gen_random_uuid() PRIMARY KEY,
    name text NOT NULL,
    status smallint NOT NULL CHECK (status IN (0, 1, 2)),
    moderation_note text,
    creator_id uuid REFERENCES users (id),
    -- 住所(任意)。都道府県は JIS X 0401 のコード(1〜47。未設定は NULL)で、名前の表は domain が持つ。
    -- 市区町村と番地以降は、未設定なら空文字。
    prefecture_code smallint,
    city text NOT NULL DEFAULT '',
    street_address text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    -- 文字数の上限(Unicode のコードポイント数。char_length と同じ数え方)。
    -- 値は internal/domain の定数と同じでなければならない(食い違いは db/migrations_test.go が検出する)。
    CONSTRAINT shops_name_max_length CHECK (char_length(name) <= 100),
    CONSTRAINT shops_moderation_note_max_length CHECK (char_length(moderation_note) <= 500),
    CONSTRAINT shops_city_max_length CHECK (char_length(city) <= 100),
    CONSTRAINT shops_street_address_max_length CHECK (char_length(street_address) <= 200),
    CONSTRAINT shops_prefecture_code_range CHECK (prefecture_code BETWEEN 1 AND 47)
);

CREATE INDEX idx_shops_status ON shops (status);
CREATE INDEX idx_shops_creator_id ON shops (creator_id);
CREATE INDEX idx_shops_prefecture_code ON shops (prefecture_code);
