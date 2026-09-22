-- ショップごとの集計(レビューの件数・評価の平均・ショップの写真)。ショップの一覧・詳細は、この表の保存された値を
-- 返す。値は、レビューを書き込むときには計算せず、バーガーの統計(burger_stats)と同じ仕組み(再計算の依頼と
-- バックグラウンドのワーカー。000010・000016)で、あとから計算し直す(結果整合)。書き込みの数秒あとに反映される。
-- 1 つのショップにつき 1 行で、レビューがまだ 1 件もない(または、まだ集計されていない)ショップは、行がない。
-- 読み取りは、行がないショップを、件数 0・平均と写真なしとして返す。
--
-- 集計の意味(数える範囲・平均の求め方・写真の選び方)は、この表ではなく domain の CalculateShopStat が持つ。
-- ここには、計算した結果だけを保存する。
CREATE TABLE shop_stats (
    shop_id uuid PRIMARY KEY REFERENCES shops (id) ON DELETE CASCADE,
    review_count bigint NOT NULL,
    -- レビューがないとき(review_count = 0)は NULL。あるときは、小数 1 桁に丸めた平均(評価は 1〜5 なので、
    -- 重み付きの平均も 1〜5)。
    average_rating double precision,
    -- ショップの写真として使う写真の保存キー。写真つきのレビューがないときは NULL。
    photo_key text,
    calculated_at timestamptz NOT NULL,
    CONSTRAINT shop_stats_review_count_check CHECK (review_count >= 0),
    CONSTRAINT shop_stats_average_rating_check CHECK (average_rating BETWEEN 1 AND 5),
    CONSTRAINT shop_stats_average_rating_presence_check CHECK ((review_count > 0) = (average_rating IS NOT NULL))
);
