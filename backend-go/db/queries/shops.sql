-- name: CreateShop :one
INSERT INTO shops (name, status, moderation_note, creator_id)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetShop :one
SELECT * FROM shops
WHERE id = $1;

-- name: ListShops :many
-- 以下の WHERE 句は domain.ShopVisibility
-- （すべて閲覧 / active / 自分のもの）を SQL に翻訳したものであり、
-- ルール自体は domain パッケージにある。status 1 = active。name_pattern は
-- あらかじめエスケープ済みの ILIKE パターン（キーワードフィルタなしなら
-- NULL）である。creator_id を NULL の viewer_id と比較しても決して真に
-- ならず、これがまさに匿名の場合である。
SELECT id, name, status, moderation_note, creator_id FROM shops
WHERE (sqlc.arg(view_all)::boolean
       OR status = 1
       OR creator_id = sqlc.narg(viewer_id)::uuid)
  AND (sqlc.narg(name_pattern)::text IS NULL OR name ILIKE sqlc.narg(name_pattern)::text)
ORDER BY name, id
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: ListShopsForModeration :many
-- admin の moderation 一覧：creator 付きのすべての shop を、新しい順に
-- （id desc が created_at の同値を解消し、順序を決定的にする）。
-- status_code は smallint の status フィルタで、すべての status なら NULL
-- である。文字列から smallint への対応付けは repository にある。
SELECT s.id, s.name, s.status, s.moderation_note, s.creator_id,
       u.username AS creator_username
FROM shops s
LEFT JOIN users u ON u.id = s.creator_id
WHERE sqlc.narg(status_code)::smallint IS NULL
   OR s.status = sqlc.narg(status_code)::smallint
ORDER BY s.created_at DESC, s.id DESC;

-- name: GetShopWithCreator :one
SELECT s.id, s.name, s.status, s.moderation_note, s.creator_id,
       u.username AS creator_username
FROM shops s
LEFT JOIN users u ON u.id = s.creator_id
WHERE s.id = $1;

-- name: ListShopReviews :many
-- その shop の、discard されていない user の、discard されていない review。
-- 新しい順。u.discarded_at フィルタは、discard 済みの user の（まだ kept な）
-- review を shop 詳細から隠す。
SELECT r.id, r.rating, r.comment, r.created_at,
       u.id AS user_id, u.username AS user_username,
       b.id AS burger_id, b.name AS burger_name,
       bs.review_count, bs.average_rating, bs.weighted_score, bs.confidence
FROM reviews r
JOIN shops_burgers sb ON sb.burger_id = r.burger_id
JOIN burgers b ON b.id = r.burger_id
JOIN users u ON u.id = r.user_id
LEFT JOIN burger_stats bs ON bs.burger_id = b.id
WHERE sb.shop_id = $1 AND r.discarded_at IS NULL AND u.discarded_at IS NULL
ORDER BY r.created_at DESC, r.id DESC;

-- name: UpdateShop :one
UPDATE shops
SET name = $2,
    status = $3,
    moderation_note = $4,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpdateShopName :one
-- 列を限定した名前変更：name だけを更新するので、並行する status の変更
-- （approve/reject）が古いスナップショットによって元に戻されることはない。
UPDATE shops
SET name = $2,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpdateShopStatus :one
-- 列を限定した moderation 遷移：status と moderation_note だけを更新する
-- ため、並行する名前変更が古いスナップショットによって元に戻される
-- ことはない。
UPDATE shops
SET status = $2,
    moderation_note = $3,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteShop :exec
DELETE FROM shops
WHERE id = $1;

-- name: ListShopSummaries :many
-- 指定した shop それぞれの、レビューの件数・評価の平均・ショップの写真のキー(写真つきで
-- 最も新しいレビューの写真)。集計の対象は、ListShopReviews(shop 詳細に出るレビュー)と同じ範囲:
-- discard されていない user の、discard されていない review。集計の意味と丸めは
-- domain.ShopSummary が持つ。1 回の集約で、shop の件数に比例してクエリを増やさない。
-- レビューのない shop は行を返さない(呼び出し側が、空の集計にする)。
WITH kept AS (
    SELECT sb.shop_id, r.id, r.rating, r.photo_key, r.created_at
    FROM reviews r
    JOIN shops_burgers sb ON sb.burger_id = r.burger_id
    JOIN users u ON u.id = r.user_id
    WHERE sb.shop_id = ANY(sqlc.arg(shop_ids)::uuid[])
      AND r.discarded_at IS NULL AND u.discarded_at IS NULL
), latest_photo AS (
    SELECT DISTINCT ON (shop_id) shop_id, photo_key
    FROM kept
    WHERE photo_key IS NOT NULL
    ORDER BY shop_id, created_at DESC, id DESC
)
SELECT k.shop_id,
       COUNT(*)::bigint AS review_count,
       AVG(k.rating)::float8 AS average_rating,
       lp.photo_key
FROM kept k
LEFT JOIN latest_photo lp ON lp.shop_id = k.shop_id
GROUP BY k.shop_id, lp.photo_key;
