package domain

import (
	"context"
	"strings"
	"time"
)

// Review は burger review の行を表す domain 上の表現である。
type Review struct {
	// ID は UUID の正規形（小文字・ハイフン区切り）である。DB が生成し、形式の判定は IsUUID が持つ。
	ID      string
	Rating  int
	Comment *string
	// PhotoKey は review の写真の storage key であり、写真が添付されて
	// いないときは nil である。key はここでは不透明な値であり、URL は
	// usecase が photo storage を介して組み立てる。domain 自身が組み立てる
	// ことは決してない。
	PhotoKey *string
	AuthorID string
	// BurgerID は burger の UUID の正規形である。
	BurgerID  string
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

// ValidateReviewContent は、書き込み可能な review の属性を検証する。rating は
// MinRating..MaxRating の整数でなければならず、comment は空でもよいが MaxCommentChars 文字を
// 超えてはならない。失敗した場合は、文言(キー + 引数)を *ValidationError に入れて返し、
// rating のメッセージが先に来る。
func ValidateReviewContent(rating int, comment string) error {
	var issues []Message
	if rating < MinRating || rating > MaxRating {
		issues = append(issues, Msg(keyReviewRatingRange, MinRating, MaxRating))
	}
	if exceedsChars(comment, MaxCommentChars) {
		issues = append(issues, Msg(keyCommentTooLong, MaxCommentChars))
	}
	if len(issues) > 0 {
		return NewValidationError(issues...)
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
		return NewValidationError(Msg(keyBurgerNameBlank))
	}
	if exceedsChars(name, MaxBurgerNameChars) {
		return NewValidationError(Msg(keyBurgerNameTooLong, MaxBurgerNameChars))
	}
	return nil
}

// NewReview は、author が burger に対して投稿する validation 済みの新しい
// review を組み立てる。comment は渡された値のまま保存され（空でもよく、
// trim もしない）、文字数の上限だけが validate される。
func NewReview(rating int, comment string, authorID string, burgerID string) (Review, error) {
	if err := ValidateReviewContent(rating, comment); err != nil {
		return Review{}, err
	}
	c := comment
	return Review{Rating: rating, Comment: &c, AuthorID: authorID, BurgerID: burgerID}, nil
}

// CanBeModifiedBy は review の所有権ルールの唯一の置き場である。author だけが
// review を編集または削除できる。意図的に admin の例外は設けない。
// moderation の権限が対象とするのは shop であり、他のユーザーの
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
	// Shop は、viewer にとっての、この review のショップである(ReviewShopFor が決める)。viewer に見える
	// ショップがないときは nil で、見えないショップの ID・名前は持たない。
	// Shop と CanReview は、**review の詳細(Reviews.Get)でだけ意味を持つ**。一覧・作成・更新では、
	// 設定されない(nil と false のままで、「できない」ではなく「求めていない」)。
	Shop *ShopRef
	// CanReview は、viewer が、Shop のショップに review を投稿できるかである(ショップ詳細の CanReview と
	// 同じ規則。Shop.CanBeReviewedByViewer)。viewer ごとに決まる値。
	CanReview bool
}

// ReviewShopFor は、review の詳細で、viewer にとっての「その review のショップ」と、viewer がそこに
// review を書けるかを決める。shops は、review の burger を持つショップすべてで、作成の古い順に並べたもの
// (burger は複数のショップにありうる)。
//
//   - viewer が review を書けるショップ(Shop.CanBeReviewedByViewer)があれば、その先頭と true を返す。
//   - なければ、viewer に見えるショップ(ShopVisibility.CanView)の先頭と false を返す。
//   - 見えるショップがなければ、nil と false を返す。
//
// viewer に見えないショップは、選ぶことにも、書けるかの判断にも使わない(そのショップがあることが、応答から
// 分からないようにする)。書けるショップは、必ず、見えるショップに含まれる。
func ReviewShopFor(shops []Shop, viewer *User) (*ShopRef, bool) {
	visibility := ShopVisibilityFor(viewer)
	var visible *Shop
	for i := range shops {
		shop := &shops[i]
		if shop.CanBeReviewedByViewer(viewer) {
			return &ShopRef{ID: shop.ID, Name: shop.Name}, true
		}
		if visible == nil && visibility.CanView(*shop) {
			visible = shop
		}
	}
	if visible == nil {
		return nil, false
	}
	return &ShopRef{ID: visible.ID, Name: visible.Name}, false
}

// ---- repository の契約(実装は adapter/repository) ----

// ReviewRepository は、レビューの書き込みの契約である。domain が宣言し、呼び出すのは domain のコード
// (書き込みオブジェクトの Reviews)だけで、usecase は直接呼ばない(読み取りは usecase の ReviewQuery)。
// 実装は、書き込みの SQL の詳細(更新する列を絞った書き込みなど)を自分の内部に留め、対象の行がない
// ときは、包んだ ErrReviewNotFound を返す。書き込み専用で、読み取りのメソッドは置かない。
//
// バーガーの統計の再計算は、ここでは行わない。レビューの書き込みと同じトランザクションで、
// トランザクションを持つ usecase が、BurgerStatRepository と組み合わせて行う。
type ReviewRepository interface {
	// CreateReview は、検証済みの新しいレビューを保存し、採番された ID と作成日時を持つレビューを返す。
	CreateReview(ctx context.Context, review Review) (Review, error)
	// CreateShopBurger は、ショップのバーガーのうち、名前が指定と完全に一致するものを返す。そのショップに
	// 同名のバーガーがなければ、バーガーを作成して、ショップと結び付ける(shops_burgers)。
	// 返すバーガーの統計は、この呼び出しの前の値である(まったく新しいバーガーなら 0)。バーガー ID を
	// 指定した投稿で、usecase の ReviewQuery.GetShopBurger が返す値と同じ意味になる。
	// バーガーと結び付けの作成は、呼び出し側のトランザクションに含まれる。レビューの登録に失敗したとき、
	// 作りかけのバーガーが残らないよう、呼び出し側は同じトランザクションでレビューの登録まで行う。
	CreateShopBurger(ctx context.Context, shopID string, burgerName string) (ShopReviewBurger, error)
	// UpdateReviewContent は、削除されていないレビューの評価とコメントだけを更新し、更新後のレビューを
	// 返す。レビューが存在しない、または論理削除済みなら、包んだ ErrReviewNotFound を返す。
	// 更新する列を評価とコメントに絞っているので、削除の目印(discarded_at)を書き換えることはない。
	UpdateReviewContent(ctx context.Context, id string, rating int, comment string) (Review, error)
	// UpdateReviewContentAndPhotoKey は、削除されていないレビューの評価・コメント・写真のキーを
	// まとめて更新し、更新後のレビューを返す。評価とコメントの更新と、写真のキーの更新は 1 つの
	// トランザクションで行うので、写真のキーが付かないままコメントだけが確定することはない。
	// レビューが存在しない、または論理削除済みなら、包んだ ErrReviewNotFound を返す(何も確定しない)。
	UpdateReviewContentAndPhotoKey(ctx context.Context, id string, rating int, comment string, photoKey *string) (Review, error)
	// DiscardReview はレビューを論理削除する(削除日時を記録するだけで、行は消さない)。レビューが
	// 存在しない、またはすでに論理削除済みなら、包んだ ErrReviewNotFound を返す。
	DiscardReview(ctx context.Context, id string) error
}

// ---- 書き込みオブジェクト(repository を呼ぶのは domain のコードだけ) ----

// Reviews は review 集約の書き込みオブジェクトである。ReviewRepository を持つのは
// この型だけで、usecase は repository に依存せず、review の書き込みをここに任せる。
// review だけを更新する書き込みは、Service ではなくこの型に置く（Service は複数の
// 集約を跨ぐ更新だけに使う。domain/doc.go を参照）。現時点では repository の
// 書き込みを 1 対 1 で包んでいる。バーガーの統計の再計算は、レビューの書き込みと、統計の元データの
// 読み取りが交互に出てくる手順なので、ここでは行わない。トランザクションを持つ usecase が、
// 書き込みと読み取りをまとめて 1 つのトランザクションにする仕組み(UnitOfWork。途中でエラーに
// なれば全体を取り消す)の中で組み立てる。
// レビューに関する業務の手順が増えたときは、usecase ではなく、ここへ置く。
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

// CreateShopBurger は、ショップのバーガーのうち、名前が指定とちょうど一致するものを返す(なければ、
// バーガーを作ってショップと結び付ける)。返すバーガーの統計は、この呼び出しの前の値である。
func (s *Reviews) CreateShopBurger(ctx context.Context, shopID string, burgerName string) (ShopReviewBurger, error) {
	return s.repo.CreateShopBurger(ctx, shopID, burgerName)
}

// UpdateContent は、id の、まだ kept な review の rating と comment だけを永続化し、
// 保存された行を返す。
func (s *Reviews) UpdateContent(ctx context.Context, id string, rating int, comment string) (Review, error) {
	return s.repo.UpdateReviewContent(ctx, id, rating, comment)
}

// UpdateContentAndPhotoKey は、id の、まだ kept な review の rating、comment、
// および photo_key を atomic に永続化し、保存された行を返す。
func (s *Reviews) UpdateContentAndPhotoKey(ctx context.Context, id string, rating int, comment string, photoKey *string) (Review, error) {
	return s.repo.UpdateReviewContentAndPhotoKey(ctx, id, rating, comment, photoKey)
}

// Discard は review を soft delete する。
func (s *Reviews) Discard(ctx context.Context, id string) error {
	return s.repo.DiscardReview(ctx, id)
}
