package rowmap_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/rowmap"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// TestShopAddress は、DB の行の住所を domain の住所に写すこと、規則に合わない値を黙って捨てずに
// エラーにすることを確かめる。
func TestShopAddress(t *testing.T) {
	t.Run("正しい住所は、そのまま読み取れる", func(t *testing.T) {
		shop, err := rowmap.Shop("id", "n", 1, pgtype.Text{}, pgtype.Text{}, pgtype.Int2{Int16: 13, Valid: true}, "渋谷区", "神南1-2-3", nil, pgtype.Timestamptz{})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if p, ok := shop.Address.Prefecture(); !ok || p.Code() != 13 || shop.Address.City() != "渋谷区" || shop.Address.StreetAddress() != "神南1-2-3" {
			t.Errorf("address = %+v", shop.Address)
		}
	})

	t.Run("NULL の都道府県と空の文字列は、住所なしになる", func(t *testing.T) {
		shop, err := rowmap.Shop("id", "n", 1, pgtype.Text{}, pgtype.Text{}, pgtype.Int2{}, "", "", nil, pgtype.Timestamptz{})
		if err != nil || !shop.Address.IsEmpty() {
			t.Errorf("address = %+v, err = %v, want 住所なし", shop.Address, err)
		}
	})

	t.Run("範囲外の都道府県のコードや上限を超える文字数は、入力の誤り(422)ではないエラーになる", func(t *testing.T) {
		if _, err := rowmap.Shop("id", "n", 1, pgtype.Text{}, pgtype.Text{}, pgtype.Int2{Int16: 48, Valid: true}, "", "", nil, pgtype.Timestamptz{}); err == nil || errors.As(err, new(*domain.ValidationError)) {
			t.Errorf("都道府県のコード 48 で err = %v, want 検証エラーでないエラー", err)
		}
		if _, err := rowmap.Shop("id", "n", 1, pgtype.Text{}, pgtype.Text{}, pgtype.Int2{}, strings.Repeat("区", 101), "", nil, pgtype.Timestamptz{}); err == nil || errors.As(err, new(*domain.ValidationError)) {
			t.Errorf("101 文字の市区町村で err = %v, want 検証エラーでないエラー", err)
		}
	})
}
