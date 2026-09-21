CREATE TABLE shops_burgers (
    shop_id uuid NOT NULL REFERENCES shops (id),
    burger_id uuid NOT NULL REFERENCES burgers (id),
    UNIQUE (shop_id, burger_id)
);

-- 上の複合 unique index は shop_id を先頭とする検索に対応する。
-- 逆方向の検索のために、burger_id には専用の index が必要である。
CREATE INDEX idx_shops_burgers_burger_id ON shops_burgers (burger_id);
