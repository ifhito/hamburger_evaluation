CREATE TABLE users (
    id uuid DEFAULT gen_random_uuid() PRIMARY KEY,
    email text NOT NULL UNIQUE,
    username text NOT NULL,
    password_digest text NOT NULL,
    admin boolean NOT NULL DEFAULT false,
    discarded_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
