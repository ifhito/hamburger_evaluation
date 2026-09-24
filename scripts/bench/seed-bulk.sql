-- 計測用の一括データ投入。cmd/seed の fixture(4 ユーザー・10 レビュー)は
-- レイテンシ計測には小さすぎるため、これで嵩を足す。
--
--   usage: psql "$DATABASE_URL" -v users=50 -v shops=50 -v burgers=200 -v reviews=1000 \
--            -f scripts/bench/seed-bulk.sql
--
-- 冪等ではない。同じ DB に 2 回流すと倍になる。検証の 4 候補で件数をそろえること。
-- 目印として name / email / comment に 'bench-' を付けるので、消すときは下の DELETE を使う。
--
-- 片付け:
--   DELETE FROM reviews  WHERE comment LIKE 'bench-%';
--   DELETE FROM shops_burgers WHERE burger_id IN (SELECT id FROM burgers WHERE name LIKE 'bench-%');
--   DELETE FROM burgers WHERE name  LIKE 'bench-%';
--   DELETE FROM shops   WHERE name  LIKE 'bench-%';
--   DELETE FROM users   WHERE email LIKE 'bench-%';

BEGIN;

-- パスワードは検証用の固定値(bcrypt の digest)。cmd/seed と同じ Password123! を使う。
-- ログインが要る計測では、既存の fixture ユーザー(alice など)を使うほうが確実。
INSERT INTO users (email, username, password_digest, admin)
SELECT
  'bench-' || i || '@example.com',
  'bench-user-' || i,
  (SELECT password_digest FROM users ORDER BY created_at LIMIT 1),
  false
FROM generate_series(1, :users) AS i;

INSERT INTO shops (name, status, creator_id)
SELECT
  'bench-shop-' || i,
  1,                                  -- 1 = 公開(active)
  (SELECT id FROM users WHERE email LIKE 'bench-%' ORDER BY random() LIMIT 1)
FROM generate_series(1, :shops) AS i;

INSERT INTO burgers (name)
SELECT 'bench-burger-' || i
FROM generate_series(1, :burgers) AS i;

-- バーガーを店に割り当てる(1 バーガー 1 店)。
INSERT INTO shops_burgers (shop_id, burger_id)
SELECT
  (SELECT id FROM shops WHERE name LIKE 'bench-%' ORDER BY random() LIMIT 1),
  b.id
FROM burgers b
WHERE b.name LIKE 'bench-%'
ON CONFLICT DO NOTHING;

-- レビュー。created_at を過去 2 年に散らして、時間減衰(半減期 180 日)の計算に幅を持たせる。
INSERT INTO reviews (rating, comment, user_id, burger_id, created_at)
SELECT
  1 + (random() * 4)::int,
  'bench-review-' || i,
  (SELECT id FROM users   WHERE email LIKE 'bench-%' ORDER BY random() LIMIT 1),
  (SELECT id FROM burgers WHERE name  LIKE 'bench-%' ORDER BY random() LIMIT 1),
  now() - (random() * interval '730 days')
FROM generate_series(1, :reviews) AS i;

-- 統計はワーカーに計算させる。全 bench バーガーの再計算依頼を積む。
-- API を起動しておけば、StatsWorkerLoop が拾って burger_stats を埋める。
INSERT INTO burger_stats_recalc_requests (burger_id)
SELECT id FROM burgers WHERE name LIKE 'bench-%'
ON CONFLICT (burger_id) DO NOTHING;

COMMIT;

\echo '--- 投入後の件数 ---'
SELECT
  (SELECT count(*) FROM users)   AS users,
  (SELECT count(*) FROM shops)   AS shops,
  (SELECT count(*) FROM burgers) AS burgers,
  (SELECT count(*) FROM reviews) AS reviews,
  (SELECT count(*) FROM burger_stats_recalc_requests) AS pending_recalc;
