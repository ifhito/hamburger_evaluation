package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository/sqlcgen"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// ReviewRepository は、sqlc 生成のクエリ上で usecase.ReviewRepository を
// 実装する。soft delete の述語（discarded_at IS NULL）と、active な shop に
// 対する EXISTS フィルタは SQL 側にあり、認可ルール自体は domain パッケージに
// ある。すべての書き込み（create、edit、discard）は、同じトランザクション内で、
// 影響を受ける burger の burger_stats 行も再計算し、LockBurgerForStats により
// burger ごとに直列化される。
type ReviewRepository struct {
	db beginnerDBTX
	q  *sqlcgen.Queries
}

// NewReviewRepository は db（通常は共有の pgx pool）をラップする。
func NewReviewRepository(db beginnerDBTX) *ReviewRepository {
	return &ReviewRepository{db: db, q: sqlcgen.New(db)}
}

var _ usecase.ReviewRepository = (*ReviewRepository)(nil)

// ListReviews は公開 review フィードを返す。対象は、discard されておらず、
// かつ author（user）も discard されていない review のうち、burger が少なくとも
// 1 つの active な shop に紐づいているものに限る。filter で絞り込まれ、その意味は
// Rails の ReviewQuery に対応する（ただし shop の絞り込みは、対象の shop 自身も
// active であることを要求する点で Rails より厳しい）。author、burger、stats を
// 1 回のクエリで取得し（N+1 なし）、新しい順（created_at desc、id desc）に並ぶ。
// keyword は likeEscaper を通して ILIKE パラメータに渡され、SQL に連結される
// ことはない。指定のない filter は NULL のままである。
func (r *ReviewRepository) ListReviews(ctx context.Context, filter usecase.ReviewListFilter, limit, offset int32) ([]domain.ReviewDetail, error) {
	params := sqlcgen.ListPublicReviewsParams{
		PageLimit:  limit,
		PageOffset: offset,
	}
	if filter.Rating != nil {
		params.FilterRating = pgtype.Int8{Int64: int64(*filter.Rating), Valid: true}
	}
	if filter.Keyword != "" {
		params.CommentPattern = pgtype.Text{String: "%" + likeEscaper.Replace(filter.Keyword) + "%", Valid: true}
	}
	if filter.ShopID != nil {
		params.FilterShopID = pgtype.Int8{Int64: *filter.ShopID, Valid: true}
	}
	rows, err := r.q.ListPublicReviews(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list reviews: %w", err)
	}
	reviews := make([]domain.ReviewDetail, 0, len(rows))
	for _, row := range rows {
		reviews = append(reviews, toReviewDetail(
			row.ID, row.Rating, row.Comment, row.PhotoKey, row.CreatedAt,
			row.UserID, row.UserUsername, row.BurgerID, row.BurgerName,
			row.ReviewCount, row.AverageRating, row.WeightedScore, row.Confidence,
		))
	}
	return reviews, nil
}

// GetReview は、discard されていない review 1 件を author、burger、stats
// とともに返す。または domain.ErrReviewNotFound を返す。author（user）が
// discard 済みの review も対象外である。SQL は、存在しない行、discard 済みの
// review、author が discard 済みの review をすべて同一に扱う。
func (r *ReviewRepository) GetReview(ctx context.Context, id int64) (domain.ReviewDetail, error) {
	row, err := r.q.GetReviewDetail(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ReviewDetail{}, fmt.Errorf("get review: %w", domain.ErrReviewNotFound)
		}
		return domain.ReviewDetail{}, fmt.Errorf("get review: %w", err)
	}
	return toReviewDetail(
		row.ID, row.Rating, row.Comment, row.PhotoKey, row.CreatedAt,
		row.UserID, row.UserUsername, row.BurgerID, row.BurgerName,
		row.ReviewCount, row.AverageRating, row.WeightedScore, row.Confidence,
	), nil
}

// GetShop は素の shop 行を返す。または domain.ErrShopNotFound を返す。
func (r *ReviewRepository) GetShop(ctx context.Context, id int64) (domain.Shop, error) {
	row, err := r.q.GetShop(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Shop{}, fmt.Errorf("get shop: %w", domain.ErrShopNotFound)
		}
		return domain.Shop{}, fmt.Errorf("get shop: %w", err)
	}
	shop, err := toDomainShop(row.ID, row.Name, row.Status, row.ModerationNote, row.CreatorID)
	if err != nil {
		return domain.Shop{}, fmt.Errorf("get shop: %w", err)
	}
	return shop, nil
}

// GetShopBurger は、burger が shops_burgers 経由でその shop に紐づいている
// とき、stats つきの burger を返す。そうでなければ domain.ErrBurgerNotFound
// を返す。存在しない burger と別の shop の burger は区別できない。
func (r *ReviewRepository) GetShopBurger(ctx context.Context, shopID, burgerID int64) (domain.ShopReviewBurger, error) {
	row, err := r.q.GetShopBurgerWithStats(ctx, sqlcgen.GetShopBurgerWithStatsParams{
		ShopID:   shopID,
		BurgerID: burgerID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ShopReviewBurger{}, fmt.Errorf("get shop burger: %w", domain.ErrBurgerNotFound)
		}
		return domain.ShopReviewBurger{}, fmt.Errorf("get shop burger: %w", err)
	}
	return domain.ShopReviewBurger{
		ID:   row.ID,
		Name: row.Name,
		// stats 行がまだ存在しないことがある。その場合はゼロ値になる。
		AverageRating: row.AverageRating.Float64,
		ReviewCount:   row.ReviewCount.Int64,
		WeightedScore: row.WeightedScore.Float64,
		Confidence:    row.Confidence.Float64,
	}, nil
}

// CreateReview は（検証済みの）review を insert し、生成された id と
// created_at を持つ review を返す。insert と burger_stats の再計算は 1 つの
// トランザクションで行われるので、stats が review に遅れることも、review より
// 長く残ることも決してない。
func (r *ReviewRepository) CreateReview(ctx context.Context, review domain.Review) (domain.Review, error) {
	var row sqlcgen.Review
	err := withTx(ctx, r.db, "create review", func(q *sqlcgen.Queries) error {
		var err error
		row, err = insertReviewAndRecalc(ctx, q, review, "create review")
		return err
	})
	if err != nil {
		return domain.Review{}, err
	}
	return toDomainReview(row), nil
}

// CreateReviewForNamedBurger は、（検証済みの）review を、名前が burgerName
// と完全に一致する shop の burger に対して insert する。shop にその名前の
// burger がないときは、burger とその shops_burgers の link を作成する
// （Rails の find_or_create_burger、S6 P3-1）。find-or-create、review の
// insert、burger_stats の再計算はただ 1 つのトランザクションを共有するので、
// どの段階で失敗しても、孤立した burger や link が commit されることはない。
// (shop, name) には unique index が意図的に存在しない。同じ新しい名前を同時に
// 作成する 2 つの creator が、両方とも burger を insert しうる。これは Rails の
// find_or_create_burger にもある同じ race であり、parity であってバグでは
// ない。返される burger は insert 前の stats を持ち、burger_id 経路での
// GetShopBurger とまったく同じである（まったく新しい burger ではゼロ）。
func (r *ReviewRepository) CreateReviewForNamedBurger(ctx context.Context, shopID int64, burgerName string, review domain.Review) (domain.Review, domain.ShopReviewBurger, error) {
	var row sqlcgen.Review
	var burger domain.ShopReviewBurger
	err := withTx(ctx, r.db, "create review for named burger", func(q *sqlcgen.Queries) error {
		found, err := q.GetShopBurgerByNameWithStats(ctx, sqlcgen.GetShopBurgerByNameWithStatsParams{
			ShopID: shopID,
			Name:   burgerName,
		})
		switch {
		case err == nil:
			burger = domain.ShopReviewBurger{
				ID:   found.ID,
				Name: found.Name,
				// stats 行がまだ存在しないことがある。その場合はゼロ値になる。
				AverageRating: found.AverageRating.Float64,
				ReviewCount:   found.ReviewCount.Int64,
				WeightedScore: found.WeightedScore.Float64,
				Confidence:    found.Confidence.Float64,
			}
		case errors.Is(err, pgx.ErrNoRows):
			created, err := q.CreateBurger(ctx, burgerName)
			if err != nil {
				return fmt.Errorf("create review for named burger: create burger: %w", err)
			}
			if err := q.CreateShopBurger(ctx, sqlcgen.CreateShopBurgerParams{ShopID: shopID, BurgerID: created.ID}); err != nil {
				return fmt.Errorf("create review for named burger: link burger: %w", err)
			}
			burger = domain.ShopReviewBurger{ID: created.ID, Name: created.Name}
		default:
			return fmt.Errorf("create review for named burger: find burger: %w", err)
		}
		review.BurgerID = burger.ID
		row, err = insertReviewAndRecalc(ctx, q, review, "create review for named burger")
		return err
	})
	if err != nil {
		return domain.Review{}, domain.ShopReviewBurger{}, err
	}
	return toDomainReview(row), burger, nil
}

// UpdateReviewContent は、id の、まだ kept な review の rating と comment
// だけを永続化し、保存された行を返す。存在しないか discard 済みの場合は
// domain.ErrReviewNotFound を返す（トランザクションは rollback されるので、
// stats は変更されない）。カラム単位に限定される：discarded_at は決して
// 書き込まれないので、edit が soft delete を復活させることも、soft delete と
// race することもない。update と burger_stats の再計算は 1 つの
// トランザクションで行われる。
func (r *ReviewRepository) UpdateReviewContent(ctx context.Context, id int64, rating int, comment string) (domain.Review, error) {
	var row sqlcgen.Review
	err := withTx(ctx, r.db, "update review content", func(q *sqlcgen.Queries) error {
		var err error
		row, err = q.UpdateReviewContent(ctx, sqlcgen.UpdateReviewContentParams{
			ID:      id,
			Rating:  int16(rating),
			Comment: pgtype.Text{String: comment, Valid: true},
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("update review content: %w", domain.ErrReviewNotFound)
			}
			return fmt.Errorf("update review content: %w", err)
		}
		if err := recalculateBurgerStats(ctx, q, row.BurgerID); err != nil {
			return fmt.Errorf("update review content: %w", err)
		}
		return nil
	})
	if err != nil {
		return domain.Review{}, err
	}
	return toDomainReview(row), nil
}

// UpdateReviewContentAndPhotoKey は、id の、まだ kept な review の rating、
// comment、photo_key を永続化し、存在しないか discard 済みの場合は
// domain.ErrReviewNotFound を返す。既存のカラム単位の 2 つのステートメント
// （UpdateReviewContent、続いて UpdateReviewPhotoKey）と burger_stats の
// 再計算はただ 1 つのトランザクションで実行されるので、photo を伴う edit は、
// content、key、stats をまとめて commit するか、何も commit しないかの
// どちらかになる。2 番目のステートメントが返す行には、最初のステートメントの
// rating/comment がすでに反映されている（同一トランザクション）。
func (r *ReviewRepository) UpdateReviewContentAndPhotoKey(ctx context.Context, id int64, rating int, comment string, photoKey *string) (domain.Review, error) {
	var row sqlcgen.Review
	err := withTx(ctx, r.db, "update review content and photo key", func(q *sqlcgen.Queries) error {
		if _, err := q.UpdateReviewContent(ctx, sqlcgen.UpdateReviewContentParams{
			ID:      id,
			Rating:  int16(rating),
			Comment: pgtype.Text{String: comment, Valid: true},
		}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("update review content and photo key: %w", domain.ErrReviewNotFound)
			}
			return fmt.Errorf("update review content and photo key: %w", err)
		}
		var err error
		row, err = q.UpdateReviewPhotoKey(ctx, sqlcgen.UpdateReviewPhotoKeyParams{
			ID:       id,
			PhotoKey: textOrNull(photoKey),
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("update review content and photo key: %w", domain.ErrReviewNotFound)
			}
			return fmt.Errorf("update review content and photo key: %w", err)
		}
		if err := recalculateBurgerStats(ctx, q, row.BurgerID); err != nil {
			return fmt.Errorf("update review content and photo key: %w", err)
		}
		return nil
	})
	if err != nil {
		return domain.Review{}, err
	}
	return toDomainReview(row), nil
}

// DiscardReview は review を soft delete する（discarded_at に時刻を刻み、
// hard DELETE は決して行わない）。存在しない review や、すでに discard 済みの
// review はどの行にも一致せず、domain.ErrReviewNotFound を返す（トランザクション
// は rollback されるので、stats は変更されない）。discard と
// burger_stats の再計算は 1 つのトランザクションで行われる。
func (r *ReviewRepository) DiscardReview(ctx context.Context, id int64) error {
	return withTx(ctx, r.db, "discard review", func(q *sqlcgen.Queries) error {
		burgerID, err := q.DiscardReview(ctx, id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("discard review: %w", domain.ErrReviewNotFound)
			}
			return fmt.Errorf("discard review: %w", err)
		}
		if err := recalculateBurgerStats(ctx, q, burgerID); err != nil {
			return fmt.Errorf("discard review: %w", err)
		}
		return nil
	})
}

// insertReviewAndRecalc は、2 つの create 経路に共通する末尾処理である。
// burger をロックし、review を insert し、その burger_stats を再計算する。
// 呼び出し側のトランザクション内で実行される。q は tx スコープでなければ
// ならず、この helper が自前のトランザクションを開くことは決してない。
// ロックは insert の「前」に取る。insert の FK チェックが burgers 行に
// KEY SHARE ロックを取り、その後でそれを FOR UPDATE に昇格させると、同時に
// 走る 2 つの creator がデッドロックしうるからである。こうして先に取って
// おけば、recalculateBurgerStats 自身のロックはコストのかからない再取得に
// なる（PostgreSQL では行ロックはトランザクションが所有する）。op はエラー
// メッセージの接頭辞になり、各呼び出し箇所の文言を保つ。
func insertReviewAndRecalc(ctx context.Context, q *sqlcgen.Queries, review domain.Review, op string) (sqlcgen.Review, error) {
	if _, err := q.LockBurgerForStats(ctx, review.BurgerID); err != nil {
		return sqlcgen.Review{}, fmt.Errorf("%s: lock burger: %w", op, err)
	}
	row, err := q.CreateReview(ctx, sqlcgen.CreateReviewParams{
		Rating:   int16(review.Rating),
		Comment:  textOrNull(review.Comment),
		UserID:   review.AuthorID,
		BurgerID: review.BurgerID,
		PhotoKey: textOrNull(review.PhotoKey),
	})
	if err != nil {
		return sqlcgen.Review{}, fmt.Errorf("%s: %w", op, err)
	}
	if err := recalculateBurgerStats(ctx, q, review.BurgerID); err != nil {
		return sqlcgen.Review{}, fmt.Errorf("%s: %w", op, err)
	}
	return row, nil
}

// recalculateBurgerStats は、burger の stats 行を、その burger の kept な review
// のうち author（user）が discard されていないもの（ListBurgerReviewFacts）から、
// domain の calculator を通して再計算し upsert する。呼び出し側の
// トランザクション内で実行され、q は tx スコープでなければならない。この
// helper 自身が、最初に LockBurgerForStats を通じて burger ごとの FOR UPDATE
// ロックを取る（lost-update の根拠はそのクエリを参照）。トランザクションが
// すでに保持しているロックの再取得は no-op である。1 つのトランザクションで
// 「複数」の burger を再計算する呼び出し側（S8 の user discard フロー、
// UserRepository.DiscardUser）は、burger の集合が重なってもデッドロックしない
// ように、burger ごとに burger_id の昇順で呼び出さなければならない。対象の
// review がゼロ件でも、ゼロの行は upsert される（Rails BurgerScore.empty）。
func recalculateBurgerStats(ctx context.Context, q *sqlcgen.Queries, burgerID int64) error {
	if _, err := q.LockBurgerForStats(ctx, burgerID); err != nil {
		return fmt.Errorf("recalculate burger stats: lock burger: %w", err)
	}
	rows, err := q.ListBurgerReviewFacts(ctx, burgerID)
	if err != nil {
		return fmt.Errorf("recalculate burger stats: list facts: %w", err)
	}
	// 重複を除いた fact の author の reviewer-trust の履歴：各 author の、
	// すべての burger にわたる kept な rating を user ごとにまとめたもの。
	historyByUser := make(map[int64][]float64, len(rows))
	userIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		if _, seen := historyByUser[row.UserID]; !seen {
			historyByUser[row.UserID] = nil
			userIDs = append(userIDs, row.UserID)
		}
	}
	if len(userIDs) > 0 {
		ratings, err := q.ListReviewerRatings(ctx, userIDs)
		if err != nil {
			return fmt.Errorf("recalculate burger stats: list reviewer ratings: %w", err)
		}
		for _, rating := range ratings {
			historyByUser[rating.UserID] = append(historyByUser[rating.UserID], float64(rating.Rating))
		}
	}
	facts := make([]domain.ReviewFact, 0, len(rows))
	for _, row := range rows {
		facts = append(facts, domain.ReviewFact{
			Rating:          float64(row.Rating),
			CreatedAt:       row.CreatedAt.Time,
			ReviewerHistory: domain.ReviewerHistory{Ratings: historyByUser[row.UserID]},
		})
	}
	// マイクロ秒に切り詰める（timestamptz の精度）。これにより、保存される
	// calculated_at はスコアの計算に使った時刻そのものになる。テストは、
	// 保存された行とこのタイムスタンプからスコアを再計算する。
	now := time.Now().Truncate(time.Microsecond)
	score := domain.CalculateBurgerScore(facts, now)
	if _, err := q.UpsertBurgerStats(ctx, sqlcgen.UpsertBurgerStatsParams{
		BurgerID:      burgerID,
		ReviewCount:   int64(len(facts)),
		AverageRating: domain.AverageRating(facts),
		WeightedScore: score.WeightedAverage,
		Confidence:    score.Confidence,
		CalculatedAt:  pgtype.Timestamptz{Time: now, Valid: true},
	}); err != nil {
		return fmt.Errorf("recalculate burger stats: upsert: %w", err)
	}
	return nil
}

// toDomainReview は sqlc の review 行を domain のエンティティに変換する。
func toDomainReview(row sqlcgen.Review) domain.Review {
	review := domain.Review{
		ID:        row.ID,
		Rating:    int(row.Rating),
		AuthorID:  row.UserID,
		BurgerID:  row.BurgerID,
		CreatedAt: row.CreatedAt.Time,
	}
	if row.Comment.Valid {
		comment := row.Comment.String
		review.Comment = &comment
	}
	if row.PhotoKey.Valid {
		key := row.PhotoKey.String
		review.PhotoKey = &key
	}
	return review
}

// toReviewDetail は、結合された review のカラム（一覧クエリと詳細クエリで
// 共有される）を domain のペイロードに変換する。存在しない stats はゼロに
// なる。
func toReviewDetail(
	id int64, rating int16, comment, photoKey pgtype.Text, createdAt pgtype.Timestamptz,
	userID int64, username string, burgerID int64, burgerName string,
	reviewCount pgtype.Int8, averageRating, weightedScore, confidence pgtype.Float8,
) domain.ReviewDetail {
	detail := domain.ReviewDetail{
		Review: domain.Review{
			ID:        id,
			Rating:    int(rating),
			AuthorID:  userID,
			BurgerID:  burgerID,
			CreatedAt: createdAt.Time,
		},
		User: &domain.UserRef{ID: userID, Username: username},
		Burger: &domain.ShopReviewBurger{
			ID:   burgerID,
			Name: burgerName,
			// stats 行がまだ存在しないことがある。その場合はゼロ値になる。
			AverageRating: averageRating.Float64,
			ReviewCount:   reviewCount.Int64,
			WeightedScore: weightedScore.Float64,
			Confidence:    confidence.Float64,
		},
	}
	if comment.Valid {
		c := comment.String
		detail.Comment = &c
	}
	if photoKey.Valid {
		key := photoKey.String
		detail.PhotoKey = &key
	}
	return detail
}
