package domain

import (
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
	AuthorID  int64
	BurgerID  int64
	CreatedAt time.Time
}

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
	}
	if len(messages) > 0 {
		return &ValidationError{Messages: messages}
	}
	return nil
}

// ValidateBurgerName は、burger_name による review 投稿の経路に対して Rails の
// Burger name の presence ルールを強制する。空またはホワイトスペースのみの
// 名前は拒否される。（Rails は空の名前に対して rescue されない RecordInvalid
// で応答するが、ここでは適切な validation failure とする。fail loud で 422
// を返す。）
func ValidateBurgerName(name string) error {
	if strings.TrimSpace(name) == "" {
		return &ValidationError{Messages: []string{"Burger name can't be blank"}}
	}
	return nil
}

// NewReview は、author が burger に対して投稿する validation 済みの新しい
// review を組み立てる。comment は渡された値のまま保存され（存在のみが
// validate される）、ユーザーのテキストを決して trim しない Rails に合わせて
// いる。
func NewReview(rating int, comment string, authorID, burgerID int64) (Review, error) {
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
}
