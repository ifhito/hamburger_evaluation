package domain

import (
	"context"
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

// CanBeReviewedByViewer は、匿名（nil）を含む viewer が shop の burger の review を
// 投稿できるかを返す。匿名は常に false で、それ以外は CanBeReviewedBy に従う。API が
// 返す can_review の元になる値で、frontend はこの判断を再計算しない。
func (s Shop) CanBeReviewedByViewer(viewer *User) bool {
	return viewer != nil && s.CanBeReviewedBy(*viewer)
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
	// CanReview は、viewer がこの shop に review を投稿できるかである。viewer ごとに
	// 決まる値なので、詳細を返す usecase が CanBeReviewedByViewer で設定する。
	CanReview bool
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

// ---- repository の契約(実装は adapter/repository) ----

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

// ---- 書き込みオブジェクト(repository を呼ぶのは domain のコードだけ) ----

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
