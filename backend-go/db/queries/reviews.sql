-- name: CreateReview :one
INSERT INTO reviews (rating, comment, user_id, burger_id, photo_key)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListPublicReviews :many
-- 全体の review フィード：discard されていない user の、discard されていない
-- review のうち、その burger が少なくとも 1 つの active な shop（status 1）で
-- 提供されているもの。author、burger、統計を 1 つのクエリで取得する
-- （N+1 なし）。shops_burgers に対する単純な JOIN ではなく EXISTS を使うのは、
-- 複数の active な shop に紐づく burger でも、ちょうど 1 行だけが返るように
-- するためである。u.discarded_at フィルタは、discard 済みの user の
-- （まだ kept な）review をフィードから隠す（S8）。
-- 4 つの narg フィルタ（NULL = 未指定、AND で結合される）のうち、
-- filter_user_id 以外は Rails の ReviewQuery に対応する：
-- filter_rating は rating の完全一致である（範囲外の値が smallint 列で
-- オーバーフローせず、比較結果が false になるよう bigint にしている）。
-- comment_pattern は comment に対する、あらかじめエスケープ済みの ILIKE
-- パターンである（keyword_search。NULL の comment は Rails と同様に決して
-- マッチしない）。
-- filter_shop_id は、burger がその shop に紐づいている review を残す
-- （Rails の shops_and_burgers の join）。ここでも行の重複を避けるために
-- EXISTS を使う。フィルタ対象の shop 自身も active（status 1）でなければ
-- ならない。これは shop フィルタが status を考慮しなかった Rails より厳しく、
-- フィードの EXISTS に既にある active な shop のルールとも一貫している。
-- filter_user_id は、その user が書いた review だけを残す（本 API の拡張で、
-- Rails の ReviewQuery にはない）。公開ルール（discard 済みの review・user の
-- 除外と active な shop の EXISTS）はそのまま維持され、このフィルタは絞り込みだけを
-- 行う。公開の範囲を広げることはない。
SELECT r.id, r.rating, r.comment, r.photo_key, r.created_at,
       u.id AS user_id, u.username AS user_username,
       b.id AS burger_id, b.name AS burger_name,
       bs.review_count, bs.average_rating, bs.weighted_score, bs.confidence
FROM reviews r
JOIN users u ON u.id = r.user_id
JOIN burgers b ON b.id = r.burger_id
LEFT JOIN burger_stats bs ON bs.burger_id = b.id
WHERE r.discarded_at IS NULL
  AND u.discarded_at IS NULL
  AND EXISTS (
      SELECT 1
      FROM shops_burgers sb
      JOIN shops s ON s.id = sb.shop_id
      WHERE sb.burger_id = r.burger_id AND s.status = 1
  )
  AND (sqlc.narg(filter_rating)::bigint IS NULL OR r.rating = sqlc.narg(filter_rating)::bigint)
  AND (sqlc.narg(comment_pattern)::text IS NULL OR r.comment ILIKE sqlc.narg(comment_pattern)::text)
  AND (sqlc.narg(filter_shop_id)::bigint IS NULL OR EXISTS (
      SELECT 1
      FROM shops_burgers fsb
      JOIN shops fs ON fs.id = fsb.shop_id AND fs.status = 1
      WHERE fsb.burger_id = r.burger_id AND fsb.shop_id = sqlc.narg(filter_shop_id)::bigint
  ))
  AND (sqlc.narg(filter_user_id)::uuid IS NULL OR r.user_id = sqlc.narg(filter_user_id)::uuid)
ORDER BY r.created_at DESC, r.id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: GetReviewDetail :one
-- discard されていない user の、discard されていない review 1 件（author、
-- burger、統計付き）。公開の詳細 endpoint と、編集・削除の認可のための
-- 読み込み（user_id が所有者チェックを担う）の両方に使われる。author が
-- discard 済みの review は、存在しない review と区別がつかなくなる（S8）。
SELECT r.id, r.rating, r.comment, r.photo_key, r.created_at,
       u.id AS user_id, u.username AS user_username,
       b.id AS burger_id, b.name AS burger_name,
       bs.review_count, bs.average_rating, bs.weighted_score, bs.confidence
FROM reviews r
JOIN users u ON u.id = r.user_id
JOIN burgers b ON b.id = r.burger_id
LEFT JOIN burger_stats bs ON bs.burger_id = b.id
WHERE r.id = $1 AND r.discarded_at IS NULL AND u.discarded_at IS NULL;

-- name: ListUserKeptReviewBurgerIDs :many
-- ユーザーの有効なレビューが付いている、重複のないバーガー。ユーザーの退会に伴って、統計を
-- 計算し直す対象を知るために使う。burger_id 昇順の ORDER BY は欠かせない。統計の再計算
-- (usecase の BurgerStatsRecalculator)は、バーガーの行を FOR UPDATE でロックする。複数の
-- バーガーを続けてロックする処理は、すべて burger_id の昇順にそろえる必要がある。そうすれば、
-- 同時に退会する 2 人のレビューが同じバーガーに付いていても、逆の順序で待ち合って止まる
-- (デッドロックする)ことがない。
SELECT DISTINCT burger_id FROM reviews
WHERE user_id = $1 AND discarded_at IS NULL
ORDER BY burger_id;

-- name: UpdateReviewContent :one
-- 列を限定した編集：rating と comment だけを更新し（discarded_at は決して
-- 更新しない）、review がまだ kept な間だけ更新するので、編集が並行する
-- soft delete を復活させることも、それと競合することもない。
UPDATE reviews
SET rating = $2,
    comment = $3,
    updated_at = now()
WHERE id = $1 AND discarded_at IS NULL
RETURNING *;

-- name: UpdateReviewPhotoKey :one
-- 列を限定した写真の差し替え（S10）：photo_key だけを更新し
-- （rating/comment/discarded_at は決して更新しない）、review がまだ kept な
-- 間だけ更新する。photo_key は burger_stats に影響しないので、
-- 再計算は不要である。
UPDATE reviews
SET photo_key = $2,
    updated_at = now()
WHERE id = $1 AND discarded_at IS NULL
RETURNING *;

-- name: DiscardReview :one
-- 列を限定した soft delete：discarded_at にタイムスタンプを設定するだけで、
-- しかも 1 回だけ行う。すでに discard 済みの review はどの行にもマッチせず、
-- not found として現れる。burger_id を返すので、呼び出し元は同じ
-- トランザクション内でその burger の統計を再計算できる。
UPDATE reviews
SET discarded_at = now(),
    updated_at = now()
WHERE id = $1 AND discarded_at IS NULL
RETURNING burger_id;
