CREATE TABLE reviews (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    rating smallint NOT NULL CHECK (rating BETWEEN 1 AND 5),
    comment text,
    user_id uuid NOT NULL REFERENCES users (id),
    burger_id bigint NOT NULL REFERENCES burgers (id),
    discarded_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_reviews_user_id ON reviews (user_id);
CREATE INDEX idx_reviews_burger_id ON reviews (burger_id);
