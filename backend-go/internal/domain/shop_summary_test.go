package domain_test

import (
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

func TestRoundAverageRating(t *testing.T) {
	for _, tt := range []struct {
		name string
		in   float64
		want float64
	}{
		{"ちょうど整数のときは、そのまま", 4, 4},
		{"小数 2 桁目が 4 以下なら、切り捨てる", 4.24, 4.2},
		{"小数 2 桁目がちょうど 5 なら、切り上げる", 4.25, 4.3},
		{"3 分の 1 のような割り切れない平均は、小数 1 桁に丸める", 13.0 / 3, 4.3},
		{"3 分の 2 のような割り切れない平均は、小数 1 桁に丸める", 14.0 / 3, 4.7},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := domain.RoundAverageRating(tt.in); got != tt.want {
				t.Errorf("RoundAverageRating(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestNewShopSummary(t *testing.T) {
	key := "reviews/a.jpg"
	avg := 13.0 / 3

	t.Run("レビューがあるときは、件数・丸めた平均・写真のキーを持つ", func(t *testing.T) {
		got := domain.NewShopSummary(3, &avg, &key)
		if got.ReviewCount != 3 || got.AverageRating == nil || *got.AverageRating != 4.3 || got.PhotoKey == nil || *got.PhotoKey != key {
			t.Errorf("summary = %+v, want count 3, average 4.3, photo %q", got, key)
		}
		if got.PhotoURL != nil {
			t.Errorf("PhotoURL = %v, want nil(URL は usecase が写真の保存先を介して導く)", *got.PhotoURL)
		}
	})

	t.Run("レビューがないときは、平均も写真も nil になる", func(t *testing.T) {
		got := domain.NewShopSummary(0, nil, nil)
		if got.ReviewCount != 0 || got.AverageRating != nil || got.PhotoKey != nil {
			t.Errorf("summary = %+v, want empty", got)
		}
	})

	t.Run("写真つきのレビューがないときは、平均だけがあり、写真は nil になる", func(t *testing.T) {
		got := domain.NewShopSummary(2, &avg, nil)
		if got.AverageRating == nil || got.PhotoKey != nil {
			t.Errorf("summary = %+v, want average and no photo", got)
		}
	})
}
