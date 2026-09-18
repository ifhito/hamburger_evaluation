CREATE TABLE shops_burgers (
    shop_id bigint NOT NULL REFERENCES shops (id),
    burger_id bigint NOT NULL REFERENCES burgers (id),
    UNIQUE (shop_id, burger_id)
);

-- The composite unique index above serves shop_id-leading lookups;
-- burger_id needs its own index for the reverse direction.
CREATE INDEX idx_shops_burgers_burger_id ON shops_burgers (burger_id);
