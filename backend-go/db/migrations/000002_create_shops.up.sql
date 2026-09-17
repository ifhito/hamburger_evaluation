-- status: 0=pending, 1=active, 2=rejected
CREATE TABLE shops (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name text NOT NULL,
    status smallint NOT NULL CHECK (status IN (0, 1, 2)),
    moderation_note text,
    creator_id bigint REFERENCES users (id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_shops_status ON shops (status);
CREATE INDEX idx_shops_creator_id ON shops (creator_id);
