package domain_test

import (
	"math"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

func TestNormalizePage(t *testing.T) {
	tests := []struct {
		name                  string
		page, perPage         int
		wantPage, wantPerPage int
	}{
		{"指定なし(0, 0)は、1 ページ目で既定の件数", 0, 0, 1, domain.DefaultPerPage},
		{"件数 1 は、そのまま", 1, 1, 1, 1},
		{"上限ちょうどの件数は、そのまま", 1, domain.MaxPerPage, 1, domain.MaxPerPage},
		{"上限 + 1 の件数は、上限に丸められる", 1, domain.MaxPerPage + 1, 1, domain.MaxPerPage},
		{"上限をはるかに超える件数も、上限に丸められる", 1, 1_000_000, 1, domain.MaxPerPage},
		{"件数が負のときは、既定の件数", 1, -1, 1, domain.DefaultPerPage},
		{"件数が int の最小値のときも、既定の件数", 1, math.MinInt, 1, domain.DefaultPerPage},
		{"件数が int の最大値のときは、上限に丸められる", 1, math.MaxInt, 1, domain.MaxPerPage},
		{"ページ 0 は、1 ページ目", 0, 10, 1, 10},
		{"ページが負のときは、1 ページ目", -7, 10, 1, 10},
		{"ページが int の最小値のときも、1 ページ目", math.MinInt, 10, 1, 10},
		{"範囲内のページと件数は、そのまま", 3, 10, 3, 10},
		{"件数が既定のときの 3 ページ目", 3, 0, 3, domain.DefaultPerPage},
		{"ページには上限がない(遠く離れたページは、そのまま)", math.MaxInt, 100, math.MaxInt, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, perPage := domain.NormalizePage(tt.page, tt.perPage)
			if page != tt.wantPage || perPage != tt.wantPerPage {
				t.Errorf("NormalizePage(%d, %d) = (%d, %d), want (%d, %d)", tt.page, tt.perPage, page, perPage, tt.wantPage, tt.wantPerPage)
			}
		})
	}
}

// 既定値と上限は、API の契約(既存の一覧の挙動)なので、変えたら気づけるように固定する。
func TestPageRuleConstants(t *testing.T) {
	if domain.DefaultPerPage != 20 || domain.MaxPerPage != 100 {
		t.Errorf("DefaultPerPage = %d, MaxPerPage = %d, want 20 と 100", domain.DefaultPerPage, domain.MaxPerPage)
	}
}
