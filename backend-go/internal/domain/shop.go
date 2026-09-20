package domain

import (
	"strings"
	"time"
)

// ShopStatus は API 向けの shop status 文字列である。storage では smallint
// として符号化され、その対応付けは repository/domain の境界に置かれる。
type ShopStatus string

const (
	ShopStatusPending  ShopStatus = "pending"
	ShopStatusActive   ShopStatus = "active"
	ShopStatusRejected ShopStatus = "rejected"
)

// Shop は shop の domain 表現である。
type Shop struct {
	ID             int64
	Name           string
	Status         ShopStatus
	ModerationNote *string
	CreatorID      *int64
}

// ValidateShopName は、shop 名に対して Rails の presence validation を強制
// する。空またはホワイトスペースのみの名前は、Rails の full message そのまま
// を *ValidationError に入れて返す。
func ValidateShopName(name string) error {
	if strings.TrimSpace(name) == "" {
		return &ValidationError{Messages: []string{"Name can't be blank"}}
	}
	return nil
}

// NewShopSubmission は、ユーザーが投稿した shop を組み立てる。名前は validate
// され、status は pending で始まり（Rails ShopStatus.initial）、moderation
// note はまだなく、投稿したユーザーが creator として記録される。
func NewShopSubmission(name string, creatorID int64) (Shop, error) {
	if err := ValidateShopName(name); err != nil {
		return Shop{}, err
	}
	return Shop{Name: name, Status: ShopStatusPending, CreatorID: &creatorID}, nil
}

// Approve は active への moderation 遷移である。Rails ShopStatus と同様に、
// 現在のどの status からでも行える無条件の値遷移であり、rejected の shop の
// 再承認も許される。また、rejection を説明するためだけに存在する moderation
// note をクリアする。
func (s Shop) Approve() Shop {
	s.Status = ShopStatusActive
	s.ModerationNote = nil
	return s
}

// Reject は rejected への moderation 遷移であり、現在のどの status からでも
// 行える。省略可能な note は以前の note を置き換える（nil ならクリアされる）。
func (s Shop) Reject(note *string) Shop {
	s.Status = ShopStatusRejected
	s.ModerationNote = note
	return s
}

// CanBeReviewedBy は reviewable ルールの唯一の置き場であり、viewer（常に認証
// 済み。投稿にはログインが必要）がこの shop の burger の review を投稿して
// よいかどうかを決める。rejected の shop は決して reviewable ではない
// （creator や admin であっても同じで、彼らは ShopVisibility.CanView を通じて
// 閲覧はできる）。active な shop は認証済みの誰でも reviewable であり、
// pending な shop はその creator か admin のみが reviewable である。
func (s Shop) CanBeReviewedBy(viewer User) bool {
	switch s.Status {
	case ShopStatusActive:
		return true
	case ShopStatusPending:
		return viewer.Admin || (s.CreatorID != nil && *s.CreatorID == viewer.ID)
	default:
		return false
	}
}

// ShopVisibility は viewer から導出されるフィルタ記述子である。shop が見える
// のは、ViewAll が設定されている、shop が active である、creator が ViewerID
// である、のいずれかであるとき、かつそのときに限る。この型は shop の可視性
// ルールの唯一の置き場であり、CanView がそれを in-process で適用し、
// repository は記述子を SQL パラメータへ変換するだけである。
type ShopVisibility struct {
	// ViewAll は、status に関わらずすべての shop を見えるようにする（admin）。
	ViewAll bool
	// ViewerID は、nil でない場合、このユーザーが作成した shop も status に
	// 関わらず追加で見えるようにする。
	ViewerID *int64
}

// ShopVisibilityFor は viewer に対する可視性の記述子を導出する。nil は匿名
// （active な shop のみ）を意味する。
func ShopVisibilityFor(viewer *User) ShopVisibility {
	if viewer == nil {
		return ShopVisibility{}
	}
	if viewer.Admin {
		return ShopVisibility{ViewAll: true}
	}
	id := viewer.ID
	return ShopVisibility{ViewerID: &id}
}

// CanView は、この記述子の下で shop が見えるかどうかを返す。
func (v ShopVisibility) CanView(shop Shop) bool {
	if v.ViewAll || shop.Status == ShopStatusActive {
		return true
	}
	return v.ViewerID != nil && shop.CreatorID != nil && *shop.CreatorID == *v.ViewerID
}

// UserRef は、shop 詳細と review の payload に埋め込まれる {id, username} の
// projection である。
type UserRef struct {
	ID       int64
	Username string
}

// ShopDetail は、creator と discard されていない review を持つ shop である。
type ShopDetail struct {
	Shop
	Creator *UserRef // shop に creator がいない場合は nil
	Reviews []ShopReview
}

// ShopReview は shop 詳細に表示される review 1 件であり、その author と、
// review 由来の統計を含む対象 burger を持つ。
type ShopReview struct {
	ID        int64
	Rating    int
	Comment   *string
	CreatedAt time.Time
	User      *UserRef
	Burger    *ShopReviewBurger
}

// ShopReviewBurger は統計付きの対象 burger であり、統計はまだ計算されて
// いない場合はゼロである。
type ShopReviewBurger struct {
	ID            int64
	Name          string
	AverageRating float64
	ReviewCount   int64
	WeightedScore float64
	Confidence    float64
}
