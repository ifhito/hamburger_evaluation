package handler

import "github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"

// このファイルは、handler が返す 4xx の文言(見つからない・権限・認証・入力の形・写真)のカタログ(英語・
// 日本語)である。検証の失敗(422)の文言は、domain のカタログ(domain/messages.go)にある。英語は、API の
// 文言の契約なので、一字一句、変えない。日本語は、デザイン(design/redesign)の語調と用語にそろえる
// (です・ます調の短い文。ログインでなく「サインイン」)。キーを足したら、英語と日本語の両方を書く
// (構造テスト messages_test.go が強制する)。
//
// 5xx の `internal server error` と `database unavailable` は、利用者に見せる文言ではなく、運用者向けの
// 固定の文字列なので、カタログに入れず、英語のままにする。OAuth の RFC のプロトコルの識別子も同じ。

const (
	keyUserNotFound       = "not_found.user"
	keyShopNotFound       = "not_found.shop"
	keyReviewNotFound     = "not_found.review"
	keyBurgerNotFound     = "not_found.burger"
	keyRouteNotFound      = "not_found.route"
	keyForbidden          = "forbidden"
	keyUnauthorized       = "unauthorized"
	keyInvalidCredentials = "invalid_credentials"
	keySignupTokenInvalid = "signup.token_invalid"
	keyInvalidJSON        = "request.invalid_json"
	keyInvalidBody        = "request.invalid_body"
	keyBodyTooLarge       = "request.body_too_large"
	keyInvalidMultipart   = "request.invalid_multipart"
	keyDuplicatePhoto     = "request.duplicate_photo"
	keyFieldTooLarge      = "request.field_too_large"
	keyMethodNotAllowed   = "request.method_not_allowed"
	keyPhotoTooLarge      = "photo.too_large"
	keyPhotoDimensions    = "photo.dimensions_too_large"
	keyPhotoUnsupported   = "photo.unsupported"
	keyPhotoHEIF          = "photo.heif_not_supported"
	keyRatingNotInteger   = "param.rating_not_integer"
	keyShopIDInvalid      = "param.shop_id_invalid"
	keyBurgerIDInvalid    = "param.burger_id_invalid"
	keyUserIDInvalid      = "param.user_id_invalid"
	keyPageNotInteger     = "param.page_not_integer"
	keyPerPageNotInteger  = "param.per_page_not_integer"
)

var catalog = map[string]domain.Entry{
	keyUserNotFound:       {EN: "User not found", JA: "ユーザーが見つかりません"},
	keyShopNotFound:       {EN: "Shop not found", JA: "ショップが見つかりません"},
	keyReviewNotFound:     {EN: "Review not found", JA: "レビューが見つかりません"},
	keyBurgerNotFound:     {EN: "Burger not found", JA: "バーガーが見つかりません"},
	keyRouteNotFound:      {EN: "not found", JA: "見つかりません"},
	keyForbidden:          {EN: "Forbidden", JA: "この操作をする権限がありません"},
	keyUnauthorized:       {EN: "Unauthorized", JA: "サインインが必要です"},
	keyInvalidCredentials: {EN: "Invalid email or password", JA: "メールアドレスかパスワードが違います"},
	keySignupTokenInvalid: {EN: "Confirmation token is invalid or has expired", JA: "確認のリンクが、無効か期限切れです"},
	keyInvalidJSON:        {EN: "invalid JSON body", JA: "送られた内容が、JSON として読めません"},
	keyInvalidBody:        {EN: "invalid request body", JA: "送られた内容が読めません"},
	keyBodyTooLarge:       {EN: "request body too large", JA: "送られた内容が大きすぎます"},
	keyInvalidMultipart:   {EN: "invalid multipart body", JA: "送られた内容(multipart)が読めません"},
	keyDuplicatePhoto:     {EN: "duplicate photo field", JA: "写真は 1 枚だけ送ってください"},
	keyFieldTooLarge:      {EN: "multipart field too large", JA: "送られた項目が大きすぎます"},
	keyMethodNotAllowed:   {EN: "method not allowed", JA: "このメソッドは使えません"},
	keyPhotoTooLarge:      {EN: "Photo is too large (max %dMB)", JA: "写真が大きすぎます(最大 %d MB)"},
	keyPhotoDimensions:    {EN: "Photo dimensions are too large (max %dpx per side and %d megapixels)", JA: "写真の縦横が大きすぎます(1 辺が最大 %d px、全体で最大 %d メガピクセル)"},
	keyPhotoUnsupported:   {EN: "Photo must be a JPEG, PNG, or WebP image", JA: "写真は、JPEG・PNG・WebP の画像にしてください"},
	keyPhotoHEIF:          {EN: "Photo must be a JPEG, PNG, or WebP image (HEIC/HEIF is not supported)", JA: "写真は、JPEG・PNG・WebP の画像にしてください(HEIC・HEIF は使えません)"},
	keyRatingNotInteger:   {EN: "Rating must be an integer", JA: "評価は、整数で指定してください"},
	keyShopIDInvalid:      {EN: "Shop id must be a valid UUID", JA: "ショップの ID の形式が正しくありません"},
	keyBurgerIDInvalid:    {EN: "Burger id must be a valid UUID", JA: "バーガーの ID の形式が正しくありません"},
	keyUserIDInvalid:      {EN: "User id must be a valid UUID", JA: "ユーザーの ID の形式が正しくありません"},
	keyPageNotInteger:     {EN: "Page must be an integer", JA: "ページは、整数で指定してください"},
	keyPerPageNotInteger:  {EN: "Per page must be an integer", JA: "1 ページの件数は、整数で指定してください"},
}

// text は、m を l の言語の文字列にする。handler のカタログにあればそれを、なければ domain のカタログ
// (検証の失敗の文言)を使う。
func text(l domain.Lang, m domain.Message) string {
	if entry, ok := catalog[m.Key]; ok {
		return entry.Format(l, m.Args...)
	}
	return m.Text(l)
}

var (
	msgRouteNotFound      = domain.Msg(keyRouteNotFound)
	msgUnauthorized       = domain.Msg(keyUnauthorized)
	msgInvalidCredentials = domain.Msg(keyInvalidCredentials)
	msgInvalidJSON        = domain.Msg(keyInvalidJSON)
	msgInvalidBody        = domain.Msg(keyInvalidBody)
	msgBodyTooLarge       = domain.Msg(keyBodyTooLarge)
	msgInvalidMultipart   = domain.Msg(keyInvalidMultipart)
	msgDuplicatePhoto     = domain.Msg(keyDuplicatePhoto)
	msgFieldTooLarge      = domain.Msg(keyFieldTooLarge)
	msgMethodNotAllowed   = domain.Msg(keyMethodNotAllowed)
	msgRatingNotInteger   = domain.Msg(keyRatingNotInteger)
	msgShopIDInvalid      = domain.Msg(keyShopIDInvalid)
	msgBurgerIDInvalid    = domain.Msg(keyBurgerIDInvalid)
	msgUserIDInvalid      = domain.Msg(keyUserIDInvalid)
	msgPageNotInteger     = domain.Msg(keyPageNotInteger)
	msgPerPageNotInteger  = domain.Msg(keyPerPageNotInteger)
)
