package domain_test

import (
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

func TestPageBounds(t *testing.T) {
	tests := []struct {
		name          string
		page, perPage int
		limit, offset int32
	}{
		{"指定なし(0, 0)は、既定の件数で 1 ページ目", 0, 0, domain.DefaultPerPage, 0},
		{"件数 1 は、そのまま", 1, 1, 1, 0},
		{"上限ちょうどの件数は、そのまま", 1, domain.MaxPerPage, domain.MaxPerPage, 0},
		{"上限 + 1 の件数は、上限に丸められる", 1, domain.MaxPerPage + 1, domain.MaxPerPage, 0},
		{"上限をはるかに超える件数も、上限に丸められる", 1, 1_000_000, domain.MaxPerPage, 0},
		{"件数が負のときは、既定の件数", 1, -1, domain.DefaultPerPage, 0},
		{"件数が int の最小値のときも、既定の件数", 1, math.MinInt, domain.DefaultPerPage, 0},
		{"件数が int の最大値のときは、上限に丸められる", 1, math.MaxInt, domain.MaxPerPage, 0},
		{"ページ 0 は、1 ページ目", 0, 10, 10, 0},
		{"ページが負のときは、1 ページ目", -7, 10, 10, 0},
		{"ページが int の最小値のときも、1 ページ目", math.MinInt, 10, 10, 0},
		{"2 ページ目は、1 ページ分を飛ばす", 2, 10, 10, 10},
		{"3 ページ目、既定の件数", 3, 0, domain.DefaultPerPage, 2 * domain.DefaultPerPage},
		{"上限の件数で 5 ページ目", 5, domain.MaxPerPage, domain.MaxPerPage, 4 * domain.MaxPerPage},
		{"ページが int32 の最大値のとき、飛ばす件数は int32 に収まる", math.MaxInt32, 100, 100, math.MaxInt32},
		{"ページが int32 の最大値を超えても、飛ばす件数は int32 に収まる", math.MaxInt32 + 1, 100, 100, math.MaxInt32},
		{"ページが int の最大値のときも、桁あふれせず、飛ばす件数は int32 の最大値", math.MaxInt, 100, 100, math.MaxInt32},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limit, offset := domain.PageBounds(tt.page, tt.perPage)
			if limit != tt.limit || offset != tt.offset {
				t.Errorf("PageBounds(%d, %d) = (%d, %d), want (%d, %d)", tt.page, tt.perPage, limit, offset, tt.limit, tt.offset)
			}
		})
	}
}

func TestParsePageParams(t *testing.T) {
	const (
		pageMsg    = "Page must be an integer"
		perPageMsg = "Per page must be an integer"
	)
	tests := []struct {
		name            string
		rawPage, rawPer string
		page, perPage   int
		wantMessages    []string
	}{
		{"両方とも省略(空文字)は、0(指定なし)で、エラーではない", "", "", 0, 0, nil},
		{"整数は、そのまま整数になる", "3", "50", 3, 50, nil},
		{"0 は 0 のまま(丸めは PageBounds が行う)", "0", "0", 0, 0, nil},
		{"負の整数も、構文としては整数", "-2", "-5", -2, -5, nil},
		{"先頭の + も、整数の構文", "+4", "+10", 4, 10, nil},
		{"先頭に 0 が並んでいても、整数", "007", "010", 7, 10, nil},
		{"int の範囲を超える整数は、エラーではなく、範囲内の最大値に丸められる", "99999999999999999999", "99999999999999999999", math.MaxInt, math.MaxInt, nil},
		{"int の範囲を下回る整数は、範囲内の最小値に丸められる", "-99999999999999999999", "-99999999999999999999", math.MinInt, math.MinInt, nil},
		{"page が数字でなければ、page のメッセージ", "abc", "10", 0, 0, []string{pageMsg}},
		{"per_page が数字でなければ、per_page のメッセージ", "1", "x", 0, 0, []string{perPageMsg}},
		{"両方が数字でなければ、page、per_page の順に両方のメッセージ", "a", "b", 0, 0, []string{pageMsg, perPageMsg}},
		{"小数は整数ではない", "1.5", "", 0, 0, []string{pageMsg}},
		{"指数表記は整数ではない", "1e3", "", 0, 0, []string{pageMsg}},
		{"16 進表記は整数ではない", "0x10", "", 0, 0, []string{pageMsg}},
		{"符号だけは整数ではない", "-", "+", 0, 0, []string{pageMsg, perPageMsg}},
		{"前後の空白は整数ではない", " 1", "1 ", 0, 0, []string{pageMsg, perPageMsg}},
		{"桁区切りは整数ではない", "1,000", "", 0, 0, []string{pageMsg}},
		{"全角の数字は整数ではない", "１", "", 0, 0, []string{pageMsg}},
		{"符号が 2 つは整数ではない", "--1", "", 0, 0, []string{pageMsg}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, perPage, err := domain.ParsePageParams(tt.rawPage, tt.rawPer)
			if tt.wantMessages == nil {
				if err != nil || page != tt.page || perPage != tt.perPage {
					t.Fatalf("ParsePageParams(%q, %q) = (%d, %d, %v), want (%d, %d, nil)", tt.rawPage, tt.rawPer, page, perPage, err, tt.page, tt.perPage)
				}
				return
			}
			var vErr *domain.ValidationError
			if !errors.As(err, &vErr) || !reflect.DeepEqual(vErr.Messages, tt.wantMessages) {
				t.Fatalf("ParsePageParams(%q, %q) error = %v, want a ValidationError with %v", tt.rawPage, tt.rawPer, err, tt.wantMessages)
			}
			if page != 0 || perPage != 0 {
				t.Errorf("エラーのとき、値は 0 のはず: (%d, %d)", page, perPage)
			}
		})
	}
}

func TestPageFetchLimitAndTrimPage(t *testing.T) {
	t.Run("次のページがあるかを知るために、1 ページの件数より 1 件多く取り出す", func(t *testing.T) {
		for _, limit := range []int32{1, domain.DefaultPerPage, domain.MaxPerPage} {
			if got := domain.PageFetchLimit(limit); got != limit+1 {
				t.Errorf("PageFetchLimit(%d) = %d, want %d", limit, got, limit+1)
			}
		}
	})

	rows := func(n int) []int {
		out := make([]int, n)
		for i := range out {
			out[i] = i
		}
		return out
	}
	tests := []struct {
		name        string
		rows        []int
		limit       int32
		want        []int
		wantHasMore bool
	}{
		{"件数が limit より 1 件多いときは、limit 件に切り詰め、続きがある", rows(4), 3, rows(3), true},
		{"件数がちょうど limit のときは、切り詰めず、続きはない", rows(3), 3, rows(3), false},
		{"件数が limit より少ないときは、そのまま、続きはない", rows(2), 3, rows(2), false},
		{"0 件のときは、そのまま(空)、続きはない", rows(0), 3, rows(0), false},
		{"nil のときも、続きはない", nil, 3, nil, false},
		{"limit が 1 で 2 件のとき", rows(2), 1, rows(1), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, hasMore := domain.TrimPage(tt.rows, tt.limit)
			if hasMore != tt.wantHasMore || len(got) != len(tt.want) || (len(got) > 0 && !reflect.DeepEqual(got, tt.want)) {
				t.Errorf("TrimPage(%v, %d) = (%v, %v), want (%v, %v)", tt.rows, tt.limit, got, hasMore, tt.want, tt.wantHasMore)
			}
		})
	}

	t.Run("PageBounds の結果の limit で取り出した件数(limit+1)を切り詰めると、いつも limit 件以下になる", func(t *testing.T) {
		for _, perPage := range []int{-1, 0, 1, 20, 100, 101, 5000} {
			limit, _ := domain.PageBounds(1, perPage)
			got, hasMore := domain.TrimPage(rows(int(domain.PageFetchLimit(limit))), limit)
			if int32(len(got)) != limit || !hasMore {
				t.Errorf("perPage=%d: %d 件、hasMore=%v, want %d 件と true", perPage, len(got), hasMore, limit)
			}
		}
	})
}

// 既定値と上限は、API の契約(既存の一覧の挙動)なので、変えたら気づけるように固定する。
func TestPageRuleConstants(t *testing.T) {
	if domain.DefaultPerPage != 20 || domain.MaxPerPage != 100 {
		t.Errorf("DefaultPerPage = %d, MaxPerPage = %d, want 20 と 100", domain.DefaultPerPage, domain.MaxPerPage)
	}
}
