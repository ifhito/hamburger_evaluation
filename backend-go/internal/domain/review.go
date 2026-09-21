package domain

import (
	"context"
	"strings"
	"time"
)

// Review は burger review の行を表す domain 上の表現である。
type Review struct {
	ID      int64
	Rating  int
	Comment *string
	// PhotoKey は review の写真の storage key であり、写真が添付されて
	// いないときは nil である（S10）。key はここでは不透明な値であり、URL は
	// usecase が photo storage を介して組み立てる。domain 自身が組み立てる
	// ことは決してない。
	PhotoKey *string
	AuthorID string
	// BurgerID は burger の UUID の正規形である。
	BurgerID  string
	CreatedAt time.Time
}

// MaxCommentChars はレビューのコメントの文字数の上限（Unicode のコードポイント数）である。
// DB の CHECK 制約 reviews_comment_max_length（000005_create_reviews）と同じ値でなければならない。
// 食い違いは db/migrations_test.go が検出する。変えるときは、この定数と、該当する CREATE TABLE の CHECK の両方を直す
// （実運用に入ったあとは、新しいマイグレーションで直す）。
const MaxCommentChars = 2000

// ValidateReviewContent は、書き込み可能な review の属性に対して Rails の
// validation を強制する。rating は 1..5 の整数でなければならず、comment は
// 存在しなければならない。失敗した場合は、Rails の full message そのままを
// *ValidationError に入れて返し、rating のメッセージが先に来る。
func ValidateReviewContent(rating int, comment string) error {
	var messages []string
	if rating < 1 || rating > 5 {
		messages = append(messages, "Rating must be in 1..5")
	}
	if strings.TrimSpace(comment) == "" {
		messages = append(messages, "Comment can't be blank")
	} else if exceedsChars(comment, MaxCommentChars) {
		messages = append(messages, tooLongMessage("Comment", MaxCommentChars))
	}
	if len(messages) > 0 {
		return &ValidationError{Messages: messages}
	}
	return nil
}

// MaxBurgerNameChars はバーガー名の文字数の上限（Unicode のコードポイント数）である。
// DB の CHECK 制約 burgers_name_max_length（000003_create_burgers）と同じ値でなければならない。
// 食い違いは db/migrations_test.go が検出する。変えるときは、この定数と、該当する CREATE TABLE の CHECK の両方を直す
// （実運用に入ったあとは、新しいマイグレーションで直す）。
const MaxBurgerNameChars = 100

// ValidateBurgerName は、burger_name による review 投稿の経路に対して Rails の
// Burger name の presence ルールを強制する。空またはホワイトスペースのみの
// 名前は拒否される。（Rails は空の名前に対して rescue されない RecordInvalid
// で応答するが、ここでは適切な validation failure とする。fail loud で 422
// を返す。）名前が MaxBurgerNameChars 文字を超えるときも拒否される。
func ValidateBurgerName(name string) error {
	if strings.TrimSpace(name) == "" {
		return &ValidationError{Messages: []string{"Burger name can't be blank"}}
	}
	if exceedsChars(name, MaxBurgerNameChars) {
		return &ValidationError{Messages: []string{tooLongMessage("Burger name", MaxBurgerNameChars)}}
	}
	return nil
}

// NewReview は、author が burger に対して投稿する validation 済みの新しい
// review を組み立てる。comment は渡された値のまま保存され（存在のみが
// validate される）、ユーザーのテキストを決して trim しない Rails に合わせて
// いる。
func NewReview(rating int, comment string, authorID string, burgerID string) (Review, error) {
	if err := ValidateReviewContent(rating, comment); err != nil {
		return Review{}, err
	}
	c := comment
	return Review{Rating: rating, Comment: &c, AuthorID: authorID, BurgerID: burgerID}, nil
}

// CanBeModifiedBy は review の所有権ルールの唯一の置き場である。author だけが
// review を編集または削除できる。意図的に admin の例外は設けない（issue #14
// AC3）。moderation の権限が対象とするのは shop であり、他のユーザーの
// review ではない。
func (r Review) CanBeModifiedBy(viewer User) bool {
	return r.AuthorID == viewer.ID
}

// CanBeModifiedByViewer は、匿名（nil）を含む viewer が review を編集・削除できるかを
// 返す。匿名は常に false で、それ以外は CanBeModifiedBy に従う。API が返す can_edit の
// 元になる値で、frontend はこの判断を再計算しない。
func (r Review) CanBeModifiedByViewer(viewer *User) bool {
	return viewer != nil && r.CanBeModifiedBy(*viewer)
}

// ReviewDetail は、author と、review 由来の統計を含む対象 burger を持つ
// review であり、review endpoint の payload である。統計は、まだ計算されて
// いない場合はゼロである。
type ReviewDetail struct {
	Review
	User   *UserRef
	Burger *ShopReviewBurger
	// PhotoURL は review の写真の公開 URL であり、添付されていないときは nil
	// である。PhotoKey から usecase が（photo storage を介して）導出する。
	// domain 自身は決して URL を組み立てない。
	PhotoURL *string
	// CanEdit は、viewer がこの review を編集・削除できるかである。viewer ごとに
	// 決まる値なので、usecase が CanBeModifiedByViewer で設定する（読み取ったままでは false）。
	CanEdit bool
}

// ---- repository の契約(実装は adapter/repository) ----

// ReviewRepository は review の書き込みの契約である。domain が宣言し、呼び出すのは
// domain のコード（書き込みオブジェクトの Reviews）だけで、usecase は呼ばない
// （読み取りは usecase の ReviewQuery）。実装は書き込みの SQL の詳細（カラム限定の書き込み、
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
	CreateReviewForNamedBurger(ctx context.Context, shopID string, burgerName string, review Review) (Review, ShopReviewBurger, error)
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

// ---- 書き込みオブジェクト(repository を呼ぶのは domain のコードだけ) ----

// Reviews は review 集約の書き込みオブジェクトである。ReviewRepository を持つのは
// この型だけで、usecase は repository に依存せず、review の書き込みをここに任せる。
// review だけを更新する書き込みは、Service ではなくこの型に置く（Service は複数の
// 集約を跨ぐ更新だけに使う。domain/doc.go を参照）。現時点では repository の
// 書き込みを 1 対 1 で包んでいる。burger_stats の再計算は repository の同一
// transaction の内部にあり、ここでは行わない。review に関する domain の手順が増えた
// ときは、usecase ではここへ置く。
type Reviews struct {
	repo ReviewRepository
}

// NewReviews は repo を使う Reviews を返す。
func NewReviews(repo ReviewRepository) *Reviews {
	return &Reviews{repo: repo}
}

// Create は、新しい（validate 済みの）review を永続化し、生成された id と
// created_at つきで返す。
func (s *Reviews) Create(ctx context.Context, review Review) (Review, error) {
	return s.repo.CreateReview(ctx, review)
}

// CreateForNamedBurger は、新しい（validate 済みの）review を、shop の burger の
// うち指定された名前と完全一致するものに対して永続化する（なければ burger を作る）。
// 返される burger は、insert 前に保存されていた stats を持つ。
func (s *Reviews) CreateForNamedBurger(ctx context.Context, shopID string, burgerName string, review Review) (Review, ShopReviewBurger, error) {
	return s.repo.CreateReviewForNamedBurger(ctx, shopID, burgerName, review)
}

// UpdateContent は、id の、まだ kept な review の rating と comment だけを永続化し、
// 保存された行を返す。
func (s *Reviews) UpdateContent(ctx context.Context, id int64, rating int, comment string) (Review, error) {
	return s.repo.UpdateReviewContent(ctx, id, rating, comment)
}

// UpdateContentAndPhotoKey は、id の、まだ kept な review の rating、comment、
// および photo_key を atomic に永続化し、保存された行を返す。
func (s *Reviews) UpdateContentAndPhotoKey(ctx context.Context, id int64, rating int, comment string, photoKey *string) (Review, error) {
	return s.repo.UpdateReviewContentAndPhotoKey(ctx, id, rating, comment, photoKey)
}

// Discard は review を soft delete する。
func (s *Reviews) Discard(ctx context.Context, id int64) error {
	return s.repo.DiscardReview(ctx, id)
}
