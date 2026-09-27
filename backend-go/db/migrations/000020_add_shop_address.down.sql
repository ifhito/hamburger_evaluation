-- 取り消すと住所は失われるが、店舗行は残る。
DROP INDEX idx_shops_prefecture_code;

ALTER TABLE shops
    DROP CONSTRAINT shops_street_address_max_length,
    DROP CONSTRAINT shops_city_max_length,
    DROP CONSTRAINT shops_prefecture_code_range,
    DROP COLUMN street_address,
    DROP COLUMN city,
    DROP COLUMN prefecture_code;
