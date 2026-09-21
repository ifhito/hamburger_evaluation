package domain

import (
	"context"
	"fmt"
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
	PhotoKey  *string
	AuthorID  string
	BurgerID  int64
	CreatedAt time.Time
}

// rating の範囲（両端を含む）。ルールを持つのはこの domain だけで、frontend には
// GET /meta で返す（frontend は値を複製しない）。DB の CHECK（000005_create_reviews の
// rating BETWEEN 1 AND 5）と同じ値でなければならない。
const (
	MinRating = 1
	MaxRating = 5
)

// MaxCommentChars はレビューのコメントの文字数の上限（Unicode のコードポイント数）である。
// DB の CHECK 制約 reviews_comment_max_length（000005_create_reviews）と同じ値でなければならない。
// 食い違いは db/migrations_test.go が検出する。変えるときは、この定数と、該当する CREATE TABLE の CHECK の両方を直す
// （実運用に入ったあとは、新しいマイグレーションで直す）。
const MaxCommentChars = 2000

// ValidateReviewContent は、書き込み可能な review の属性に対して Rails の
// validation を強制する。rating は MinRating..MaxRating の整数でなければならず、comment は
// 存在しなければならない。失敗した場合は、Rails の full message そのままを
// *ValidationError に入れて返し、rating のメッセージが先に来る。
func ValidateReviewContent(rating int, comment string) error {
	var messages []string
	if rating < MinRating || rating > MaxRating {
		messages = append(messages, fmt.Sprintf("Rating must be in %d..%d", MinRating, MaxRating))
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
func NewReview(rating int, comment string, authorID string, burgerID int64) (Review, error) {
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
// （読み取りは usecase の ReviewQuery）。実装は書き込みの SQL の詳細（カラム限定の書き込み）を
// 自分の内部に留め、一致する行がないときは（wrap された）ErrReviewNotFound を返す。書き込み専用で、
// 読み取りのメソッドは置かない。burger_stats の再計算はここでは行わない（トランザクションを持つ
// usecase が、BurgerStatRepository と組み合わせて行う）。
type ReviewRepository interface {
	// CreateReview は、新しい（validate 済みの）review を永続化し、生成された
	// id と created_at つきで返す。
	CreateReview(ctx context.Context, review Review) (Review, error)
	// CreateShopBurger は、shop の burger のうち指定された名前と完全一致するものを返す。
	// shop にその名前の burger がなければ、burger とその shops_burgers のリンクを
	// 作成する（Rails の find_or_create_burger、S6 P3-1）。返される burger は、
	// 呼び出しの前に保存されていた stats を持つ。これは burger_id の経路で usecase の
	// ReviewQuery.GetShopBurger が返すものとまったく同じである。まったく新しい burger の
	// stats はゼロである。burger とリンクの作成は、呼び出し側のトランザクションに
	// 入る（review の insert が失敗しても、孤立した burger やリンクを残さないため、
	// 呼び出し側は同じトランザクションで CreateReview まで行う）。
	CreateShopBurger(ctx context.Context, shopID int64, burgerName string) (ShopReviewBurger, error)
	// UpdateReviewContent は、id の、まだ kept な review の rating と comment
	// だけを永続化し、保存された行を返す。存在しないか discard 済みのときは
	// （wrap された）ErrReviewNotFound を返す。カラム限定の書き込み
	// なので、discarded_at が書き込まれることは決してない。
	UpdateReviewContent(ctx context.Context, id int64, rating int, comment string) (Review, error)
	// UpdateReviewContentAndPhotoKey は、id の、まだ kept な review の
	// rating、comment、「および」photo_key を atomic に永続化する。カラム限定の
	// 2 つの書き込みは「1 つの」transaction を共有するので、写真つきの編集が
	// content だけを key なしで commit してしまうことは決してない（S10 の review fix）。
	// 保存された行を返すか、review が存在しないか discard 済みのときは
	// （wrap された）ErrReviewNotFound を返す（その場合は何も commit されない）。
	UpdateReviewContentAndPhotoKey(ctx context.Context, id int64, rating int, comment string, photoKey *string) (Review, error)
	// DiscardReview は review を soft delete する（discarded_at を記録し、
	// hard DELETE は決して行わない）。存在しないか、すでに discard 済みの
	// ときは（wrap された）ErrReviewNotFound を返す。
	DiscardReview(ctx context.Context, id int64) error
}

// ---- 書き込みオブジェクト(repository を呼ぶのは domain のコードだけ) ----

// Reviews は review 集約の書き込みオブジェクトである。ReviewRepository を持つのは
// この型だけで、usecase は repository に依存せず、review の書き込みをここに任せる。
// review だけを更新する書き込みは、Service ではなくこの型に置く（Service は複数の
// 集約を跨ぐ更新だけに使う。domain/doc.go を参照）。現時点では repository の
// 書き込みを 1 対 1 で包んでいる。burger_stats の再計算は、review の書き込みと、
// 読み取り（facts）を挟む手順なので、ここでは行わず、トランザクションを持つ usecase が
// UnitOfWork の中で組み立てる。review に関する domain の手順が増えたときは、usecase ではここへ置く。
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

// CreateShopBurger は、shop の burger のうち指定された名前と完全一致するものを返す
// （なければ burger とリンクを作る）。返される burger は、呼び出しの前に保存されていた
// stats を持つ。
func (s *Reviews) CreateShopBurger(ctx context.Context, shopID int64, burgerName string) (ShopReviewBurger, error) {
	return s.repo.CreateShopBurger(ctx, shopID, burgerName)
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
