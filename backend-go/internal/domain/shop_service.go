package domain

import "context"

// ShopRepository は shop の書き込みの契約である。domain が宣言し、呼び出すのは
// domain の ShopService だけで、usecase は呼ばない（読み取りは usecase の
// ShopQuery）。実装は smallint の status のエンコードを自分の内部に留め、
// id に一致する shop がないときは（wrap された）ErrShopNotFound を返す。
// 書き込み専用で、読み取りのメソッドは置かない。
type ShopRepository interface {
	// CreateShop は新しい shop を永続化し、生成された id つきで返す。
	CreateShop(ctx context.Context, shop Shop) (Shop, error)
	// UpdateShopName は、id の shop の name だけを永続化し、保存された行を
	// 返す。カラム限定なので、並行する status の変更が古いスナップショットで
	// 元に戻されることは決してない。
	UpdateShopName(ctx context.Context, id int64, name string) (Shop, error)
	// UpdateShopStatus は、id の shop の status と moderation note だけを
	// 永続化し、保存された行を返す。カラム限定なので、並行する rename が
	// 古いスナップショットで元に戻されることは決してない。
	UpdateShopStatus(ctx context.Context, id int64, status ShopStatus, note *string) (Shop, error)
}

// ShopService は shop の書き込みの窓口である。ShopRepository を呼ぶのは domain の
// この型だけで、usecase は repository に依存せず、書き込みをここに任せる。
// 現時点では repository の書き込みを 1 対 1 で包む窓口にすぎない。domain の手順が増えたときに、
// usecase ではなくここへ置く。
type ShopService struct {
	repo ShopRepository
}

// NewShopService は repo を使う ShopService を返す。
func NewShopService(repo ShopRepository) *ShopService {
	return &ShopService{repo: repo}
}

// Create は新しい shop を永続化し、生成された id つきで返す。
func (s *ShopService) Create(ctx context.Context, shop Shop) (Shop, error) {
	return s.repo.CreateShop(ctx, shop)
}

// UpdateName は、id の shop の name だけを永続化し、保存された行を返す。
func (s *ShopService) UpdateName(ctx context.Context, id int64, name string) (Shop, error) {
	return s.repo.UpdateShopName(ctx, id, name)
}

// UpdateStatus は、id の shop の status と moderation note だけを永続化し、
// 保存された行を返す。
func (s *ShopService) UpdateStatus(ctx context.Context, id int64, status ShopStatus, note *string) (Shop, error) {
	return s.repo.UpdateShopStatus(ctx, id, status, note)
}
