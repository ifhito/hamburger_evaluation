package domain

import "strings"

// Prefecture は都道府県である。JIS X 0401 の都道府県コード(1〜47)と正式な日本語の名前だけを持ち、
// PrefectureOf でしか作れない(表にないコードの都道府県は存在しない)。ショップの住所と一覧の絞り込みに使う。
// 表はここだけが持ち、frontend は GET /meta の表を表示に使う。
type Prefecture struct {
	code int
	name string
}

// prefectureNames は、コードの昇順の 47 都道府県の正式な名前である(添字 + 1 がコード)。
var prefectureNames = [...]string{
	"北海道", "青森県", "岩手県", "宮城県", "秋田県", "山形県", "福島県", "茨城県",
	"栃木県", "群馬県", "埼玉県", "千葉県", "東京都", "神奈川県", "新潟県", "富山県",
	"石川県", "福井県", "山梨県", "長野県", "岐阜県", "静岡県", "愛知県", "三重県",
	"滋賀県", "京都府", "大阪府", "兵庫県", "奈良県", "和歌山県", "鳥取県", "島根県",
	"岡山県", "広島県", "山口県", "徳島県", "香川県", "愛媛県", "高知県", "福岡県",
	"佐賀県", "長崎県", "熊本県", "大分県", "宮崎県", "鹿児島県", "沖縄県",
}

// PrefectureOf は、コードの都道府県を返す。表にないコード(1〜47 以外)は "Prefecture is invalid" の
// *ValidationError である。DB の CHECK 制約 shops_prefecture_code_range と同じ範囲である。
func PrefectureOf(code int) (Prefecture, error) {
	if code < 1 || code > len(prefectureNames) {
		return Prefecture{}, NewValidationError(Msg(keyPrefectureInvalid))
	}
	return Prefecture{code: code, name: prefectureNames[code-1]}, nil
}

// Code は、JIS X 0401 の都道府県コード(1〜47)である。
func (p Prefecture) Code() int { return p.code }

// Name は、正式な日本語の名前(例: 東京都)である。
func (p Prefecture) Name() string { return p.name }

// Prefectures は、47 都道府県をコードの昇順で返す(呼び出し側が書き換えても表は変わらない)。
func Prefectures() []Prefecture {
	all := make([]Prefecture, len(prefectureNames))
	for i, name := range prefectureNames {
		all[i] = Prefecture{code: i + 1, name: name}
	}
	return all
}

// MaxCityChars は市区町村の文字数の上限(Unicode のコードポイント数)である。
// DB の CHECK 制約 shops_city_max_length(000020_add_shop_address)と同じ値でなければならない。
// 食い違いは db/migrations_test.go が検出する。
const MaxCityChars = 100

// MaxStreetAddressChars は番地以降の文字数の上限(Unicode のコードポイント数)である。
// DB の CHECK 制約 shops_street_address_max_length(000020_add_shop_address)と同じ値でなければならない。
// 食い違いは db/migrations_test.go が検出する。
const MaxStreetAddressChars = 200

// Address はショップの住所である。どの項目も任意で、NewAddress でしか作れず、作ったあとは変わらない。
// ゼロ値は「住所なし」(すべて未設定)である。
type Address struct {
	// prefecture は都道府県で、ゼロ値(コード 0)は未設定である。
	prefecture    Prefecture
	city          string
	streetAddress string
}

// NewAddress は住所を検証して作る。prefectureCode は nil なら未設定で、値があるときは PrefectureOf の規則に
// 従う。city(市区町村。例: 渋谷区)と streetAddress(番地以降。例: 神南1-2-3)は前後の空白を取り除き
// (空白だけなら未設定の空文字)、それぞれ MaxCityChars・MaxStreetAddressChars 文字まで。違反は、
// 都道府県・市区町村・番地以降の順に、すべて列挙する。
func NewAddress(prefectureCode *int, city, streetAddress string) (Address, error) {
	var issues []Message
	var address Address
	if prefectureCode != nil {
		prefecture, err := PrefectureOf(*prefectureCode)
		if err != nil {
			issues = append(issues, Msg(keyPrefectureInvalid))
		}
		address.prefecture = prefecture
	}
	address.city = strings.TrimSpace(city)
	if exceedsChars(address.city, MaxCityChars) {
		issues = append(issues, Msg(keyCityTooLong, MaxCityChars))
	}
	address.streetAddress = strings.TrimSpace(streetAddress)
	if exceedsChars(address.streetAddress, MaxStreetAddressChars) {
		issues = append(issues, Msg(keyStreetTooLong, MaxStreetAddressChars))
	}
	if len(issues) > 0 {
		return Address{}, NewValidationError(issues...)
	}
	return address, nil
}

// Prefecture は都道府県を返す。未設定なら false である。
func (a Address) Prefecture() (Prefecture, bool) { return a.prefecture, a.prefecture.code != 0 }

// City は市区町村である(未設定なら空文字)。
func (a Address) City() string { return a.city }

// StreetAddress は番地以降である(未設定なら空文字)。
func (a Address) StreetAddress() string { return a.streetAddress }

// IsEmpty は、どの項目も未設定かを返す。
func (a Address) IsEmpty() bool { return a == Address{} }
