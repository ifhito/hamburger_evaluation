package domain

// このファイルは、検証の失敗として利用者に返す文言のカタログ(英語・日本語)である。
// 英語は、API の文言の契約なので、一字一句、変えない。日本語は、デザイン(design/redesign)の日本語と
// 語調をそろえる(です・ます調の短い文。「〜してください」「〜が長すぎます」)。値(上限の数字など)は、
// どちらの言語にも、同じ値が入る(構造テスト messages_test.go が、書式の値の並びの食い違いを検出する)。
// キーを足したら、英語と日本語の両方を書く(片方が空だと、構造テストが失敗する)。

const (
	keyReviewRatingRange = "review.rating_range"
	keyCommentTooLong    = "review.comment_too_long"
	keyVisitedAtFuture   = "review.visited_at_future"
	keyBurgerNameBlank   = "burger_name.blank"
	keyBurgerNameTooLong = "burger_name.too_long"
	keyShopNameBlank     = "shop_name.blank"
	keyShopNameTooLong   = "shop_name.too_long"
	keyModerationNote    = "moderation_note.too_long"
	keyShopCannotClose   = "shop.cannot_close"
	keyShopCannotReopen  = "shop.cannot_reopen"
	keyBioTooLong        = "bio.too_long"
	keyUsernameBlank     = "username.blank"
	keyUsernameTooLong   = "username.too_long"
	keyEmailBlank        = "email.blank"
	keyEmailTooLong      = "email.too_long"
	keyEmailInvalid      = "email.invalid"
	keyEmailTaken        = "email.taken"
	keyPasswordBlank     = "password.blank"
	keyPasswordTooShort  = "password.too_short"
	keyPasswordTooLong   = "password.too_long"
	keyPasswordCharKinds = "password.char_kinds"
	keyPasswordConfirm   = "password.confirmation_mismatch"
)

// catalog は、検証の失敗の文言のカタログである。キーは Message.Key。
var catalog = map[string]Entry{
	keyReviewRatingRange: {EN: "Rating must be in %d..%d", JA: "評価は %d〜%d の整数で指定してください"},
	keyCommentTooLong:    {EN: "Comment is too long (maximum is %d characters)", JA: "コメントが長すぎます(最大 %d 文字)"},
	keyVisitedAtFuture:   {EN: "Visited at can't be in the future", JA: "実食日には、未来の日付を指定できません"},
	keyBurgerNameBlank:   {EN: "Burger name can't be blank", JA: "バーガーの名前を入力してください"},
	keyBurgerNameTooLong: {EN: "Burger name is too long (maximum is %d characters)", JA: "バーガーの名前が長すぎます(最大 %d 文字)"},
	keyShopNameBlank:     {EN: "Name can't be blank", JA: "ショップの名前を入力してください"},
	keyShopNameTooLong:   {EN: "Name is too long (maximum is %d characters)", JA: "ショップの名前が長すぎます(最大 %d 文字)"},
	keyModerationNote:    {EN: "Moderation note is too long (maximum is %d characters)", JA: "却下の理由が長すぎます(最大 %d 文字)"},
	keyShopCannotClose:   {EN: "Shop cannot be closed", JA: "このショップは閉業にできません"},
	keyShopCannotReopen:  {EN: "Shop is not closed", JA: "このショップは閉業していません"},
	keyBioTooLong:        {EN: "Bio is too long (maximum is %d characters)", JA: "自己紹介が長すぎます(最大 %d 文字)"},
	keyUsernameBlank:     {EN: "Username can't be blank", JA: "ユーザー名を入力してください"},
	keyUsernameTooLong:   {EN: "Username is too long (maximum is %d characters)", JA: "ユーザー名が長すぎます(最大 %d 文字)"},
	keyEmailBlank:        {EN: "Email can't be blank", JA: "メールアドレスを入力してください"},
	keyEmailTooLong:      {EN: "Email is too long (maximum is %d characters)", JA: "メールアドレスが長すぎます(最大 %d 文字)"},
	keyEmailInvalid:      {EN: "Email is invalid", JA: "メールアドレスの形式が正しくありません"},
	keyEmailTaken:        {EN: "Email has already been taken", JA: "このメールアドレスは、すでに使われています"},
	keyPasswordBlank:     {EN: "Password can't be blank", JA: "パスワードを入力してください"},
	// パスワードの長さは、文字数ではなくバイト数で数える(bcrypt の入力の上限に合わせる)。英語の文言は
	// 契約なので「characters」のまま変えない。日本語は、実際の数え方(バイト)を、そのまま書く。
	keyPasswordTooShort:  {EN: "Password is too short (minimum is %d characters)", JA: "パスワードが短すぎます(最小 %d バイト)"},
	keyPasswordTooLong:   {EN: "Password is too long (maximum is %d characters)", JA: "パスワードが長すぎます(最大 %d バイト)"},
	keyPasswordCharKinds: {EN: "Password must include letters, numbers and symbols", JA: "パスワードには、半角の英字・数字・記号を、それぞれ 1 文字以上含めてください"},
	keyPasswordConfirm:   {EN: "Password confirmation doesn't match Password", JA: "パスワード(確認)が、パスワードと一致しません"},
}

// MsgEmailTaken は、すでに使われているメールアドレスへの変更を断る文言である(プロフィールの更新。
// 登録の有無を判別させない signup の経路では使わない)。
var MsgEmailTaken = Msg(keyEmailTaken)

// MsgPasswordConfirmationMismatch は、確認欄がパスワードと一致しないときの文言である。確認欄は入力欄同士の
// 整合の確認(フォームの都合)で、サービスの規則ではないので、判定は use case に置くが、検証の失敗の文言は、
// ほかの検証の文言と同じカタログに置く。
var MsgPasswordConfirmationMismatch = Msg(keyPasswordConfirm)

// MsgShopCannotClose は、閉業できない shop(pending・rejected、またはすでに閉業した shop)を
// 閉業しようとしたときの文言である。遷移の可否(Shop.CanBeClosed)は domain が判断するが、
// usecase が使うので、カタログの文言として公開する。
var MsgShopCannotClose = Msg(keyShopCannotClose)

// MsgShopCannotReopen は、閉業していない shop を再開しようとしたときの文言である。遷移の可否
// (Shop.CanBeReopened)は domain が判断するが、usecase が使うので、カタログの文言として公開する。
var MsgShopCannotReopen = Msg(keyShopCannotReopen)
