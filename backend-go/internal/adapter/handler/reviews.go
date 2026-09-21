package handler

import (
	"context"
	"errors"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/photo"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// reviewNotFoundMessage は、存在しない review、discard 済みの review、
// および数値でない review の id に共通の 404 body であり、soft delete 済みの
// review が一度も存在しなかったものと区別できないようにする。
const reviewNotFoundMessage = "Review not found"

// burgerNotFoundMessage は、存在しない burger、または要求された shop が
// 提供していない burger に対する 404 body である。
const burgerNotFoundMessage = "Burger not found"

// reviewParamsRequest は POST /reviews と PUT /reviews/{id} の
// {"review":{...}} ラッパーである（PUT は shop_id/burger_id/burger_name を
// 無視する。review が別の burger に移ることはない）。POST では、burger は
// burger_id で指定し、それがない場合は burger_name で指定する
// （find-or-create、frontend の契約、S6 P3-1）。優先順位は usecase の判断
// である。欠けているフィールドはゼロ値にデコードされ、rating、comment、
// （burger_id もない場合の）burger_name は domain が拒否するので 400 では
// なく Rails-parity の 422 になる。POST の shop_id は存在しない shop として
// 404 になる。
type reviewParamsRequest struct {
	Review struct {
		Rating     int    `json:"rating"`
		Comment    string `json:"comment"`
		ShopID     int64  `json:"shop_id"`
		BurgerID   int64  `json:"burger_id"`
		BurgerName string `json:"burger_name"`
	} `json:"review"`
}

const (
	// maxPhotoBytes は生の写真アップロードを 5 MiB に制限する（S10 AC3）。
	// これより大きなアップロードは 413 ではなく 422 になる。グローバルな
	// review の body cap の方が広い。
	maxPhotoBytes int64 = 5 << 20
	// maxMultipartTextBytes は multipart の review 投稿の各 text フィールドを
	// 制限する。写真の cap に比べれば小さいが、現実的なコメントには十分な
	// 余裕がある（コメントの文字数の上限は domain の検証が 422 で判定し、
	// これはその外側の、暴走した入力を止めるためのガードである）。
	maxMultipartTextBytes int64 = 64 << 10
)

const photoTooLargeMessage = "Photo is too large (max 5MB)"

const photoUnsupportedMessage = "Photo must be a JPEG, PNG, or WebP image"

// multipartReviewForm は multipart/form-data の review 投稿（S10 の wire
// 契約）のフラットなフィールドを保持する：reviewParamsRequest と同じ値に
// 加え、処理済みの写真（photo part がない場合は nil）。
type multipartReviewForm struct {
	rating     int
	comment    string
	shopID     int64
	burgerID   int64
	burgerName string
	photo      *photo.Processed
}

// isMultipart は request が multipart/form-data を宣言しているかどうかを
// 返す（S10 の写真投稿の経路）。それ以外はすべて既存の JSON の経路にとどまる
// （後方互換性）。
func isMultipart(r *http.Request) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && mediaType == "multipart/form-data"
}

// decodeReviewMultipart は multipart の body を r.MultipartReader 経由で
// ストリーミングし、body 全体を一度にバッファすることはない（ただし photo
// part は photo.Process が cap までの全量をメモリに読み込む）。
// name が "photo" の part は 5 MiB の cap の下で photo.Process により検証・
// 正規化される。それ以外の part は、名前が未知であっても readTextPart で
// フィールドごとの小さな cap の下に text として読み取られ、既知の名前
// （rating/comment/shop_id/burger_id/burger_name）の値だけが使われ、未知の
// 名前の値は捨てられる。false は、エラーレスポンスが既に書き込まれたことを
// 意味する：グローバルな body cap が作動した場合は 413、サイズ超過または
// 画像でない写真には 422、不正な multipart（photo part の重複や、text
// フィールドの cap 超過を含む）には 400 である。rating/shop_id/burger_id が
// 存在しない、または数値でない場合は 0 にデコードされ、JSON の body と同じ
// validation/not-found の経路に流れる。
func decodeReviewMultipart(w http.ResponseWriter, r *http.Request) (multipartReviewForm, bool) {
	var form multipartReviewForm
	mr, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart body")
		return form, false
	}
	for {
		part, err := mr.NextPart()
		// 意図的に厳密比較にしている（stdlib の ReadForm と同様）。Go 1.20
		// 以降、NextPart は、最後の boundary が「ない」まま正常に終わる body
		// （切り詰められているが HTTP としては完結している）に対して、%w で
		// 「wrap された」io.EOF を返すため、errors.Is ではその切り詰めを
		// 完全な form と誤認してしまう。
		if err == io.EOF { //nolint:errorlint // 上のコメントを参照
			return form, true
		}
		if err != nil {
			writeMultipartReadError(w, err)
			return form, false
		}
		name := part.FormName()
		if name == "photo" {
			if form.photo != nil {
				writeError(w, http.StatusBadRequest, "duplicate photo field")
				return form, false
			}
			processed, ok := readPhotoPart(r.Context(), w, part)
			if !ok {
				return form, false
			}
			form.photo = processed
			continue
		}
		value, ok := readTextPart(w, part)
		if !ok {
			return form, false
		}
		switch name {
		case "rating":
			form.rating, _ = strconv.Atoi(value)
		case "comment":
			form.comment = value
		case "shop_id":
			form.shopID, _ = strconv.ParseInt(value, 10, 64)
		case "burger_id":
			form.burgerID, _ = strconv.ParseInt(value, 10, 64)
		case "burger_name":
			form.burgerName = value
		}
		// 未知のフィールドは無視される。decodeJSON が未知のフィールドを
		// 許容するのと同じである。
	}
}

// readTextPart は text フィールド 1 つをフィールドごとの cap の下で読み取る。
// false は、エラーレスポンスが既に書き込まれたことを意味する。
func readTextPart(w http.ResponseWriter, part *multipart.Part) (string, bool) {
	data, err := io.ReadAll(io.LimitReader(part, maxMultipartTextBytes+1))
	if err != nil {
		writeMultipartReadError(w, err)
		return "", false
	}
	if int64(len(data)) > maxMultipartTextBytes {
		writeError(w, http.StatusBadRequest, "multipart field too large")
		return "", false
	}
	return string(data), true
}

// readPhotoPart は photo part を 5 MiB の cap の下で photo.Process へ
// ストリーミングする。+1 の sentinel バイトにより、超過分をバッファせずに
// "cap 超過" を検出する。Process の後、err の検査より先にサイズ超過を判定する
// ので、cap を超えるまで読み込まれた画像は、途中までデコードされた結果の
// エラーではなく too large として報告される。ただし Process は先頭の magic
// bytes が jpeg/png/webp でなければ残りを読まずに ErrUnsupportedImage を
// 返すため、巨大でも画像でないデータは too large ではなく unsupported になる。
// ctx（request の context）は、Process 内の decode semaphore の待機を制限
// する。false は、エラーレスポンスが既に書き込まれたことを意味する。
func readPhotoPart(ctx context.Context, w http.ResponseWriter, part *multipart.Part) (*photo.Processed, bool) {
	limited := &io.LimitedReader{R: part, N: maxPhotoBytes + 1}
	processed, err := photo.Process(ctx, limited)
	if limited.N == 0 { // part が maxPhotoBytes を超えるデータを保持していた
		writeJSON(w, http.StatusUnprocessableEntity, errorsResponse{Errors: []string{photoTooLargeMessage}})
		return nil, false
	}
	if err != nil {
		if errors.Is(err, photo.ErrUnsupportedImage) {
			writeJSON(w, http.StatusUnprocessableEntity, errorsResponse{Errors: []string{photoUnsupportedMessage}})
			return nil, false
		}
		// この経路での sentinel でない Process のエラーは、ストリーム途中の
		// 読み取り失敗、すなわち壊れた multipart body（またはグローバルな
		// body cap）、もしくは decode semaphore の待ち行列にいる間に request の
		// context がキャンセルされたことを表す。キャンセルは意図的に 400 の
		// 経路を共有する。client は既にいないので、どのみちレスポンスは
		// 観測できない。
		writeMultipartReadError(w, err)
		return nil, false
	}
	return &processed, true
}

// writeMultipartReadError は、multipart のストリーム途中の読み取り失敗を
// HTTP に対応させる：グローバルな body cap の MaxBytesReader は 413 として
// 現れ、それ以外は不正な body（400）として扱う。
func writeMultipartReadError(w http.ResponseWriter, err error) {
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return
	}
	writeError(w, http.StatusBadRequest, "invalid multipart body")
}

// newReviewResponse は domain の payload を、shop detail の reviews と共有する
// wire 形状（frontend の Review、domains/reviews/api/types.ts。ワイヤ上は snake_case）に対応させる。
func newReviewResponse(detail domain.ReviewDetail) shopReviewResponse {
	resp := shopReviewResponse{
		ID:        detail.ID,
		Rating:    detail.Rating,
		Comment:   detail.Comment,
		CreatedAt: detail.CreatedAt.UTC().Format(time.RFC3339),
		PhotoURL:  detail.PhotoURL,
		User:      newUserRefResponse(detail.User),
	}
	if detail.Burger != nil {
		resp.Burger = &reviewBurgerResponse{
			ID:            detail.Burger.ID,
			Name:          detail.Burger.Name,
			AverageRating: detail.Burger.AverageRating,
			ReviewCount:   detail.Burger.ReviewCount,
			WeightedScore: detail.Burger.WeightedScore,
			Confidence:    detail.Burger.Confidence,
		}
	}
	return resp
}

// reviewIDPathValue は {id} の path value をパースする。false は統一された
// review の 404 が既に書き込まれたことを意味する（数値でない id は存在しない
// review とまったく同じに見える。shop の id と同じ規約）。
func reviewIDPathValue(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, reviewNotFoundMessage)
		return 0, false
	}
	return id, true
}

// writeReviewError は review の usecase のエラーを HTTP に対応させる：
// domain の認可の判断は 403、3 つの not-found sentinel はそれぞれの
// endpoint 固有の 404 body、validation は 422、それ以外は 500 である。
func writeReviewError(w http.ResponseWriter, op string, err error) {
	var vErr *domain.ValidationError
	switch {
	case errors.Is(err, domain.ErrForbidden):
		writeError(w, http.StatusForbidden, forbiddenMessage)
	case errors.Is(err, domain.ErrReviewNotFound):
		writeError(w, http.StatusNotFound, reviewNotFoundMessage)
	case errors.Is(err, domain.ErrShopNotFound):
		writeError(w, http.StatusNotFound, shopNotFoundMessage)
	case errors.Is(err, domain.ErrBurgerNotFound):
		writeError(w, http.StatusNotFound, burgerNotFoundMessage)
	case errors.As(err, &vErr):
		writeJSON(w, http.StatusUnprocessableEntity, errorsResponse{Errors: vErr.Messages})
	default:
		log.Printf("reviews: %s: %v", op, err)
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

// reviewListFilter は GET /reviews の省略可能な rating/keyword/shop_id/user_id の
// クエリフィルタをパースする（Rails ReviewQuery。user_id は本 API の拡張）。
// 空の値は存在しないものと数える（params[:x].present?）。false は、rating、
// shop_id、user_id のいずれかが整数でない場合の 422 が既に書き込まれたことを
// 意味する。rating と shop_id については Rails からの意図的な fail-loud な
// 乖離であり（Rails はゴミを 0 にキャストして黙って空のリストを返す）、
// user_id は Rails に対応物がないため、同じ fail-loud の形に揃えただけである。
func reviewListFilter(w http.ResponseWriter, r *http.Request) (usecase.ReviewListFilter, bool) {
	filter := usecase.ReviewListFilter{Keyword: r.URL.Query().Get("keyword")}
	if raw := r.URL.Query().Get("rating"); raw != "" {
		rating, err := strconv.Atoi(raw)
		if err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, errorsResponse{Errors: []string{"Rating must be an integer"}})
			return usecase.ReviewListFilter{}, false
		}
		filter.Rating = &rating
	}
	if raw := r.URL.Query().Get("shop_id"); raw != "" {
		shopID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, errorsResponse{Errors: []string{"Shop id must be an integer"}})
			return usecase.ReviewListFilter{}, false
		}
		filter.ShopID = &shopID
	}
	if raw := r.URL.Query().Get("user_id"); raw != "" {
		userID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, errorsResponse{Errors: []string{"User id must be an integer"}})
			return usecase.ReviewListFilter{}, false
		}
		filter.UserID = &userID
	}
	return filter, true
}

// handleListReviews は GET /reviews を処理する：active な shop の burger の
// review の、公開されたトップレベルの JSON 配列で、任意で rating/keyword/
// shop_id/user_id のフィルタにより絞り込まれ、新しい順で、ページネーションされる。
// OptionalAuth の viewer は、ここでは絞り込みに関与しない。filter と
// page / per_page のどちらも不正な場合は、filter の 422 が先に返る。
func handleListReviews(reviews *usecase.Reviews) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter, ok := reviewListFilter(w, r)
		if !ok {
			return
		}
		page, perPage, ok := pageParams(w, r)
		if !ok {
			return
		}
		list, err := reviews.List(r.Context(), filter, page, perPage)
		if err != nil {
			log.Printf("reviews: list: %v", err)
			writeError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		resp := make([]shopReviewResponse, 0, len(list)) // nil ではない：[] として marshal される
		for _, detail := range list {
			resp = append(resp, newReviewResponse(detail))
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

// handleGetReview は GET /reviews/{id} を処理する：author、burger、stats を
// 伴う review、または未知の id、discard 済みの review、author が discard 済みの
// user である review、数値でない id に対する統一された 404。
func handleGetReview(reviews *usecase.Reviews) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := reviewIDPathValue(w, r)
		if !ok {
			return
		}
		detail, err := reviews.Get(r.Context(), id)
		if err != nil {
			writeReviewError(w, "get", err)
			return
		}
		writeJSON(w, http.StatusOK, newReviewResponse(detail))
	}
}

// handleCreateReview は RequireAuth の背後で POST /reviews を処理する：作成
// された review を伴う 201。チェックの順序（shop の 404、reviewable の 403、
// burger の 404、validation の 422）は usecase で決まり、ここでは決して
// 決めない。
func handleCreateReview(reviews *usecase.Reviews) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		viewer, ok := requireViewer(w, r)
		if !ok {
			return
		}
		var form multipartReviewForm
		if isMultipart(r) {
			var ok bool
			if form, ok = decodeReviewMultipart(w, r); !ok {
				return
			}
		} else {
			var req reviewParamsRequest
			if !decodeJSON(w, r, &req) {
				return
			}
			form = multipartReviewForm{
				rating:     req.Review.Rating,
				comment:    req.Review.Comment,
				shopID:     req.Review.ShopID,
				burgerID:   req.Review.BurgerID,
				burgerName: req.Review.BurgerName,
			}
		}
		detail, err := reviews.Create(r.Context(), viewer, form.shopID, form.burgerID,
			form.burgerName, form.rating, form.comment, form.photo)
		if err != nil {
			writeReviewError(w, "create", err)
			return
		}
		writeJSON(w, http.StatusCreated, newReviewResponse(detail))
	}
}

// handleUpdateReview は RequireAuth の背後で PUT /reviews/{id} を処理する：
// 完全な payload を伴う 200。所有権（author のみ、admin の例外なし）は
// domain の判断であり、403 として表面化する。
func handleUpdateReview(reviews *usecase.Reviews) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		viewer, ok := requireViewer(w, r)
		if !ok {
			return
		}
		id, ok := reviewIDPathValue(w, r)
		if !ok {
			return
		}
		// PUT が使うのは rating、comment、写真だけである。multipart パーサーの
		// その他のフィールドは、JSON body の shop_id/burger_id/burger_name と
		// まったく同じように無視される。
		var form multipartReviewForm
		if isMultipart(r) {
			var ok bool
			if form, ok = decodeReviewMultipart(w, r); !ok {
				return
			}
		} else {
			var req reviewParamsRequest
			if !decodeJSON(w, r, &req) {
				return
			}
			form = multipartReviewForm{rating: req.Review.Rating, comment: req.Review.Comment}
		}
		detail, err := reviews.Update(r.Context(), viewer, id, form.rating, form.comment, form.photo)
		if err != nil {
			writeReviewError(w, "update", err)
			return
		}
		writeJSON(w, http.StatusOK, newReviewResponse(detail))
	}
}

// handleDeleteReview は RequireAuth の背後で DELETE /reviews/{id} を処理する：
// soft delete で、body なしの 204 を返す。
func handleDeleteReview(reviews *usecase.Reviews) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		viewer, ok := requireViewer(w, r)
		if !ok {
			return
		}
		id, ok := reviewIDPathValue(w, r)
		if !ok {
			return
		}
		if err := reviews.Delete(r.Context(), viewer, id); err != nil {
			writeReviewError(w, "delete", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
