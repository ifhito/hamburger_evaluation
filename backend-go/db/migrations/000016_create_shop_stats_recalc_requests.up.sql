-- ショップの集計の再計算の依頼。バーガーの統計の再計算の依頼(burger_stats_recalc_requests。000010)と同じ仕組みの
-- 待ち行列(順番待ちの列)で、「このショップの集計を、計算し直す必要がある」という依頼を溜めておく。バーガーの
-- 統計を計算し直したワーカーが、同じトランザクションで、そのバーガーが紐づくショップの依頼を登録し、
-- ショップの集計のワーカーが、依頼を取り出して集計を計算し直し、終えた依頼を消す。
-- 1 つのショップにつき 1 行で、同じショップの依頼(同じショップの別のバーガーからの依頼を含む)は、
-- 1 件にまとまり、ショップの集計の再計算は 1 回で済む(ショップの集計は、そのショップのすべてのレビューを読む
-- 重い計算なので、バーガーごとに繰り返さないための、別の待ち行列である)。
--
-- version・attempts・next_attempt_at・last_error の意味は、burger_stats_recalc_requests と同じ。
CREATE SEQUENCE shop_stats_recalc_requests_version_seq;

CREATE TABLE shop_stats_recalc_requests (
    shop_id uuid PRIMARY KEY REFERENCES shops (id) ON DELETE CASCADE,
    version bigint NOT NULL DEFAULT nextval('shop_stats_recalc_requests_version_seq'),
    attempts integer NOT NULL DEFAULT 0,
    next_attempt_at timestamptz,
    last_error text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT shop_stats_recalc_requests_attempts_check CHECK (attempts >= 0),
    CONSTRAINT shop_stats_recalc_requests_last_error_max_length CHECK (char_length(last_error) <= 500)
);

-- 表を消すと sequence も一緒に消えるようにする。
ALTER SEQUENCE shop_stats_recalc_requests_version_seq OWNED BY shop_stats_recalc_requests.version;

-- 既存のショップの集計を作る。集計の表(000015)を足した時点では、すべてのショップの行がないので、バーガーが
-- 紐づくすべてのショップの再計算の依頼を積む。集計の規則は、ここに複製せず、ワーカーが CalculateShopStat で
-- 計算する(migrate up のあと、ワーカーが動けば、既存のデータの集計が埋まる。開発用 DB の作り直しは要らない)。
INSERT INTO shop_stats_recalc_requests (shop_id)
SELECT DISTINCT shop_id FROM shops_burgers;
