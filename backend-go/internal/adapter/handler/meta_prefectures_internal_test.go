package handler

import (
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// TestPrefectureNamesENMatchDomain は、英語の名前の表(prefectureNamesEN)が、domain の都道府県の表と
// 同じ件数で、domain のどのコードにも英語の名前があることを確かめる(食い違うと GET /meta が壊れる)。
func TestPrefectureNamesENMatchDomain(t *testing.T) {
	prefectures := domain.Prefectures()
	if len(prefectureNamesEN) != len(prefectures) {
		t.Fatalf("英語の名前 = %d 件, domain = %d 件", len(prefectureNamesEN), len(prefectures))
	}
	for _, p := range prefectures {
		if i := p.Code() - 1; i < 0 || i >= len(prefectureNamesEN) || prefectureNamesEN[i] == "" {
			t.Errorf("コード %d(%s)の英語の名前がない", p.Code(), p.Name())
		}
	}
}
