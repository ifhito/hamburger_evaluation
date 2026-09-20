package domain

import "context"

// ReviewRepository は review の書き込みの契約である。domain が宣言し、呼び出すのは
// domain の ReviewService だけで、usecase は呼ばない（読み取りは usecase の
// ReviewQuery）。実装は書き込みの SQL の詳細（カラム限定の書き込み、
// burger_stats の再計算）を自分の内部に留め、一致する行がないときは
// （wrap された）ErrReviewNotFound を返す。書き込み専用で、読み取りのメソッドは
// 置かない。
type ReviewRepository interface {
	// CreateReview は、新しい（validate 済みの）review を永続化し、生成された
	// id と created_at つきで返す。この書き込みは、同一 transaction 内で
	// burger の burger_stats も再計算する（issue #15、S7）。
	CreateReview(ctx context.Context, review Review) (Review, error)
	// CreateReviewForNamedBurger は、新しい（validate 済みの）review を、
	// shop の burger のうち指定された名前と完全一致するものに対して永続化する。
	// shop にその名前の burger がなければ、burger とその shops_burgers の
	// リンクを作成する（Rails の find_or_create_burger、S6 P3-1）。review の
	// BurgerID の入力は無視され、解決された burger に設定される。
	// find-or-create、review の insert、burger_stats の再計算は「1 つの」
	// transaction 内で行われるので、insert が失敗しても孤立した burger や
	// リンクは残らない。返される burger は、insert 前に保存されていた stats を
	// 持つ。これは burger_id の経路で usecase の ReviewQuery.GetShopBurger が
	// 返すものとまったく同じである。まったく新しい burger の stats はゼロである。
	CreateReviewForNamedBurger(ctx context.Context, shopID int64, burgerName string, review Review) (Review, ShopReviewBurger, error)
	// UpdateReviewContent は、id の、まだ kept な review の rating と comment
	// だけを永続化し、保存された行を返す。存在しないか discard 済みのときは
	// （wrap された）ErrReviewNotFound を返す。カラム限定の書き込み
	// なので、discarded_at が書き込まれることは決してない。この書き込みは、
	// 同一 transaction 内で burger の burger_stats も再計算する。
	UpdateReviewContent(ctx context.Context, id int64, rating int, comment string) (Review, error)
	// UpdateReviewContentAndPhotoKey は、id の、まだ kept な review の
	// rating、comment、「および」photo_key を atomic に永続化する。カラム限定の
	// 2 つの書き込みと burger_stats の再計算が「1 つの」transaction を共有する
	// ので、写真つきの編集が content だけを key なしで commit してしまうことは
	// 決してない（S10 の review fix）。保存された行を返すか、review が存在しない
	// か discard 済みのときは（wrap された）ErrReviewNotFound を返す
	// （その場合は何も commit されない）。
	UpdateReviewContentAndPhotoKey(ctx context.Context, id int64, rating int, comment string, photoKey *string) (Review, error)
	// DiscardReview は review を soft delete する（discarded_at を記録し、
	// hard DELETE は決して行わない）。存在しないか、すでに discard 済みの
	// ときは（wrap された）ErrReviewNotFound を返す。この書き込みは、
	// 同一 transaction 内で burger の burger_stats も再計算する。
	DiscardReview(ctx context.Context, id int64) error
}

// ReviewService は review の書き込みの窓口である。ReviewRepository を呼ぶのは
// domain のこの型だけで、usecase は repository に依存せず、書き込みをここに任せる。
// 現時点では repository の書き込みを 1 対 1 で包む窓口にすぎない。burger_stats の再計算は
// repository の同一 transaction の内部にあり、ここでは行わない。domain の手順が増えたときに、
// usecase ではなくここへ置く。
type ReviewService struct {
	repo ReviewRepository
}

// NewReviewService は repo を使う ReviewService を返す。
func NewReviewService(repo ReviewRepository) *ReviewService {
	return &ReviewService{repo: repo}
}

// Create は、新しい（validate 済みの）review を永続化し、生成された id と
// created_at つきで返す。
func (s *ReviewService) Create(ctx context.Context, review Review) (Review, error) {
	return s.repo.CreateReview(ctx, review)
}

// CreateForNamedBurger は、新しい（validate 済みの）review を、shop の burger の
// うち指定された名前と完全一致するものに対して永続化する（なければ burger を作る）。
// 返される burger は、insert 前に保存されていた stats を持つ。
func (s *ReviewService) CreateForNamedBurger(ctx context.Context, shopID int64, burgerName string, review Review) (Review, ShopReviewBurger, error) {
	return s.repo.CreateReviewForNamedBurger(ctx, shopID, burgerName, review)
}

// UpdateContent は、id の、まだ kept な review の rating と comment だけを永続化し、
// 保存された行を返す。
func (s *ReviewService) UpdateContent(ctx context.Context, id int64, rating int, comment string) (Review, error) {
	return s.repo.UpdateReviewContent(ctx, id, rating, comment)
}

// UpdateContentAndPhotoKey は、id の、まだ kept な review の rating、comment、
// および photo_key を atomic に永続化し、保存された行を返す。
func (s *ReviewService) UpdateContentAndPhotoKey(ctx context.Context, id int64, rating int, comment string, photoKey *string) (Review, error) {
	return s.repo.UpdateReviewContentAndPhotoKey(ctx, id, rating, comment, photoKey)
}

// Discard は review を soft delete する。
func (s *ReviewService) Discard(ctx context.Context, id int64) error {
	return s.repo.DiscardReview(ctx, id)
}
