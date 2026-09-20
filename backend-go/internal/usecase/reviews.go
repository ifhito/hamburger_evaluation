package usecase

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/photo"
)

// ReviewRepository は review 向けの consumer 側の永続化の契約である。
// 実装は SQL の詳細（EXISTS による active な shop のフィルタ、smallint の
// status のエンコード、soft delete の述語）を自分の内部に留め、一致する行が
// ないときは wrap した domain の sentinel（ErrReviewNotFound、
// ErrShopNotFound、ErrBurgerNotFound）を返す。
type ReviewRepository interface {
	// ListReviews は、burger が少なくとも 1 つの active な shop で提供されて
	// いる、discard されていない review を返す。filter で絞り込み、author、
	// burger、stats を結合し（N+1 なし）、新しい順（created_at desc、
	// id desc）に並べる。
	ListReviews(ctx context.Context, filter ReviewListFilter, limit, offset int32) ([]domain.ReviewDetail, error)
	// GetReview は、author、burger、stats つきの、discard されていない
	// review を 1 件返すか、（wrap された）domain.ErrReviewNotFound を返す。
	// 存在しない review と discard 済みの review は区別できない。
	GetReview(ctx context.Context, id int64) (domain.ReviewDetail, error)
	// GetShop は素の shop の行（creator なし、review なし）を返すか、
	// （wrap された）domain.ErrShopNotFound を返す。
	GetShop(ctx context.Context, id int64) (domain.Shop, error)
	// GetShopBurger は、burger が shops_burgers 経由でその shop に紐づいて
	// いるときに限り、stats つき（まだ計算されていなければゼロ）の burger を
	// 返し、そうでなければ（wrap された）domain.ErrBurgerNotFound を返す。
	GetShopBurger(ctx context.Context, shopID, burgerID int64) (domain.ShopReviewBurger, error)
	// CreateReview は、新しい（validate 済みの）review を永続化し、生成された
	// id と created_at つきで返す。この書き込みは、同一 transaction 内で
	// burger の burger_stats も再計算する（issue #15、S7）。
	CreateReview(ctx context.Context, review domain.Review) (domain.Review, error)
	// CreateReviewForNamedBurger は、新しい（validate 済みの）review を、
	// shop の burger のうち指定された名前と完全一致するものに対して永続化する。
	// shop にその名前の burger がなければ、burger とその shops_burgers の
	// リンクを作成する（Rails の find_or_create_burger、S6 P3-1）。review の
	// BurgerID の入力は無視され、解決された burger に設定される。
	// find-or-create、review の insert、burger_stats の再計算は「1 つの」
	// transaction 内で行われるので、insert が失敗しても孤立した burger や
	// リンクは残らない。返される burger は、insert 前に保存されていた stats を
	// 持つ。これは burger_id の経路で GetShopBurger が返すものとまったく同じで
	// ある。まったく新しい burger の stats はゼロである。
	CreateReviewForNamedBurger(ctx context.Context, shopID int64, burgerName string, review domain.Review) (domain.Review, domain.ShopReviewBurger, error)
	// UpdateReviewContent は、id の、まだ kept な review の rating と comment
	// だけを永続化し、保存された行を返す。存在しないか discard 済みのときは
	// （wrap された）domain.ErrReviewNotFound を返す。カラム限定の書き込み
	// なので、discarded_at が書き込まれることは決してない。この書き込みは、
	// 同一 transaction 内で burger の burger_stats も再計算する。
	UpdateReviewContent(ctx context.Context, id int64, rating int, comment string) (domain.Review, error)
	// UpdateReviewContentAndPhotoKey は、id の、まだ kept な review の
	// rating、comment、「および」photo_key を atomic に永続化する。カラム限定の
	// 2 つの書き込みと burger_stats の再計算が「1 つの」transaction を共有する
	// ので、写真つきの編集が content だけを key なしで commit してしまうことは
	// 決してない（S10 の review fix）。保存された行を返すか、review が存在しない
	// か discard 済みのときは（wrap された）domain.ErrReviewNotFound を返す
	// （その場合は何も commit されない）。
	UpdateReviewContentAndPhotoKey(ctx context.Context, id int64, rating int, comment string, photoKey *string) (domain.Review, error)
	// DiscardReview は review を soft delete する（discarded_at を記録し、
	// hard DELETE は決して行わない）。存在しないか、すでに discard 済みの
	// ときは（wrap された）domain.ErrReviewNotFound を返す。この書き込みは、
	// 同一 transaction 内で burger の burger_stats も再計算する。
	DiscardReview(ctx context.Context, id int64) error
}

// ReviewListFilter は、GET /reviews の省略可能なクエリフィルタを保持する。
// Rails の ReviewQuery を再現しており、指定されたフィルタはそれぞれフィードを
// 絞り込み、指定されたフィルタはすべて AND で組み合わされる。nil の
// Rating/ShopID と空の Keyword は「absent」を意味する（Rails の
// params[:x].present?）。したがって、0 や負の id/rating が指定された場合も、
// Rails とまったく同様にフィルタとして働く（結果は空のページになる）。
type ReviewListFilter struct {
	// Rating は rating の完全一致フィルタである（Rails の by_rating）。
	Rating *int
	// Keyword は、comment に対するリテラルで大文字小文字を区別しない部分文字列
	// 一致である（Rails の keyword_search、comment ILIKE %escaped%）。
	Keyword string
	// ShopID は、shops_burgers 経由でその shop に burger が紐づいている review
	// だけを残す（Rails の shops_and_burgers の join）。
	ShopID *int64
}

// Reviews は review の use case を実装する。公開フィードと詳細、および
// author に限定された create/edit/delete であり、review ごとに任意で 1 枚の
// 写真を photos 経由で保存する（S10）。
type Reviews struct {
	repo   ReviewRepository
	photos PhotoStorage
}

// NewReviews は review の use case を配線する。photos は non-nil でなければ
// ならない（本番では disk か S3、テストでは fake）。どのリクエスト経路も
// それを dereference しうる（photoURL、deletePhotoBestEffort）ので、nil の
// storage は、リクエストの途中で panic するのではなく、ここで fail-loud する。
func NewReviews(repo ReviewRepository, photos PhotoStorage) *Reviews {
	if photos == nil {
		panic("usecase.NewReviews: nil PhotoStorage")
	}
	return &Reviews{repo: repo, photos: photos}
}

// List は、filter で絞り込んだ公開 review フィードを返す。ページネーションは
// clampPage に従い、Shops.List と同じフォールバック規則で行う。
func (s *Reviews) List(ctx context.Context, filter ReviewListFilter, page, perPage int) ([]domain.ReviewDetail, error) {
	limit, offset := clampPage(page, perPage)
	reviews, err := s.repo.ListReviews(ctx, filter, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list reviews: %w", err)
	}
	for i := range reviews {
		reviews[i].PhotoURL = s.photoURL(reviews[i].PhotoKey)
	}
	return reviews, nil
}

// Get は author、burger、stats つきの review を 1 件返す。存在しない review と
// discard 済みの review は、どちらも domain.ErrReviewNotFound を返す。
func (s *Reviews) Get(ctx context.Context, id int64) (domain.ReviewDetail, error) {
	detail, err := s.repo.GetReview(ctx, id)
	if err != nil {
		return domain.ReviewDetail{}, fmt.Errorf("get review: %w", err)
	}
	detail.PhotoURL = s.photoURL(detail.PhotoKey)
	return detail, nil
}

// Create は、viewer による、shop の burger に対する review を投稿する。
// チェックの順序は契約どおりで、未知の shop（404）、次に domain の
// reviewable ルール（403）、次に burger の解決、次に content の validation
// （422）である。正の burgerID が優先され、shop に紐づく burger を指して
// いなければならない（そうでなければ 404）。そうでない場合は、空白でない
// burgerName が、insert と同じ transaction 内で、完全一致かつ trim されて
// いない名前で shop の burger を find-or-create する（Rails parity、
// S6 P3-1）。どちらでもない場合は validation の失敗（422）であり、黙って
// デフォルトを使うことは決してない。レスポンスの detail は、viewer と、
// 存在確認のために解決した burger から組み立てる。再取得はしない。nil でない
// upload（handler で validate 済み/正規化済み、S10）は、insert の前に新しい
// ランダムな key で保存される。その後 insert が失敗した場合は、アップロード
// したばかりの blob を best-effort で削除するので、リクエストより長く残る
// 孤立ファイルはない。
func (s *Reviews) Create(ctx context.Context, viewer domain.User, shopID, burgerID int64, burgerName string, rating int, comment string, upload *photo.Processed) (domain.ReviewDetail, error) {
	shop, err := s.repo.GetShop(ctx, shopID)
	if err != nil {
		return domain.ReviewDetail{}, fmt.Errorf("create review: %w", err)
	}
	if !shop.CanBeReviewedBy(viewer) {
		return domain.ReviewDetail{}, domain.ErrForbidden
	}
	var burger domain.ShopReviewBurger
	if burgerID > 0 {
		if burger, err = s.repo.GetShopBurger(ctx, shopID, burgerID); err != nil {
			return domain.ReviewDetail{}, fmt.Errorf("create review: %w", err)
		}
	} else if err := domain.ValidateBurgerName(burgerName); err != nil {
		return domain.ReviewDetail{}, err
	}
	// BurgerID 0：repository が transaction 内でこれを解決する。
	review, err := domain.NewReview(rating, comment, viewer.ID, burgerID)
	if err != nil {
		return domain.ReviewDetail{}, err
	}
	if review.PhotoKey, err = s.putPhoto(ctx, upload); err != nil {
		return domain.ReviewDetail{}, fmt.Errorf("create review: %w", err)
	}
	var created domain.Review
	if burgerID > 0 {
		created, err = s.repo.CreateReview(ctx, review)
	} else {
		created, burger, err = s.repo.CreateReviewForNamedBurger(ctx, shopID, burgerName, review)
	}
	if err != nil {
		s.deletePhotoBestEffort(ctx, review.PhotoKey)
		return domain.ReviewDetail{}, fmt.Errorf("create review: %w", err)
	}
	return domain.ReviewDetail{
		Review:   created,
		User:     &domain.UserRef{ID: viewer.ID, Username: viewer.Username},
		Burger:   &burger,
		PhotoURL: s.photoURL(created.PhotoKey),
	}, nil
}

// Update は review の rating と comment を編集する。load（存在しない review
// と discard 済みの review はどちらも 404）、domain の所有権ルール（403。
// issue #14 AC3、admin でも通らない）、content の validation（422）、
// そしてカラム限定の書き込みの順で行う。保存された行は load した detail に
// マージされるので、レスポンスは再取得なしで author、burger、stats を持つ。
// nil でない upload（S10）は写真を置き換える。新しい blob を先に保存し、
// 続いて content と photo_key を「1 つの」repository の transaction で切り
// 替え（失敗しても content だけが key なしで commit されることは決して
// ない）、その DB の成功の後にはじめて古い blob を best-effort で削除する。
// nil の upload は content だけの書き込みを行い、photo_key には触れない
// （写真を削除する経路はない）。
func (s *Reviews) Update(ctx context.Context, viewer domain.User, id int64, rating int, comment string, upload *photo.Processed) (domain.ReviewDetail, error) {
	detail, err := s.repo.GetReview(ctx, id)
	if err != nil {
		return domain.ReviewDetail{}, fmt.Errorf("update review: %w", err)
	}
	if !detail.CanBeModifiedBy(viewer) {
		return domain.ReviewDetail{}, domain.ErrForbidden
	}
	if err := domain.ValidateReviewContent(rating, comment); err != nil {
		return domain.ReviewDetail{}, err
	}
	newKey, err := s.putPhoto(ctx, upload)
	if err != nil {
		return domain.ReviewDetail{}, fmt.Errorf("update review: %w", err)
	}
	var updated domain.Review
	if newKey != nil {
		if updated, err = s.repo.UpdateReviewContentAndPhotoKey(ctx, id, rating, comment, newKey); err != nil {
			s.deletePhotoBestEffort(ctx, newKey)
			return domain.ReviewDetail{}, fmt.Errorf("update review: %w", err)
		}
		// 古い blob が参照されなくなるのは、DB が新しい key を指すように
		// なった今になってからである。それを失っても、漏れたファイルに
		// なるだけで、review が壊れることはない。
		s.deletePhotoBestEffort(ctx, detail.PhotoKey)
	} else if updated, err = s.repo.UpdateReviewContent(ctx, id, rating, comment); err != nil {
		return domain.ReviewDetail{}, fmt.Errorf("update review: %w", err)
	}
	detail.Review = updated
	detail.PhotoURL = s.photoURL(updated.PhotoKey)
	return detail, nil
}

// Delete は review を soft delete する。load（404）、domain の所有権ルール
// （403、Update と同様に author のみ）、そしてカラム限定の discard の順で
// 行い、hard DELETE は決して行わない。写真の blob があれば、discard が
// 成功した後に best-effort で削除される（S10）。
func (s *Reviews) Delete(ctx context.Context, viewer domain.User, id int64) error {
	detail, err := s.repo.GetReview(ctx, id)
	if err != nil {
		return fmt.Errorf("delete review: %w", err)
	}
	if !detail.CanBeModifiedBy(viewer) {
		return domain.ErrForbidden
	}
	if err := s.repo.DiscardReview(ctx, id); err != nil {
		return fmt.Errorf("delete review: %w", err)
	}
	s.deletePhotoBestEffort(ctx, detail.PhotoKey)
	return nil
}

// putPhoto は、処理済みの upload を新しいランダムな key
// （"reviews/<32 hex chars><ext>"、crypto/rand。衝突は無視できるほど小さい）
// で保存し、その key を返す。nil の upload は nil の key を返し、storage の
// 呼び出しは行わない。
func (s *Reviews) putPhoto(ctx context.Context, upload *photo.Processed) (*string, error) {
	if upload == nil {
		return nil, nil
	}
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return nil, fmt.Errorf("put photo: random key: %w", err)
	}
	key := "reviews/" + hex.EncodeToString(raw[:]) + upload.Ext
	if err := s.photos.Put(ctx, key, upload.ContentType, bytes.NewReader(upload.Data)); err != nil {
		return nil, fmt.Errorf("put photo %q: %w", key, err)
	}
	return &key, nil
}

// deletePhotoBestEffort は、key の下の blob があれば削除する。これは
// fail-loud に対する、文書化された best-effort の例外である。実行される
// 時点で DB はすでに source of truth になっているので、ここでの storage の
// 失敗が意味するのは、漏れた（またはすでに消えている）blob であり、
// review が壊れることは決してない。key つきでログに記録され、リクエストは
// 成功のままである。delete は、リクエストのキャンセルから切り離され
// （WithoutCancel）、それ専用の短い timeout の下で実行される。クライアントが
// 接続を切っても、すべての delete が確実に孤立ファイルになってしまっては
// ならない（S3 モード）。一方で timeout が呼び出しの長さを有限に保つ。
func (s *Reviews) deletePhotoBestEffort(ctx context.Context, key *string) {
	if key == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := s.photos.Delete(ctx, *key); err != nil {
		slog.Warn("best-effort review photo delete failed", "key", *key, "error", err)
	}
}

// photoURL は、保存された写真の key をその公開 URL に対応させる（nil を渡せば
// nil が返る）。
func (s *Reviews) photoURL(key *string) *string {
	if key == nil {
		return nil
	}
	url := s.photos.URL(*key)
	return &url
}
