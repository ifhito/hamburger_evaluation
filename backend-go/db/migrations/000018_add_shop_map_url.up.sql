ALTER TABLE shops
    ADD COLUMN map_url text,
    ADD CONSTRAINT shops_map_url_max_length CHECK (char_length(map_url) <= 2048);
