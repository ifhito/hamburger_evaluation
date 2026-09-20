-- Review-derived statistics per burger, recalculated as a whole row;
-- burger_id (unique) is the natural key, so no surrogate id.
CREATE TABLE burger_stats (
    burger_id bigint NOT NULL UNIQUE REFERENCES burgers (id),
    review_count bigint NOT NULL,
    average_rating double precision NOT NULL,
    weighted_score double precision NOT NULL,
    confidence double precision NOT NULL,
    calculated_at timestamptz NOT NULL
);
