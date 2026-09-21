package domain_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
)

// TestValidateReviewContent は Rails parity の validation メッセージを固定する
// （issue #14 AC4）。1..5 の範囲外の rating と空の comment は full message
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
			if !reflect.DeepEqual(vErr.Messages, tt.want) {
				t.Errorf("messages = %v, want %v", vErr.Messages, tt.want)
			}
		})
	}
}

// TestNewReview はコンストラクタを固定する。有効な入力は、comment をそのまま
// 保持し（決して trim しない）、author/burger を記録した review を返す。
// 無効な入力は review を返さずに ValidationError を表に出す。
func TestNewReview(t *testing.T) {
	t.Run("有効な入力から review を作る", func(t *testing.T) {
		review, err := domain.NewReview(4, " Tasty ", uid.N(7), 9)
		if err != nil {
			t.Fatalf("NewReview returned error: %v", err)
		}
		if review.Rating != 4 || review.AuthorID != uid.N(7) || review.BurgerID != 9 {
			t.Errorf("review = %+v, want rating 4, author 7, burger 9", review)
		}
		if review.Comment == nil || *review.Comment != " Tasty " {
			t.Errorf("comment = %v, want verbatim \" Tasty \"", review.Comment)
		}
	})

	t.Run("無効な入力は検証エラーになる", func(t *testing.T) {
		_, err := domain.NewReview(0, "", uid.N(7), 9)
		var vErr *domain.ValidationError
		if !errors.As(err, &vErr) {
			t.Fatalf("error = %v, want *domain.ValidationError", err)
		}
	})
}

// TestReviewCanBeModifiedBy は所有権ルールを固定する（issue #14 AC3）。
// author だけが編集または削除でき、admin にも例外はない。
func TestReviewCanBeModifiedBy(t *testing.T) {
	review := domain.Review{ID: 1, AuthorID: uid.N(7)}

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
