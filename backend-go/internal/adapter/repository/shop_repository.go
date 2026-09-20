package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository/sqlcgen"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/rowmap"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// ShopRepository は、sqlc 生成のクエリ上で usecase.ShopRepository を実装する。
// 書き込み（作成、name の更新、status の更新）だけを担い、読み取りは
// adapter/query の ShopQuery が担う。smallint の status コードと
// domain.ShopStatus との対応づけは rowmap にある。
type ShopRepository struct {
	q *sqlcgen.Queries
}

// NewShopRepository は db（通常は共有の pgx pool）をラップする。
func NewShopRepository(db sqlcgen.DBTX) *ShopRepository {
	return &ShopRepository{q: sqlcgen.New(db)}
}

var _ usecase.ShopRepository = (*ShopRepository)(nil)

// CreateShop は（検証済みの）shop を insert し、生成された id を持つ shop を
// 返す。
func (r *ShopRepository) CreateShop(ctx context.Context, shop domain.Shop) (domain.Shop, error) {
	code, err := rowmap.ShopStatusCode(shop.Status)
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
	created, err := rowmap.Shop(row.ID, row.Name, row.Status, row.ModerationNote, row.CreatorID)
	if err != nil {
		return domain.Shop{}, fmt.Errorf("create shop: %w", err)
	}
	return created, nil
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
	updated, err := rowmap.Shop(row.ID, row.Name, row.Status, row.ModerationNote, row.CreatorID)
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
	code, err := rowmap.ShopStatusCode(status)
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
	updated, err := rowmap.Shop(row.ID, row.Name, row.Status, row.ModerationNote, row.CreatorID)
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
