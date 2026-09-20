package usecase

import (
	"context"
	"fmt"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// ShopRepository は shops 向けの consumer 側の永続化の契約である。
// 実装は visibility の記述子を SQL のパラメータに変換し（ルール自体は
// domain.ShopVisibility にある）、smallint の status のエンコードを自分の
// 内部に留め、id に一致する shop がないときは（wrap された）
// domain.ErrShopNotFound を返す。
type ShopRepository interface {
	// ListShops は、keyword に一致する見える shop を、name、次に id の順で
	// 返す（keyword は name のリテラルな部分文字列で、大文字小文字を区別
	// しない。空ならすべてに一致する）。
	ListShops(ctx context.Context, vis domain.ShopVisibility, keyword string, limit, offset int32) ([]domain.Shop, error)
	// GetShopWithCreator は shop とその creator を返す。Reviews は空の
	// ままである。
	GetShopWithCreator(ctx context.Context, id int64) (domain.ShopDetail, error)
	// ListShopReviews は、shop の burger に対する discard されていない review
	// （author が discard 済みの user である review は除く）を、新しい順に
	// 返す（created_at desc、id desc）。
	ListShopReviews(ctx context.Context, shopID int64) ([]domain.ShopReview, error)
	// CreateShop は新しい shop を永続化し、生成された id つきで返す。
	CreateShop(ctx context.Context, shop domain.Shop) (domain.Shop, error)
	// ListShopsForModeration は、creator つきのすべての shop（Reviews は
	// 空のまま）を新しい順（created_at desc、id desc）に返す。任意で 1 つの
	// status に絞り込める（nil = すべて）。
	ListShopsForModeration(ctx context.Context, status *domain.ShopStatus) ([]domain.ShopDetail, error)
	// UpdateShopName は、id の shop の name だけを永続化し、保存された行を
	// 返す。カラム限定なので、並行する status の変更が古いスナップショットで
	// 元に戻されることは決してない。
	UpdateShopName(ctx context.Context, id int64, name string) (domain.Shop, error)
	// UpdateShopStatus は、id の shop の status と moderation note だけを
	// 永続化し、保存された行を返す。カラム限定なので、並行する rename が
	// 古いスナップショットで元に戻されることは決してない。
	UpdateShopStatus(ctx context.Context, id int64, status domain.ShopStatus, note *string) (domain.Shop, error)
}

// Shops は shop の use case を実装する。公開の一覧と詳細、ユーザーによる
// 投稿、そして admin による moderation である。
type Shops struct {
	repo ShopRepository
}

func NewShops(repo ShopRepository) *Shops { return &Shops{repo: repo} }

// List は、viewer（nil = 匿名）から見える shop のうち keyword に一致する
// ものを、ページネーションして返す。範囲外の page/perPage は、エラーにせず
// clampPage の規則で補正される（page < 1 は 1、perPage < 1 は 20、perPage の
// 上限は 100）。
func (s *Shops) List(ctx context.Context, viewer *domain.User, keyword string, page, perPage int) ([]domain.Shop, error) {
	limit, offset := clampPage(page, perPage)
	shops, err := s.repo.ListShops(ctx, domain.ShopVisibilityFor(viewer), keyword, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list shops: %w", err)
	}
	return shops, nil
}

// Get は、viewer が見てよいときに shop の詳細（creator と review を含む）を
// 返す。存在しない shop と隠された shop は、どちらも domain.ErrShopNotFound を
// 返すので、存在の有無は漏れない。
func (s *Shops) Get(ctx context.Context, viewer *domain.User, id int64) (domain.ShopDetail, error) {
	detail, err := s.repo.GetShopWithCreator(ctx, id)
	if err != nil {
		return domain.ShopDetail{}, fmt.Errorf("get shop: %w", err)
	}
	if !domain.ShopVisibilityFor(viewer).CanView(detail.Shop) {
		return domain.ShopDetail{}, domain.ErrShopNotFound
	}
	reviews, err := s.repo.ListShopReviews(ctx, id)
	if err != nil {
		return domain.ShopDetail{}, fmt.Errorf("list shop reviews: %w", err)
	}
	detail.Reviews = reviews
	return detail, nil
}

// Create は viewer に代わって新しい shop を投稿する。shop は pending で
// 始まり（後で moderator が activate する）、viewer が creator として
// 記録される。空白の name は、domain の *ValidationError をそのまま返す。
func (s *Shops) Create(ctx context.Context, viewer domain.User, name string) (domain.ShopDetail, error) {
	shop, err := domain.NewShopSubmission(name, viewer.ID)
	if err != nil {
		return domain.ShopDetail{}, err
	}
	created, err := s.repo.CreateShop(ctx, shop)
	if err != nil {
		return domain.ShopDetail{}, fmt.Errorf("create shop: %w", err)
	}
	// creator は viewer 自身なので、レスポンスの ref は、join で行を
	// 再取得せずに、ここで組み立てる。
	return domain.ShopDetail{
		Shop:    created,
		Creator: &domain.UserRef{ID: viewer.ID, Username: viewer.Username},
	}, nil
}

// AdminList は moderation 画面向けにすべての shop を新しい順で返し、
// API の status 文字列で任意に絞り込む。未知の status はエラーにならず、
// 何にも一致しない（Rails の where(status: unknown) と同様）。
// admin でない viewer には domain.ErrForbidden を返す。
func (s *Shops) AdminList(ctx context.Context, viewer domain.User, status string) ([]domain.ShopDetail, error) {
	if !viewer.Admin {
		return nil, domain.ErrForbidden
	}
	var filter *domain.ShopStatus
	if status != "" {
		switch st := domain.ShopStatus(status); st {
		case domain.ShopStatusPending, domain.ShopStatusActive, domain.ShopStatusRejected:
			filter = &st
		default:
			return []domain.ShopDetail{}, nil
		}
	}
	shops, err := s.repo.ListShopsForModeration(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("admin list shops: %w", err)
	}
	return shops, nil
}

// AdminUpdateName は shop の名前を変更する（唯一の moderation 編集、Rails
// parity）。admin でない viewer には、どの id が存在するかを探れないよう、
// lookup の前に domain.ErrForbidden を返す。空白の name は
// *ValidationError である。
func (s *Shops) AdminUpdateName(ctx context.Context, viewer domain.User, id int64, name string) (domain.ShopDetail, error) {
	if !viewer.Admin {
		return domain.ShopDetail{}, domain.ErrForbidden
	}
	if err := domain.ValidateShopName(name); err != nil {
		return domain.ShopDetail{}, err
	}
	// fetch はレスポンス用の creator（と、未知の id に対する 404）を
	// 供給する。書き込み自体は name のカラムにしか触れないので、並行する
	// status の変更を元に戻すことはない。
	detail, err := s.repo.GetShopWithCreator(ctx, id)
	if err != nil {
		return domain.ShopDetail{}, fmt.Errorf("admin update shop name: %w", err)
	}
	updated, err := s.repo.UpdateShopName(ctx, id, name)
	if err != nil {
		return domain.ShopDetail{}, fmt.Errorf("admin update shop name: %w", err)
	}
	detail.Shop = updated
	return detail, nil
}

// Approve は shop を activate し、moderation note を消去して（domain の
// 遷移）、公開して見えるようにする。admin でない viewer には、lookup の前に
// domain.ErrForbidden を返す。
func (s *Shops) Approve(ctx context.Context, viewer domain.User, id int64) (domain.ShopDetail, error) {
	if !viewer.Admin {
		return domain.ShopDetail{}, domain.ErrForbidden
	}
	return s.moderate(ctx, id, domain.Shop.Approve)
}

// Reject は、任意の moderation note つきで shop を reject し、公開の一覧から
// 隠す。admin でない viewer には、lookup の前に domain.ErrForbidden を返す。
func (s *Shops) Reject(ctx context.Context, viewer domain.User, id int64, note *string) (domain.ShopDetail, error) {
	if !viewer.Admin {
		return domain.ShopDetail{}, domain.ErrForbidden
	}
	return s.moderate(ctx, id, func(shop domain.Shop) domain.Shop {
		return shop.Reject(note)
	})
}

// moderate は status 遷移に共通のフローである。shop を creator とともに
// load し（レスポンスと 404 のため）、domain の遷移を適用し、その status と
// moderation note だけを永続化し、保存された行を持つ detail を返す。
// カラム限定の書き込みなので、古いスナップショットから並行する rename を
// 元に戻すことはない。
func (s *Shops) moderate(ctx context.Context, id int64, transition func(domain.Shop) domain.Shop) (domain.ShopDetail, error) {
	detail, err := s.repo.GetShopWithCreator(ctx, id)
	if err != nil {
		return domain.ShopDetail{}, fmt.Errorf("moderate shop: %w", err)
	}
	next := transition(detail.Shop)
	updated, err := s.repo.UpdateShopStatus(ctx, id, next.Status, next.ModerationNote)
	if err != nil {
		return domain.ShopDetail{}, fmt.Errorf("moderate shop: %w", err)
	}
	detail.Shop = updated
	return detail, nil
}
