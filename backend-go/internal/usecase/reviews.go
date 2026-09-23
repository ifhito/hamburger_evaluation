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

// ReviewQuery は review 向けの consumer 側の読み取りの契約である。
// 実装は SQL の詳細（EXISTS による active な shop のフィルタ、smallint の
// status のエンコード、soft delete の述語）を自分の内部に留め、一致する行が
// ないときは wrap した domain の sentinel（ErrReviewNotFound、
// ErrShopNotFound、ErrBurgerNotFound）を返す。読み取り専用で、書き込みの
// メソッドは置かない（書き込みは domain.Reviews を通す）。
type ReviewQuery interface {
	// ListReviews は、burger が少なくとも 1 つの active な shop で提供されて
	// いる、discard されていない review（author が discard 済みの user である
	// review も除く）を返す。filter で絞り込み、author、burger、stats を
	// 結合し（N+1 なし）、新しい順（created_at desc、id desc）に並べる。
	// 2 つ目の戻り値は、offset+limit 件より後ろにも一致する review があるか（has_more）で、
	// 実装は limit+1 件を取得して判定する。
	ListReviews(ctx context.Context, filter ReviewListFilter, limit, offset int32) ([]domain.ReviewDetail, bool, error)
	// GetReview は、author、burger、stats つきの、discard されていない
	// review を 1 件返すか、（wrap された）domain.ErrReviewNotFound を返す。
	// 存在しない review、discard 済みの review、author が discard 済みの
	// user である review は区別できない。
	GetReview(ctx context.Context, id string) (domain.ReviewDetail, error)
	// GetShop は素の shop の行（creator なし、review なし）を返すか、
	// （wrap された）domain.ErrShopNotFound を返す。
	GetShop(ctx context.Context, id string) (domain.Shop, error)
	// GetShopBurger は、burger が shops_burgers 経由でその shop に紐づいて
	// いるときに限り、stats つき（まだ計算されていなければゼロ）の burger を
	// 返し、そうでなければ（wrap された）domain.ErrBurgerNotFound を返す。
	GetShopBurger(ctx context.Context, shopID, burgerID string) (domain.ShopReviewBurger, error)
	// ListReviewShops は、review が属する shop(review の burger を持つ shop すべて)を、作成の古い順に返す。
	// 存在しない review・削除済みの review・shop に紐づかない burger の review は、空の一覧を返す(エラーではない)。
	ListReviewShops(ctx context.Context, reviewID string) ([]domain.Shop, error)
}

// ReviewListFilter は、GET /reviews の省略可能なクエリフィルタを保持する。
// user_id 以外は Rails の ReviewQuery を再現しており、指定されたフィルタは
// それぞれフィードを絞り込み、指定されたフィルタはすべて AND で組み合わされる。
// nil の Rating/ShopID/UserID と空の Keyword は「absent」を意味する（Rails の
// params[:x].present?）。したがって、0 や負の id/rating が指定された場合も
// フィルタとして働く（結果は空のページになる。Rating/ShopID は Rails と
// まったく同様で、Rails に対応物のない UserID も同様に扱う）。
type ReviewListFilter struct {
	// Rating は rating の完全一致フィルタである（Rails の by_rating）。
	Rating *int
	// Keyword は、comment に対するリテラルで大文字小文字を区別しない部分文字列
	// 一致である（Rails の keyword_search、comment ILIKE %escaped%）。
	Keyword string
	// ShopID は、shops_burgers 経由でその shop に burger が紐づいている review
	// だけを残す（Rails の shops_and_burgers の join）。ただし対象の shop 自身も
	// active でなければならず、active でない（または存在しない）shop の id を
	// 指定すると結果は空になる。
	ShopID *string
	// UserID は、その user が書いた review だけを残す（本 API の拡張で、Rails の
	// ReviewQuery にはない）。公開ルールは維持される：discard 済みの review や
	// user、active な shop に紐づかない burger の review は、UserID を指定しても
	// 現れない。存在しない（または discard 済みの）user の id を指定すると結果は
	// 空になる。
	UserID *string
	// BurgerID は、その burger の review だけを残す（本 API の拡張で、Rails の
	// ReviewQuery にはない）。UserID と同じ形で AND 結合され、公開ルールは維持される。
	// 存在しない（または active な shop に紐づかない）burger の id を指定すると結果は空になる。
	BurgerID *string
}

// Reviews は review の use case を実装する。公開フィードと詳細、および
// author に限定された create/edit/delete であり、review ごとに任意で 1 枚の
// 写真を photos 経由で保存する。読み取りは query、書き込みは domain の書き込みオブジェクトを通し、
// repository には依存しない。
//
// レビューの書き込みと、バーガーの統計の再計算の依頼の登録は、UnitOfWork(ここからここまでの書き込みと
// 読み取りを、まとめて 1 つのトランザクションにする範囲を、usecase が指定する仕組み)の中で行う。途中で
// エラーになれば全体を取り消すので、レビューだけ、または依頼だけが反映されることがない。統計そのものは、
// あとからバックグラウンドのワーカーが計算する(書き込みは統計の計算を待たない)。
type Reviews struct {
	query     ReviewQuery
	uow       UnitOfWork
	recalc    *BurgerStatsRecalculator
	shopStats *ShopStatsRecalculator
	photos    PhotoStorage
}

// NewReviews は review の use case を配線する。photos は non-nil でなければ
// ならない（本番では disk か S3、テストでは fake）。どのリクエスト経路も
// それを dereference しうる（photoURL、deletePhotoBestEffort）ので、nil の
// storage は、リクエストの途中で panic するのではなく、ここで fail-loud する。
func NewReviews(query ReviewQuery, uow UnitOfWork, recalc *BurgerStatsRecalculator, shopStats *ShopStatsRecalculator, photos PhotoStorage) *Reviews {
	if photos == nil {
		panic("usecase.NewReviews: nil PhotoStorage")
	}
	if shopStats == nil {
		panic("usecase.NewReviews: nil ShopStatsRecalculator")
	}
	return &Reviews{query: query, uow: uow, recalc: recalc, shopStats: shopStats, photos: photos}
}

// List は、filter で絞り込んだ公開 review フィードを返す。ページネーションは
// clampPage に従い、Shops.List と同じフォールバック規則で行う。viewer（nil = 匿名）は
// 絞り込みには関与せず、各 review の CanEdit の設定だけに使う。2 つ目の戻り値は、
// 次のページがあるか（has_more）である。
func (s *Reviews) List(ctx context.Context, viewer *domain.User, filter ReviewListFilter, page, perPage int) ([]domain.ReviewDetail, bool, error) {
	limit, offset := clampPage(page, perPage)
	reviews, hasMore, err := s.query.ListReviews(ctx, filter, limit, offset)
	if err != nil {
		return nil, false, fmt.Errorf("list reviews: %w", err)
	}
	for i := range reviews {
		reviews[i].PhotoURL = s.photoURL(reviews[i].PhotoKey)
		reviews[i].CanEdit = reviews[i].CanBeModifiedByViewer(viewer)
	}
	return reviews, hasMore, nil
}

// Get は author、burger、stats つきの review を 1 件返す。存在しない review、
// discard 済みの review、author が discard 済みの user である review は、
// いずれも domain.ErrReviewNotFound を返す。viewer（nil = 匿名）は CanEdit・Shop・CanReview の
// 設定に使う。Shop と CanReview は、review の burger を持つショップ（複数ありうる）から、domain の
// ReviewShopFor が viewer ごとに決める（viewer に見えるショップがなければ、Shop は nil で CanReview は false。
// エラーにはしない）。CanReview の規則は、ショップ詳細の can_review と同じ Shop.CanBeReviewedByViewer である。
func (s *Reviews) Get(ctx context.Context, viewer *domain.User, id string) (domain.ReviewDetail, error) {
	detail, err := s.query.GetReview(ctx, id)
	if err != nil {
		return domain.ReviewDetail{}, fmt.Errorf("get review: %w", err)
	}
	detail.PhotoURL = s.photoURL(detail.PhotoKey)
	detail.CanEdit = detail.CanBeModifiedByViewer(viewer)
	shops, err := s.query.ListReviewShops(ctx, id)
	if err != nil {
		return domain.ReviewDetail{}, fmt.Errorf("list review shops: %w", err)
	}
	detail.Shop, detail.CanReview = domain.ReviewShopFor(shops, viewer)
	return detail, nil
}

// Create は、viewer による、shop の burger に対する review を投稿する。
// チェックの順序は契約どおりで、未知の shop（404）、次に domain の
// reviewable ルール（403）、次に burger の解決、次に content の validation
// （422）である。正の burgerID が優先され、shop に紐づく burger を指して
// いなければならない（そうでなければ 404）。そうでない場合は、空白でない
// burgerName が、insert と同じ transaction 内で、完全一致かつ trim されて
// いない名前で shop の burger を探し、なければ作る（Rails parity）。
// どちらでもない場合は validation の失敗（422）であり、黙って
// デフォルトを使うことは決してない。レスポンスの detail は、viewer と、
// 存在確認のために解決した burger から組み立てる。再取得はしない。バーガー名の経路でのバーガーの
// 作成、レビューの登録、統計の再計算の依頼の登録は、1 つの UnitOfWork(まとめて 1 つのトランザクションにする範囲)の
// 中で行うので、どの段階で失敗しても、レビューのない作りかけのバーガーが残ることはない。nil でない
// upload（handler で validate 済み/正規化済み）は、insert の前に新しい
// ランダムな key で保存される。その後 insert が失敗した場合は、アップロード
// したばかりの blob を best-effort で削除するので、リクエストより長く残る
// 孤立ファイルはない。
func (s *Reviews) Create(ctx context.Context, viewer domain.User, shopID, burgerID string, burgerName string, rating int, comment string, upload *photo.Processed) (domain.ReviewDetail, error) {
	shop, err := s.query.GetShop(ctx, shopID)
	if err != nil {
		return domain.ReviewDetail{}, fmt.Errorf("create review: %w", err)
	}
	if !shop.CanBeReviewedBy(viewer) {
		return domain.ReviewDetail{}, domain.ErrForbidden
	}
	var burger domain.ShopReviewBurger
	if burgerID != "" {
		if burger, err = s.query.GetShopBurger(ctx, shopID, burgerID); err != nil {
			return domain.ReviewDetail{}, fmt.Errorf("create review: %w", err)
		}
	} else if err := domain.ValidateBurgerName(burgerName); err != nil {
		return domain.ReviewDetail{}, err
	}
	// burgerID が空のとき（burger_name の経路）は、この BurgerID は
	// 使われない。永続化の実装が transaction 内で burger を解決して上書きする。
	review, err := domain.NewReview(rating, comment, viewer.ID, burgerID)
	if err != nil {
		return domain.ReviewDetail{}, err
	}
	if review.PhotoKey, err = s.putPhoto(ctx, upload); err != nil {
		return domain.ReviewDetail{}, fmt.Errorf("create review: %w", err)
	}
	var created domain.Review
	err = s.uow.Do(ctx, func(ctx context.Context, tx Tx) error {
		if burgerID == "" {
			// バーガー名で投稿する場合は、このトランザクションの中で、名前のバーガーを探し、
			// なければ作る。返るバーガーの統計は、この投稿より前の値である。
			var err error
			if burger, err = tx.Reviews.CreateShopBurger(ctx, shopID, burgerName); err != nil {
				return err
			}
			review.BurgerID = burger.ID
		}
		var err error
		if created, err = tx.Reviews.Create(ctx, review); err != nil {
			return err
		}
		// 統計は、ここでは計算せず、再計算の依頼(バーガーの統計とショップの集計の両方)を同じトランザクションで
		// 登録する。burger の行をロックしないので、同じ burger への他の書き込みを待たない。
		if err := s.recalc.RequestRecalculation(ctx, tx, created.BurgerID); err != nil {
			return err
		}
		return s.shopStats.RequestRecalculationForBurger(ctx, tx, created.BurgerID)
	})
	if err != nil {
		s.deletePhotoBestEffort(ctx, review.PhotoKey)
		return domain.ReviewDetail{}, fmt.Errorf("create review: %w", err)
	}
	detail := domain.ReviewDetail{
		Review:   created,
		User:     &domain.UserRef{ID: viewer.ID, Username: viewer.Username},
		Burger:   &burger,
		PhotoURL: s.photoURL(created.PhotoKey),
	}
	detail.CanEdit = detail.CanBeModifiedByViewer(&viewer)
	return detail, nil
}

// Update は review の rating と comment を編集する。load（存在しない review、
// discard 済みの review、author が discard 済みの user である review は
// いずれも 404）、domain の所有権ルール（403。
// admin でも通らない）、content の validation（422）、
// そしてカラム限定の書き込みの順で行う。保存された行は load した detail に
// マージされるので、レスポンスは再取得なしで author、burger、stats を持つ。
// nil でない upload は写真を置き換える。新しい blob を先に保存し、
// 続いて content と photo_key を「1 つの」transaction で切り替え（失敗しても
// content だけが key なしで commit されることは決してない）、その DB の成功の
// 後にはじめて古い blob を best-effort で削除する。
// nil の upload は content だけの書き込みを行い、photo_key には触れない
// （写真を削除する経路はない）。
func (s *Reviews) Update(ctx context.Context, viewer domain.User, id string, rating int, comment string, upload *photo.Processed) (domain.ReviewDetail, error) {
	detail, err := s.query.GetReview(ctx, id)
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
	err = s.uow.Do(ctx, func(ctx context.Context, tx Tx) error {
		var err error
		if newKey != nil {
			updated, err = tx.Reviews.UpdateContentAndPhotoKey(ctx, id, rating, comment, newKey)
		} else {
			updated, err = tx.Reviews.UpdateContent(ctx, id, rating, comment)
		}
		if err != nil {
			return err
		}
		if err := s.recalc.RequestRecalculation(ctx, tx, updated.BurgerID); err != nil {
			return err
		}
		return s.shopStats.RequestRecalculationForBurger(ctx, tx, updated.BurgerID)
	})
	if err != nil {
		if newKey != nil {
			s.deletePhotoBestEffort(ctx, newKey)
		}
		return domain.ReviewDetail{}, fmt.Errorf("update review: %w", err)
	}
	if newKey != nil {
		// 古い blob が参照されなくなるのは、DB が新しい key を指すように
		// なった今になってからである。それを失っても、漏れたファイルに
		// なるだけで、review が壊れることはない。
		s.deletePhotoBestEffort(ctx, detail.PhotoKey)
	}
	detail.Review = updated
	detail.PhotoURL = s.photoURL(updated.PhotoKey)
	detail.CanEdit = detail.CanBeModifiedByViewer(&viewer)
	return detail, nil
}

// Delete は review を soft delete する。load（404）、domain の所有権ルール
// （403、Update と同様に author のみ）、そしてカラム限定の discard の順で
// 行い、hard DELETE は決して行わない。写真の blob があれば、discard が
// 成功した後に best-effort で削除される。
func (s *Reviews) Delete(ctx context.Context, viewer domain.User, id string) error {
	detail, err := s.query.GetReview(ctx, id)
	if err != nil {
		return fmt.Errorf("delete review: %w", err)
	}
	if !detail.CanBeModifiedBy(viewer) {
		return domain.ErrForbidden
	}
	err = s.uow.Do(ctx, func(ctx context.Context, tx Tx) error {
		if err := tx.Reviews.Discard(ctx, id); err != nil {
			return err
		}
		if err := s.recalc.RequestRecalculation(ctx, tx, detail.BurgerID); err != nil {
			return err
		}
		return s.shopStats.RequestRecalculationForBurger(ctx, tx, detail.BurgerID)
	})
	if err != nil {
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
