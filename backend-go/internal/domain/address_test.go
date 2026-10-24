package domain_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// TestNewAddress は住所の組み立てを固定する: どの項目も任意で、都道府県のコードは 1〜47、市区町村と
// 番地以降は上限の文字数(日本語も 1 文字)まで。違反はすべて列挙し、前後の空白は取り除く。
func TestNewAddress(t *testing.T) {
	t.Run("すべて未設定なら、住所なしとして有効", func(t *testing.T) {
		got, err := domain.NewAddress(nil, "", "")
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
		if _, ok := got.Prefecture(); ok || !got.IsEmpty() {
			t.Errorf("got = %+v, want 住所なし", got)
		}
		if !(domain.Address{}).IsEmpty() {
			t.Error("ゼロ値の住所が IsEmpty でない")
		}
	})

	t.Run("空白だけの市区町村と番地以降は未設定になる", func(t *testing.T) {
		got, err := domain.NewAddress(nil, "  ", "\t")
		if err != nil || !got.IsEmpty() {
			t.Errorf("got = %+v, err = %v, want 住所なし", got, err)
		}
	})

	t.Run("上限ちょうどの住所は有効で、前後の空白は取り除かれる", func(t *testing.T) {
		got, err := domain.NewAddress(ptr(47),
			" "+strings.Repeat("区", domain.MaxCityChars)+" ",
			" "+strings.Repeat("丁", domain.MaxStreetAddressChars)+"\n")
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
		if p, ok := got.Prefecture(); !ok || p.Code() != 47 || p.Name() != "沖縄県" {
			t.Errorf("Prefecture = %+v, %v, want 47 沖縄県", p, ok)
		}
		if got.City() != strings.Repeat("区", domain.MaxCityChars) || got.StreetAddress() != strings.Repeat("丁", domain.MaxStreetAddressChars) {
			t.Errorf("got = %q・%q, want 空白を除いた値", got.City(), got.StreetAddress())
		}
		if got.IsEmpty() {
			t.Error("IsEmpty = true, want false")
		}
	})

	for _, n := range []int{0, 48, -1} {
		t.Run(fmt.Sprintf("都道府県のコード %d は不正", n), func(t *testing.T) {
			_, err := domain.NewAddress(ptr(n), "", "")
			assertTexts(t, err, "Prefecture is invalid")
		})
	}

	t.Run("市区町村が上限を超えると不正", func(t *testing.T) {
		_, err := domain.NewAddress(nil, strings.Repeat("区", domain.MaxCityChars+1), "")
		assertTexts(t, err, "City is too long (maximum is 100 characters)")
	})

	t.Run("番地以降が上限を超えると不正", func(t *testing.T) {
		_, err := domain.NewAddress(nil, "", strings.Repeat("丁", domain.MaxStreetAddressChars+1))
		assertTexts(t, err, "Street address is too long (maximum is 200 characters)")
	})

	t.Run("すべての違反を、都道府県・市区町村・番地以降の順に列挙する", func(t *testing.T) {
		_, err := domain.NewAddress(ptr(48),
			strings.Repeat("a", domain.MaxCityChars+1),
			strings.Repeat("a", domain.MaxStreetAddressChars+1))
		assertTexts(t, err, "Prefecture is invalid", "City is too long (maximum is 100 characters)", "Street address is too long (maximum is 200 characters)")
	})
}

// assertTexts は、err が *ValidationError で、英語の文言が want と順番どおりに一致することを確かめる。
func assertTexts(t *testing.T, err error, want ...string) {
	t.Helper()
	var vErr *domain.ValidationError
	if !errors.As(err, &vErr) {
		t.Fatalf("err = %v, want *ValidationError", err)
	}
	if got := vErr.Texts(domain.LangEN); !slices.Equal(got, want) {
		t.Errorf("texts = %v, want %v", got, want)
	}
}

// TestPrefectureOf は、都道府県をコードから作れるのが 1〜47 だけであることを固定する。
func TestPrefectureOf(t *testing.T) {
	for code, name := range map[int]string{1: "北海道", 47: "沖縄県"} {
		t.Run(fmt.Sprintf("コード %d は %s", code, name), func(t *testing.T) {
			p, err := domain.PrefectureOf(code)
			if err != nil || p.Code() != code || p.Name() != name {
				t.Errorf("PrefectureOf(%d) = %d %q, %v", code, p.Code(), p.Name(), err)
			}
		})
	}
	for _, code := range []int{0, 48} {
		t.Run(fmt.Sprintf("コード %d は不正", code), func(t *testing.T) {
			_, err := domain.PrefectureOf(code)
			assertTexts(t, err, "Prefecture is invalid")
		})
	}
}

// TestPrefectures は、都道府県の表が 47 件で、コード 1〜47 の昇順であり、呼び出し側が書き換えても
// 表が変わらないことを固定する。
func TestPrefectures(t *testing.T) {
	got := domain.Prefectures()
	if len(got) != 47 {
		t.Fatalf("len = %d, want 47", len(got))
	}
	for i, p := range got {
		if p.Code() != i+1 || p.Name() == "" {
			t.Errorf("[%d] = %d %q, want code %d と名前", i, p.Code(), p.Name(), i+1)
		}
	}
	if got[12].Name() != "東京都" || got[26].Name() != "大阪府" {
		t.Errorf("13 = %q, 27 = %q", got[12].Name(), got[26].Name())
	}
	got[0] = got[46]
	if again := domain.Prefectures(); again[0].Code() != 1 || again[0].Name() != "北海道" {
		t.Errorf("書き換えたあとの表の先頭 = %d %q, want 1 北海道", again[0].Code(), again[0].Name())
	}
}
