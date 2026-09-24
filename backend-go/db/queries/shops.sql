-- name: CreateShop :one
INSERT INTO shops (name, status, moderation_note, map_url, creator_id)
VALUES ($1, $2, $3, $4, $5)
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
-- 集計(件数・平均・写真)は、shop_stats の保存された値を LEFT JOIN で添える(1 回のクエリ)。集計は非同期に
-- 計算されるので、行がないショップ(未集計)は、件数 0・平均と写真なしになる。
SELECT s.id, s.name, s.status, s.moderation_note, s.map_url, s.creator_id, s.closed_at,
       COALESCE(ss.review_count, 0)::bigint AS review_count,
       ss.average_rating,
       ss.photo_key
FROM shops s
LEFT JOIN shop_stats ss ON ss.shop_id = s.id
WHERE (sqlc.arg(view_all)::boolean
       OR s.status = 1
       OR s.creator_id = sqlc.narg(viewer_id)::uuid)
  AND (sqlc.narg(name_pattern)::text IS NULL OR s.name ILIKE sqlc.narg(name_pattern)::text)
ORDER BY s.name, s.id
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: ListShopsByNewest :many
-- GET /shops?sort=newest 向け。WHERE 句(可視性・keyword フィルタ)と集計の JOIN は ListShops と
-- 同一で、ORDER BY だけが違う(name 昇順+id 昇順 → created_at 降順+id 降順)。列ごとに向きが違う
-- 並び替えを 1 つの動的な ORDER BY に詰め込むより、クエリを分けた方が読みやすく安全なので、
-- そうしている。
SELECT s.id, s.name, s.status, s.moderation_note, s.map_url, s.creator_id, s.closed_at,
       COALESCE(ss.review_count, 0)::bigint AS review_count,
       ss.average_rating,
       ss.photo_key
FROM shops s
LEFT JOIN shop_stats ss ON ss.shop_id = s.id
WHERE (sqlc.arg(view_all)::boolean
       OR s.status = 1
       OR s.creator_id = sqlc.narg(viewer_id)::uuid)
  AND (sqlc.narg(name_pattern)::text IS NULL OR s.name ILIKE sqlc.narg(name_pattern)::text)
ORDER BY s.created_at DESC, s.id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: ListShopsForModeration :many
-- admin の moderation 一覧：creator 付きのすべての shop を、新しい順に
-- （id desc が created_at の同値を解消し、順序を決定的にする）。
-- status_code は smallint の status フィルタで、すべての status なら NULL
-- である。文字列から smallint への対応付けは repository にある。
SELECT s.id, s.name, s.status, s.moderation_note, s.map_url, s.creator_id, s.closed_at,
       u.username AS creator_username
FROM shops s
LEFT JOIN users u ON u.id = s.creator_id
WHERE sqlc.narg(status_code)::smallint IS NULL
   OR s.status = sqlc.narg(status_code)::smallint
ORDER BY s.created_at DESC, s.id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: GetShopWithCreator :one
-- 集計(件数・平均・写真)は、shop_stats の保存された値を添える(未集計のショップは、件数 0・平均と写真なし)。
SELECT s.id, s.name, s.status, s.moderation_note, s.map_url, s.creator_id, s.closed_at,
       u.username AS creator_username,
       COALESCE(ss.review_count, 0)::bigint AS review_count,
       ss.average_rating,
       ss.photo_key
FROM shops s
LEFT JOIN users u ON u.id = s.creator_id
LEFT JOIN shop_stats ss ON ss.shop_id = s.id
WHERE s.id = $1;

-- name: ListShopReviews :many
-- その shop の、discard されていない user の、discard されていない review。
-- 新しい順。u.discarded_at フィルタは、discard 済みの user の（まだ kept な）
-- review を shop 詳細から隠す。
SELECT r.id, r.rating, r.comment, r.created_at, r.visited_at,
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
-- 列を限定した名前変更：name と map_url だけを更新するので、並行する status の変更
-- （approve/reject）が古いスナップショットによって元に戻されることはない。
UPDATE shops
SET name = $2,
    map_url = $3,
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

-- name: UpdateShopClosedAt :one
-- 列を限定した閉業/再開の遷移：closed_at だけを更新するため、並行する
-- 名前変更や moderation の status の変更が古いスナップショットによって
-- 元に戻されることはない。
UPDATE shops
SET closed_at = $2,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteShop :exec
DELETE FROM shops
WHERE id = $1;
