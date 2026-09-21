-- name: CreateShopBurger :exec
INSERT INTO shops_burgers (shop_id, burger_id)
VALUES ($1, $2);

-- name: ListShopBurgersByShop :many
-- burger の並びは、作成が古い順(同時刻は id で決定的に)。id は UUID なので、
-- id だけでは作成順にならない。
SELECT sb.* FROM shops_burgers sb
JOIN burgers b ON b.id = sb.burger_id
WHERE sb.shop_id = $1
ORDER BY b.created_at, b.id;

-- name: GetShopBurgerWithStats :one
-- shops_burgers を介してその shop に紐づいている場合に限った burger と、
-- その統計（まだ計算されていなければ NULL）。review 投稿時の存在チェックと
-- レスポンスの payload を 1 つのクエリで兼ねる。
SELECT b.id, b.name,
       bs.review_count, bs.average_rating, bs.weighted_score, bs.confidence
FROM burgers b
JOIN shops_burgers sb ON sb.burger_id = b.id
LEFT JOIN burger_stats bs ON bs.burger_id = b.id
WHERE sb.shop_id = sqlc.arg(shop_id) AND b.id = sqlc.arg(burger_id);

-- name: GetShopBurgerByNameWithStats :one
-- その shop の burger のうち、名前が指定と完全に一致するもの
-- （review 投稿時の burger_name による検索。Rails の
-- shop.burgers.find_by(name:) に対応する）と、その統計
-- （まだ計算されていなければ NULL）。find-or-create の find 側である。
-- shop 内で名前の一意性を強制するものは何もないので、作成が最も古いもの
-- (同時刻なら id が小さいもの)が決定的に採用される。
SELECT b.id, b.name,
       bs.review_count, bs.average_rating, bs.weighted_score, bs.confidence
FROM burgers b
JOIN shops_burgers sb ON sb.burger_id = b.id
LEFT JOIN burger_stats bs ON bs.burger_id = b.id
WHERE sb.shop_id = sqlc.arg(shop_id) AND b.name = sqlc.arg(name)
ORDER BY b.created_at, b.id
LIMIT 1;

-- name: DeleteShopBurger :exec
DELETE FROM shops_burgers
WHERE shop_id = $1 AND burger_id = $2;
