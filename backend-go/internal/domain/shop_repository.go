package domain

import "context"

// ShopRepository は shop の書き込みの契約である。domain が宣言し、呼び出すのは
// domain のコード（書き込みオブジェクトの Shops）だけで、usecase は呼ばない
// （読み取りは usecase の ShopQuery）。実装は smallint の status のエンコードを自分の内部に留め、
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

// Shops は shop 集約の書き込みオブジェクトである。ShopRepository を持つのは
// この型だけで、usecase は repository に依存せず、shop の書き込みをここに任せる。
// shop だけを更新する書き込みは、Service ではなくこの型に置く（Service は複数の
// 集約を跨ぐ更新だけに使う。domain/doc.go を参照）。現時点では repository の
// 書き込みを 1 対 1 で包んでいる。shop に関する domain の手順が増えたときは、
// usecase ではここへ置く。
type Shops struct {
	repo ShopRepository
}

// NewShops は repo を使う Shops を返す。
func NewShops(repo ShopRepository) *Shops {
	return &Shops{repo: repo}
}

// Create は新しい shop を永続化し、生成された id つきで返す。
func (s *Shops) Create(ctx context.Context, shop Shop) (Shop, error) {
	return s.repo.CreateShop(ctx, shop)
}

// UpdateName は、id の shop の name だけを永続化し、保存された行を返す。
func (s *Shops) UpdateName(ctx context.Context, id int64, name string) (Shop, error) {
	return s.repo.UpdateShopName(ctx, id, name)
}

// UpdateStatus は、id の shop の status と moderation note だけを永続化し、
// 保存された行を返す。
func (s *Shops) UpdateStatus(ctx context.Context, id int64, status ShopStatus, note *string) (Shop, error) {
	return s.repo.UpdateShopStatus(ctx, id, status, note)
}
