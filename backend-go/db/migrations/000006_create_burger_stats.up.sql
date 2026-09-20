-- review から導出される burger ごとの統計で、行全体として再計算される。
-- burger_id（unique）が natural key なので、surrogate id は持たない。
CREATE TABLE burger_stats (
    burger_id bigint NOT NULL UNIQUE REFERENCES burgers (id),
    review_count bigint NOT NULL,
    average_rating double precision NOT NULL,
    weighted_score double precision NOT NULL,
    confidence double precision NOT NULL,
    calculated_at timestamptz NOT NULL
);
