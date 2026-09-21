-- users.id と、それを参照する reviews.user_id・shops.creator_id を bigint から uuid にする(S27)。
-- bigint から uuid への対応は作れないので、既存の行はすべて捨てる(開発用データだけで、残す
-- データはないというユーザー決定)。開発用の DB は、migrate up のあとに seed で入れ直す。
-- 一覧は 1 つの TRUNCATE にまとめる(外部キーで結ばれた表は、まとめて指定しないと空にできない)。
TRUNCATE users, shops, burgers, shops_burgers, reviews, burger_stats RESTART IDENTITY;

ALTER TABLE reviews DROP CONSTRAINT reviews_user_id_fkey;
ALTER TABLE shops DROP CONSTRAINT shops_creator_id_fkey;

ALTER TABLE users ALTER COLUMN id DROP IDENTITY;
ALTER TABLE users ALTER COLUMN id TYPE uuid USING gen_random_uuid();
ALTER TABLE users ALTER COLUMN id SET DEFAULT gen_random_uuid();

-- 表は空なので、USING の式は評価されない(型変換に必要なので書く)。
ALTER TABLE reviews ALTER COLUMN user_id TYPE uuid USING NULL;
ALTER TABLE shops ALTER COLUMN creator_id TYPE uuid USING NULL;

ALTER TABLE reviews ADD CONSTRAINT reviews_user_id_fkey FOREIGN KEY (user_id) REFERENCES users (id);
ALTER TABLE shops ADD CONSTRAINT shops_creator_id_fkey FOREIGN KEY (creator_id) REFERENCES users (id);
