package domain_test

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
)

// TestValidateReviewContent は Rails parity の validation メッセージを固定する。
// 1..5 の範囲外の rating と空の comment は full message
// そのままで失敗し、両方が失敗する場合は rating のメッセージが先に来る。
func TestValidateReviewContent(t *testing.T) {
	tests := []struct {
		name    string
		rating  int
		comment string
		want    []string // nil = 有効
	}{
		{name: "有効な入力は検証を通る", rating: 4, comment: "Tasty"},
		{name: "境界値の rating 1 は有効", rating: 1, comment: "ok"},
		{name: "rating 5 は有効", rating: 5, comment: "ok"},
		{name: "rating 0 は検証エラーになる", rating: 0, comment: "ok", want: []string{"Rating must be in 1..5"}},
		{name: "rating 6 は検証エラーになる", rating: 6, comment: "ok", want: []string{"Rating must be in 1..5"}},
		{name: "負の rating は検証エラーになる", rating: -1, comment: "ok", want: []string{"Rating must be in 1..5"}},
		{name: "空の comment は検証エラーになる", rating: 3, comment: "", want: []string{"Comment can't be blank"}},
		{name: "空白のみの comment は検証エラーになる", rating: 3, comment: " \t\n", want: []string{"Comment can't be blank"}},
		{
			name: "両方が不正なら rating のメッセージが先に来る", rating: 0, comment: "",
			want: []string{"Rating must be in 1..5", "Comment can't be blank"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := domain.ValidateReviewContent(tt.rating, tt.comment)
			if tt.want == nil {
				if err != nil {
					t.Fatalf("ValidateReviewContent returned error: %v", err)
				}
				return
			}
			var vErr *domain.ValidationError
			if !errors.As(err, &vErr) {
				t.Fatalf("error = %v, want *domain.ValidationError", err)
			}
			if !reflect.DeepEqual(vErr.Texts(domain.LangEN), tt.want) {
				t.Errorf("messages = %v, want %v", vErr.Texts(domain.LangEN), tt.want)
			}
		})
	}
}

// TestRatingRange は、rating の範囲の定数(GET /meta で frontend に返す値)が、検証の境界と一致し、
// メッセージにも使われることを固定する。
func TestRatingRange(t *testing.T) {
	if domain.MinRating > domain.MaxRating {
		t.Fatalf("MinRating %d > MaxRating %d", domain.MinRating, domain.MaxRating)
	}
	for _, r := range []int{domain.MinRating, domain.MaxRating} {
		if err := domain.ValidateReviewContent(r, "ok"); err != nil {
			t.Errorf("rating %d(範囲の端)は有効なはず: %v", r, err)
		}
	}
	for _, r := range []int{domain.MinRating - 1, domain.MaxRating + 1} {
		var vErr *domain.ValidationError
		err := domain.ValidateReviewContent(r, "ok")
		if !errors.As(err, &vErr) {
			t.Fatalf("rating %d(範囲の外)は検証エラーのはず: %v", r, err)
		}
		want := fmt.Sprintf("Rating must be in %d..%d", domain.MinRating, domain.MaxRating)
		if len(vErr.Texts(domain.LangEN)) != 1 || vErr.Texts(domain.LangEN)[0] != want {
			t.Errorf("messages = %v, want [%s]", vErr.Texts(domain.LangEN), want)
		}
	}
}

// TestNewReview はコンストラクタを固定する。有効な入力は、comment をそのまま
// 保持し（決して trim しない）、author/burger を記録した review を返す。
// 無効な入力は review を返さずに ValidationError を表に出す。
func TestNewReview(t *testing.T) {
	t.Run("有効な入力から review を作る", func(t *testing.T) {
		review, err := domain.NewReview(4, " Tasty ", uid.N(7), uid.N(9))
		if err != nil {
			t.Fatalf("NewReview returned error: %v", err)
		}
		if review.Rating != 4 || review.AuthorID != uid.N(7) || review.BurgerID != uid.N(9) {
			t.Errorf("review = %+v, want rating 4, author 7, burger 9", review)
		}
		if review.Comment == nil || *review.Comment != " Tasty " {
			t.Errorf("comment = %v, want verbatim \" Tasty \"", review.Comment)
		}
	})

	t.Run("無効な入力は検証エラーになる", func(t *testing.T) {
		_, err := domain.NewReview(0, "", uid.N(7), uid.N(9))
		var vErr *domain.ValidationError
		if !errors.As(err, &vErr) {
			t.Fatalf("error = %v, want *domain.ValidationError", err)
		}
	})
}

// TestReviewCanBeModifiedBy は所有権ルールを固定する。
// author だけが編集または削除でき、admin にも例外はない。
func TestReviewCanBeModifiedBy(t *testing.T) {
	review := domain.Review{ID: uid.N(1), AuthorID: uid.N(7)}

	tests := []struct {
		name   string
		viewer domain.User
		want   bool
	}{
		{name: "author は変更できる", viewer: domain.User{ID: uid.N(7)}, want: true},
		{name: "他のユーザーは変更できない", viewer: domain.User{ID: uid.N(8)}, want: false},
		{name: "admin でも特別扱いされず変更できない", viewer: domain.User{ID: uid.N(9), Admin: true}, want: false},
		{name: "author である admin は変更できる", viewer: domain.User{ID: uid.N(7), Admin: true}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := review.CanBeModifiedBy(tt.viewer); got != tt.want {
				t.Errorf("CanBeModifiedBy(%+v) = %v, want %v", tt.viewer, got, tt.want)
			}
		})
	}
}

// TestReviewCanBeModifiedByViewer は、API の can_edit の元になる値を固定する。
// 匿名（nil）は常に false で、それ以外は CanBeModifiedBy の規則（author だけ。admin にも
// 例外なし）に従う。
func TestReviewCanBeModifiedByViewer(t *testing.T) {
	review := domain.Review{ID: uid.N(1), AuthorID: uid.N(7)}
	author := domain.User{ID: uid.N(7)}
	other := domain.User{ID: uid.N(8)}
	admin := domain.User{ID: uid.N(9), Admin: true}

	tests := []struct {
		name   string
		viewer *domain.User
		want   bool
	}{
		{name: "匿名は変更できない", viewer: nil, want: false},
		{name: "author は変更できる", viewer: &author, want: true},
		{name: "他のユーザーは変更できない", viewer: &other, want: false},
		{name: "admin でも他人の review は変更できない", viewer: &admin, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := review.CanBeModifiedByViewer(tt.viewer); got != tt.want {
				t.Errorf("CanBeModifiedByViewer(%+v) = %v, want %v", tt.viewer, got, tt.want)
			}
		})
	}
}
