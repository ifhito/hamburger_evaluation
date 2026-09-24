package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// ShopSort は GET /shops の並び順を選ぶクエリ整形用の値である。ゼロ値(空文字)は既定の
// 店名順で、ShopSortNewest だけが新着順を表す。handler が query 文字列から作る。
// これは業務の規則ではなく(検証も権限判断もない)、並び方を選ぶだけの値なので domain には
// 置かない(usecase.ReviewListFilter と同じ位置づけ)。
type ShopSort string

// ShopSortNewest を指定すると、created_at 降順・id 降順(新着順)になる。それ以外の値
// (ゼロ値を含む)は、すべて既定の店名順として扱う。
const ShopSortNewest ShopSort = "newest"

// ShopQuery は shops 向けの consumer 側の読み取りの契約である。
// 実装は visibility の記述子を SQL のパラメータに変換し（ルール自体は
// domain.ShopVisibility にある）、smallint の status のエンコードを自分の
// 内部に留め、id に一致する shop がないときは（wrap された）
// domain.ErrShopNotFound を返す。読み取り専用で、書き込みのメソッドは
// 置かない（書き込みは domain.Shops を通す）。
type ShopQuery interface {
	// ListShops は、keyword に一致する見える shop を、集計(件数・評価の平均・ショップの
	// 写真のキー)つきで返す（keyword は name のリテラルな部分文字列で、大文字小文字を区別しない。空ならすべてに
	// 一致する）。並び順は sort で選ぶ：ShopSortNewest なら created_at 降順・id 降順（新着順）、それ以外
	// （ゼロ値を含む）は name 昇順・id 昇順（既定の店名順）。集計は、保存された値(shop_stats)を、同じクエリで添える(shop の件数に比例してクエリを増やさない。
	// まだ集計されていない shop は、空の集計(件数 0・平均と写真は nil))。集計の意味は domain.CalculateShopStat が
	// 定義し、レビューの書き込みのあとに、バックグラウンドのワーカーが計算し直す(結果整合)。
	// 2 つ目の戻り値は、offset+limit 件より後ろにも見える shop があるか（has_more）で、実装は limit+1 件を
	// 取得して判定する。
	ListShops(ctx context.Context, vis domain.ShopVisibility, keyword string, sort ShopSort, limit, offset int32) ([]domain.ShopListing, bool, error)
	// GetShopWithCreator は shop とその creator を、集計(保存された値。ListShops と同じ)つきで返す。
	// Reviews は空のままである。
	GetShopWithCreator(ctx context.Context, id string) (domain.ShopDetail, error)
	// ListShopReviews は、shop の burger に対する discard されていない review
	// （author が discard 済みの user である review は除く）を、新しい順に
	// 返す（created_at desc、id desc）。
	ListShopReviews(ctx context.Context, shopID string) ([]domain.ShopReview, error)
	// ListShopsForModeration は、creator つきのすべての shop（Reviews は
	// 空のまま）を新しい順（created_at desc、id desc）に返す。任意で 1 つの
	// status に絞り込める（nil = すべて）。limit+1 件を取得して次ページの有無も返す。
	ListShopsForModeration(ctx context.Context, status *domain.ShopStatus, limit, offset int32) ([]domain.ShopDetail, bool, error)
}

// Shops は shop の use case を実装する。公開の一覧と詳細、ユーザーによる
// 投稿、そして admin による moderation である。読み取りは query、書き込みは
// domain の書き込みオブジェクト（domain.Shops）だけを通し、repository には依存しない。
type Shops struct {
	query  ShopQuery
	shops  *domain.Shops
	photos PhotoURLs
}

// NewShops は shop の use case を配線する。photos(ショップの写真のキーを公開 URL に直す写真の保存先)は
// non-nil でなければならない(本番では disk か S3、テストでは fake)。渡し忘れて、写真の URL が黙って
// null になることのないよう、nil の photos は、ここで fail-loud する(NewReviews と同じ)。
func NewShops(query ShopQuery, shops *domain.Shops, photos PhotoURLs) *Shops {
	if photos == nil {
		panic("usecase.NewShops: nil PhotoURLs")
	}
	return &Shops{query: query, shops: shops, photos: photos}
}

// withPhotoURL は、集計の写真のキーを公開 URL に直す(写真がないときは nil)。
func (s *Shops) withPhotoURL(summary domain.ShopSummary) domain.ShopSummary {
	if summary.PhotoKey != nil {
		url := s.photos.URL(*summary.PhotoKey)
		summary.PhotoURL = &url
	}
	return summary
}

// List は、viewer（nil = 匿名）から見える shop のうち keyword に一致する
// ものを、sort の並び順で（ShopSortNewest = 新着順、それ以外 = 既定の店名順）ページネーションして
// 返す。範囲外の page/perPage は、エラーにせず clampPage の規則で補正される（page < 1 は 1、
// perPage < 1 は 20、perPage の上限は 100）。2 つ目の戻り値は、次のページがあるか（has_more）である。
func (s *Shops) List(ctx context.Context, viewer *domain.User, keyword string, sort ShopSort, page, perPage int) ([]domain.ShopListing, bool, error) {
	limit, offset := clampPage(page, perPage)
	listings, hasMore, err := s.query.ListShops(ctx, domain.ShopVisibilityFor(viewer), keyword, sort, limit, offset)
	if err != nil {
		return nil, false, fmt.Errorf("list shops: %w", err)
	}
	for i := range listings {
		listings[i].Summary = s.withPhotoURL(listings[i].Summary)
	}
	return listings, hasMore, nil
}

// Get は、viewer が見てよいときに shop の詳細（creator と review を含む）を
// 返す。存在しない shop と隠された shop は、どちらも domain.ErrShopNotFound を
// 返すので、存在の有無は漏れない。CanReview には、viewer（nil = 匿名）が
// この shop に review を投稿できるか（domain の reviewable ルール）を設定する。
func (s *Shops) Get(ctx context.Context, viewer *domain.User, id string) (domain.ShopDetail, error) {
	detail, err := s.query.GetShopWithCreator(ctx, id)
	if err != nil {
		return domain.ShopDetail{}, fmt.Errorf("get shop: %w", err)
	}
	if !domain.ShopVisibilityFor(viewer).CanView(detail.Shop) {
		return domain.ShopDetail{}, domain.ErrShopNotFound
	}
	reviews, err := s.query.ListShopReviews(ctx, id)
	if err != nil {
		return domain.ShopDetail{}, fmt.Errorf("list shop reviews: %w", err)
	}
	for i := range reviews {
		if reviews[i].PhotoKey != nil {
			url := s.photos.URL(*reviews[i].PhotoKey)
			reviews[i].PhotoURL = &url
		}
	}
	detail.Reviews = reviews
	detail.CanReview = detail.CanBeReviewedByViewer(viewer)
	detail.Summary = s.withPhotoURL(detail.Summary)
	return detail, nil
}

// Create は viewer に代わって新しい shop を投稿する。shop は pending で
// 始まり（後で moderator が activate する）、viewer が creator として
// 記録される。空白の name は、domain の *ValidationError をそのまま返す。mapURL は
// 任意の地図リンクで、domain.ValidateMapURL の規則に従う。
func (s *Shops) Create(ctx context.Context, viewer domain.User, name string, mapURL string) (domain.ShopDetail, error) {
	shop, err := domain.NewShopSubmission(name, viewer.ID, mapURL)
	if err != nil {
		return domain.ShopDetail{}, err
	}
	created, err := s.shops.Create(ctx, shop)
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

// AdminList は moderation 画面向けに shop を新しい順でページングして返し、
// API の status 文字列で任意に絞り込む。未知の status はエラーにならず、
// 何にも一致しない（Rails の where(status: unknown) と同様）。
// admin でない viewer には domain.ErrForbidden を返す。
func (s *Shops) AdminList(ctx context.Context, viewer domain.User, status string, page, perPage int) ([]domain.ShopDetail, bool, error) {
	if !viewer.CanModerate() {
		return nil, false, domain.ErrForbidden
	}
	var filter *domain.ShopStatus
	if status != "" {
		switch st := domain.ShopStatus(status); st {
		case domain.ShopStatusPending, domain.ShopStatusActive, domain.ShopStatusRejected:
			filter = &st
		default:
			return []domain.ShopDetail{}, false, nil
		}
	}
	limit, offset := clampPage(page, perPage)
	shops, hasMore, err := s.query.ListShopsForModeration(ctx, filter, limit, offset)
	if err != nil {
		return nil, false, fmt.Errorf("admin list shops: %w", err)
	}
	return shops, hasMore, nil
}

// AdminUpdateName は shop の名前(と地図リンク)を変更する（唯一の moderation 編集、Rails
// parity）。admin でない viewer には、どの id が存在するかを探れないよう、
// lookup の前に domain.ErrForbidden を返す。空白の name は
// *ValidationError であり、mapURL は domain.ValidateMapURL の規則に従う。
func (s *Shops) AdminUpdateName(ctx context.Context, viewer domain.User, id string, name string, mapURL string) (domain.ShopDetail, error) {
	if !viewer.CanModerate() {
		return domain.ShopDetail{}, domain.ErrForbidden
	}
	if err := domain.ValidateShopName(name); err != nil {
		return domain.ShopDetail{}, err
	}
	normalizedMapURL, err := domain.ValidateMapURL(mapURL)
	if err != nil {
		return domain.ShopDetail{}, err
	}
	// fetch はレスポンス用の creator（と、未知の id に対する 404）を
	// 供給する。書き込み自体は name と map_url のカラムにしか触れないので、並行する
	// status の変更を元に戻すことはない。
	detail, err := s.query.GetShopWithCreator(ctx, id)
	if err != nil {
		return domain.ShopDetail{}, fmt.Errorf("admin update shop name: %w", err)
	}
	updated, err := s.shops.UpdateName(ctx, id, name, normalizedMapURL)
	if err != nil {
		return domain.ShopDetail{}, fmt.Errorf("admin update shop name: %w", err)
	}
	detail.Shop = updated
	return detail, nil
}

// Approve は shop を activate し、moderation note を消去して（domain の
// 遷移）、公開して見えるようにする。admin でない viewer には、lookup の前に
// domain.ErrForbidden を返す。
func (s *Shops) Approve(ctx context.Context, viewer domain.User, id string) (domain.ShopDetail, error) {
	if !viewer.CanModerate() {
		return domain.ShopDetail{}, domain.ErrForbidden
	}
	return s.moderate(ctx, id, domain.Shop.Approve)
}

// Reject は、任意の moderation note つきで shop を reject し、公開の一覧から
// 隠す。admin でない viewer には、lookup の前に domain.ErrForbidden を返す。
// note が上限を超えるときは、lookup の前に *domain.ValidationError（422）を返す。
func (s *Shops) Reject(ctx context.Context, viewer domain.User, id string, note *string) (domain.ShopDetail, error) {
	if !viewer.CanModerate() {
		return domain.ShopDetail{}, domain.ErrForbidden
	}
	if err := domain.ValidateModerationNote(note); err != nil {
		return domain.ShopDetail{}, err
	}
	return s.moderate(ctx, id, func(shop domain.Shop) domain.Shop {
		return shop.Reject(note)
	})
}

// Close は active でまだ閉業していない shop を閉業にする（closed_at を今にする）。閉業した
// shop は、status に関わらず review を受け付けなくなる（domain.Shop.CanBeReviewedBy）。
// admin でない viewer には、lookup の前に domain.ErrForbidden を返す。閉業できない遷移
// （pending・rejected、またはすでに閉業した shop）は *domain.ValidationError（422）を返す。
func (s *Shops) Close(ctx context.Context, viewer domain.User, id string) (domain.ShopDetail, error) {
	if !viewer.CanModerate() {
		return domain.ShopDetail{}, domain.ErrForbidden
	}
	detail, err := s.query.GetShopWithCreator(ctx, id)
	if err != nil {
		return domain.ShopDetail{}, fmt.Errorf("close shop: %w", err)
	}
	if !detail.Shop.CanBeClosed() {
		return domain.ShopDetail{}, domain.NewValidationError(domain.MsgShopCannotClose)
	}
	closed := detail.Shop.Close(time.Now().UTC())
	updated, err := s.shops.UpdateClosedAt(ctx, id, closed.ClosedAt)
	if err != nil {
		return domain.ShopDetail{}, fmt.Errorf("close shop: %w", err)
	}
	detail.Shop = updated
	return detail, nil
}

// Reopen は閉業した shop の閉業を解く（closed_at を null に戻す）。admin でない viewer には、
// lookup の前に domain.ErrForbidden を返す。閉業していない shop への再開は
// *domain.ValidationError（422）を返す。
func (s *Shops) Reopen(ctx context.Context, viewer domain.User, id string) (domain.ShopDetail, error) {
	if !viewer.CanModerate() {
		return domain.ShopDetail{}, domain.ErrForbidden
	}
	detail, err := s.query.GetShopWithCreator(ctx, id)
	if err != nil {
		return domain.ShopDetail{}, fmt.Errorf("reopen shop: %w", err)
	}
	if !detail.Shop.CanBeReopened() {
		return domain.ShopDetail{}, domain.NewValidationError(domain.MsgShopCannotReopen)
	}
	reopened := detail.Shop.Reopen()
	updated, err := s.shops.UpdateClosedAt(ctx, id, reopened.ClosedAt)
	if err != nil {
		return domain.ShopDetail{}, fmt.Errorf("reopen shop: %w", err)
	}
	detail.Shop = updated
	return detail, nil
}

// moderate は status 遷移に共通のフローである。shop を creator とともに
// load し（レスポンスと 404 のため）、domain の遷移を適用し、その status と
// moderation note だけを永続化し、保存された行を持つ detail を返す。
// カラム限定の書き込みなので、古いスナップショットから並行する rename を
// 元に戻すことはない。
func (s *Shops) moderate(ctx context.Context, id string, transition func(domain.Shop) domain.Shop) (domain.ShopDetail, error) {
	detail, err := s.query.GetShopWithCreator(ctx, id)
	if err != nil {
		return domain.ShopDetail{}, fmt.Errorf("moderate shop: %w", err)
	}
	next := transition(detail.Shop)
	updated, err := s.shops.UpdateStatus(ctx, id, next.Status, next.ModerationNote)
	if err != nil {
		return domain.ShopDetail{}, fmt.Errorf("moderate shop: %w", err)
	}
	detail.Shop = updated
	return detail, nil
}
