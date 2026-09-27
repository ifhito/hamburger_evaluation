-- ショップの住所(任意)を追加する。都道府県は JIS X 0401 のコード(1〜47。未設定は NULL)で、名前の表は domain が持つ。
-- 市区町村と番地以降は、未設定なら空文字。上限の値は internal/domain の定数と同じ(食い違いは db/migrations_test.go が検出する)。
ALTER TABLE shops
    ADD COLUMN prefecture_code smallint,
    ADD COLUMN city text NOT NULL DEFAULT '',
    ADD COLUMN street_address text NOT NULL DEFAULT '',
    ADD CONSTRAINT shops_prefecture_code_range CHECK (prefecture_code BETWEEN 1 AND 47),
    ADD CONSTRAINT shops_city_max_length CHECK (char_length(city) <= 100),
    ADD CONSTRAINT shops_street_address_max_length CHECK (char_length(street_address) <= 200);

CREATE INDEX idx_shops_prefecture_code ON shops (prefecture_code);
