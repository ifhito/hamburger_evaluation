-- uuid から bigint への対応は作れないので、up と同様に既存の行はすべて捨てる。
TRUNCATE users, shops, burgers, shops_burgers, reviews, burger_stats RESTART IDENTITY;

ALTER TABLE reviews DROP CONSTRAINT reviews_user_id_fkey;
ALTER TABLE shops DROP CONSTRAINT shops_creator_id_fkey;

ALTER TABLE reviews ALTER COLUMN user_id TYPE bigint USING NULL;
ALTER TABLE shops ALTER COLUMN creator_id TYPE bigint USING NULL;

ALTER TABLE users ALTER COLUMN id DROP DEFAULT;
ALTER TABLE users ALTER COLUMN id TYPE bigint USING 0;
ALTER TABLE users ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY;

ALTER TABLE reviews ADD CONSTRAINT reviews_user_id_fkey FOREIGN KEY (user_id) REFERENCES users (id);
ALTER TABLE shops ADD CONSTRAINT shops_creator_id_fkey FOREIGN KEY (creator_id) REFERENCES users (id);
