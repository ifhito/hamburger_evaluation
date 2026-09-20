package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository/sqlcgen"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// ShopRepository は、sqlc 生成のクエリ上で usecase.ShopRepository を実装する。
// domain の visibility 記述子を SQL パラメータに変換し、smallint の status
// コードを domain.ShopStatus に対応づける。visibility のルール自体は
// domain.ShopVisibility にある。
type ShopRepository struct {
	q *sqlcgen.Queries
}

// NewShopRepository は db（通常は共有の pgx pool）をラップする。
func NewShopRepository(db sqlcgen.DBTX) *ShopRepository {
	return &ShopRepository{q: sqlcgen.New(db)}
}

var _ usecase.ShopRepository = (*ShopRepository)(nil)

// likeEscaper は LIKE のメタ文字をエスケープする（最初にバックスラッシュ）。
// これにより、user の keyword は ILIKE パターンの中で常にリテラルとして
// 一致する。
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// ListShops は、keyword に一致する可視の shop を name、id の順に並べて返す。
// keyword はエスケープ済みの ILIKE パラメータとして渡され、SQL に連結される
// ことはない。
func (r *ShopRepository) ListShops(ctx context.Context, vis domain.ShopVisibility, keyword string, limit, offset int32) ([]domain.Shop, error) {
	params := sqlcgen.ListShopsParams{
		ViewAll:    vis.ViewAll,
		PageLimit:  limit,
		PageOffset: offset,
	}
	if vis.ViewerID != nil {
		params.ViewerID = pgtype.Int8{Int64: *vis.ViewerID, Valid: true}
	}
	if keyword != "" {
		params.NamePattern = pgtype.Text{String: "%" + likeEscaper.Replace(keyword) + "%", Valid: true}
	}
	rows, err := r.q.ListShops(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list shops: %w", err)
	}
	shops := make([]domain.Shop, 0, len(rows))
	for _, row := range rows {
		shop, err := toDomainShop(row.ID, row.Name, row.Status, row.ModerationNote, row.CreatorID)
		if err != nil {
			return nil, fmt.Errorf("list shops: %w", err)
		}
		shops = append(shops, shop)
	}
	return shops, nil
}

// GetShopWithCreator は shop とその creator を返す（Reviews は空のまま）。
// または domain.ErrShopNotFound を返す。
func (r *ShopRepository) GetShopWithCreator(ctx context.Context, id int64) (domain.ShopDetail, error) {
	row, err := r.q.GetShopWithCreator(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ShopDetail{}, fmt.Errorf("get shop with creator: %w", domain.ErrShopNotFound)
		}
		return domain.ShopDetail{}, fmt.Errorf("get shop with creator: %w", err)
	}
	shop, err := toDomainShop(row.ID, row.Name, row.Status, row.ModerationNote, row.CreatorID)
	if err != nil {
		return domain.ShopDetail{}, fmt.Errorf("get shop with creator: %w", err)
	}
	detail := domain.ShopDetail{Shop: shop}
	if row.CreatorID.Valid {
		// users.id は外部キーなので、LEFT JOIN で creator が見つかっている。
		detail.Creator = &domain.UserRef{ID: row.CreatorID.Int64, Username: row.CreatorUsername.String}
	}
	return detail, nil
}

// ListShopReviews は、shop の discard されていない review を author、
// burger、stats とともに新しい順に返す。単一の JOIN クエリである（N+1 なし）。
func (r *ShopRepository) ListShopReviews(ctx context.Context, shopID int64) ([]domain.ShopReview, error) {
	rows, err := r.q.ListShopReviews(ctx, shopID)
	if err != nil {
		return nil, fmt.Errorf("list shop reviews: %w", err)
	}
	reviews := make([]domain.ShopReview, 0, len(rows))
	for _, row := range rows {
		review := domain.ShopReview{
			ID:        row.ID,
			Rating:    int(row.Rating),
			CreatedAt: row.CreatedAt.Time,
			User:      &domain.UserRef{ID: row.UserID, Username: row.UserUsername},
			Burger: &domain.ShopReviewBurger{
				ID:   row.BurgerID,
				Name: row.BurgerName,
				// stats 行がまだ存在しないことがある。その場合はゼロ値になる。
				AverageRating: row.AverageRating.Float64,
				ReviewCount:   row.ReviewCount.Int64,
				WeightedScore: row.WeightedScore.Float64,
				Confidence:    row.Confidence.Float64,
			},
		}
		if row.Comment.Valid {
			comment := row.Comment.String
			review.Comment = &comment
		}
		reviews = append(reviews, review)
	}
	return reviews, nil
}

// CreateShop は（検証済みの）shop を insert し、生成された id を持つ shop を
// 返す。
func (r *ShopRepository) CreateShop(ctx context.Context, shop domain.Shop) (domain.Shop, error) {
	code, err := statusCode(shop.Status)
	if err != nil {
		return domain.Shop{}, fmt.Errorf("create shop: %w", err)
	}
	row, err := r.q.CreateShop(ctx, sqlcgen.CreateShopParams{
		Name:           shop.Name,
		Status:         code,
		ModerationNote: textOrNull(shop.ModerationNote),
		CreatorID:      int8OrNull(shop.CreatorID),
	})
	if err != nil {
		return domain.Shop{}, fmt.Errorf("create shop: %w", err)
	}
	created, err := toDomainShop(row.ID, row.Name, row.Status, row.ModerationNote, row.CreatorID)
	if err != nil {
		return domain.Shop{}, fmt.Errorf("create shop: %w", err)
	}
	return created, nil
}

// ListShopsForModeration は、すべての shop をその creator とともに新しい順
// （created_at desc、id desc）に返す。任意で 1 つの status に絞り込める。
func (r *ShopRepository) ListShopsForModeration(ctx context.Context, status *domain.ShopStatus) ([]domain.ShopDetail, error) {
	var filter pgtype.Int2
	if status != nil {
		code, err := statusCode(*status)
		if err != nil {
			return nil, fmt.Errorf("list shops for moderation: %w", err)
		}
		filter = pgtype.Int2{Int16: code, Valid: true}
	}
	rows, err := r.q.ListShopsForModeration(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("list shops for moderation: %w", err)
	}
	details := make([]domain.ShopDetail, 0, len(rows))
	for _, row := range rows {
		shop, err := toDomainShop(row.ID, row.Name, row.Status, row.ModerationNote, row.CreatorID)
		if err != nil {
			return nil, fmt.Errorf("list shops for moderation: %w", err)
		}
		detail := domain.ShopDetail{Shop: shop}
		if row.CreatorID.Valid {
			// users.id は外部キーなので、LEFT JOIN で creator が見つかっている。
			detail.Creator = &domain.UserRef{ID: row.CreatorID.Int64, Username: row.CreatorUsername.String}
		}
		details = append(details, detail)
	}
	return details, nil
}

// UpdateShopName は、id の shop の name だけを永続化し、保存された行を返す。
// 読み取りから書き込みまでの間に shop が消えた場合は domain.ErrShopNotFound
// を返す。単一のカラムだけを書くことで、同時に行われた status の変更が古い
// スナップショットによって元に戻されるのを防ぐ。
func (r *ShopRepository) UpdateShopName(ctx context.Context, id int64, name string) (domain.Shop, error) {
	row, err := r.q.UpdateShopName(ctx, sqlcgen.UpdateShopNameParams{ID: id, Name: name})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Shop{}, fmt.Errorf("update shop name: %w", domain.ErrShopNotFound)
		}
		return domain.Shop{}, fmt.Errorf("update shop name: %w", err)
	}
	updated, err := toDomainShop(row.ID, row.Name, row.Status, row.ModerationNote, row.CreatorID)
	if err != nil {
		return domain.Shop{}, fmt.Errorf("update shop name: %w", err)
	}
	return updated, nil
}

// UpdateShopStatus は、id の shop の status と moderation note だけを
// 永続化し、保存された行を返す。読み取りから書き込みまでの間に shop が
// 消えた場合は domain.ErrShopNotFound を返す。name に触れないことで、
// 同時に行われた rename が古いスナップショットによって元に戻されるのを防ぐ。
func (r *ShopRepository) UpdateShopStatus(ctx context.Context, id int64, status domain.ShopStatus, note *string) (domain.Shop, error) {
	code, err := statusCode(status)
	if err != nil {
		return domain.Shop{}, fmt.Errorf("update shop status: %w", err)
	}
	row, err := r.q.UpdateShopStatus(ctx, sqlcgen.UpdateShopStatusParams{
		ID:             id,
		Status:         code,
		ModerationNote: textOrNull(note),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Shop{}, fmt.Errorf("update shop status: %w", domain.ErrShopNotFound)
		}
		return domain.Shop{}, fmt.Errorf("update shop status: %w", err)
	}
	updated, err := toDomainShop(row.ID, row.Name, row.Status, row.ModerationNote, row.CreatorID)
	if err != nil {
		return domain.Shop{}, fmt.Errorf("update shop status: %w", err)
	}
	return updated, nil
}

// textOrNull は、省略可能な string を null 許容な pgx の形式に変換する。
func textOrNull(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *s, Valid: true}
}

// int8OrNull は、省略可能な int64 を null 許容な pgx の形式に変換する。
func int8OrNull(n *int64) pgtype.Int8 {
	if n == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *n, Valid: true}
}

// statusCode は domain.ShopStatus を smallint の保存用コードにエンコードする。
// toDomainShop の逆である。この対応づけがこのパッケージの外に出ることはない。
func statusCode(status domain.ShopStatus) (int16, error) {
	switch status {
	case domain.ShopStatusPending:
		return 0, nil
	case domain.ShopStatusActive:
		return 1, nil
	case domain.ShopStatusRejected:
		return 2, nil
	default:
		// 呼び出し側が domain の定数からしか status を作らない限り到達
		// しない。ゴミを永続化する代わりに fail loudly する。
		return 0, fmt.Errorf("unknown shop status %q", status)
	}
}

// toDomainShop は sqlc の shop のカラムを domain のエンティティに変換し、
// smallint の status をデコードする（0=pending、1=active、2=rejected）。
func toDomainShop(id int64, name string, status int16, note pgtype.Text, creatorID pgtype.Int8) (domain.Shop, error) {
	shop := domain.Shop{ID: id, Name: name}
	switch status {
	case 0:
		shop.Status = domain.ShopStatusPending
	case 1:
		shop.Status = domain.ShopStatusActive
	case 2:
		shop.Status = domain.ShopStatusRejected
	default:
		// CHECK 制約が保たれている限り到達しない。保たれていなければ
		// fail loudly する。
		return domain.Shop{}, fmt.Errorf("shop %d: unknown status code %d", id, status)
	}
	if note.Valid {
		n := note.String
		shop.ModerationNote = &n
	}
	if creatorID.Valid {
		c := creatorID.Int64
		shop.CreatorID = &c
	}
	return shop, nil
}
