package domain_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
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
		{name: "valid", rating: 4, comment: "Tasty"},
		{name: "boundary ratings 1 and 5 are valid", rating: 1, comment: "ok"},
		{name: "rating 5 is valid", rating: 5, comment: "ok"},
		{name: "rating 0 fails", rating: 0, comment: "ok", want: []string{"Rating must be in 1..5"}},
		{name: "rating 6 fails", rating: 6, comment: "ok", want: []string{"Rating must be in 1..5"}},
		{name: "negative rating fails", rating: -1, comment: "ok", want: []string{"Rating must be in 1..5"}},
		{name: "empty comment fails", rating: 3, comment: "", want: []string{"Comment can't be blank"}},
		{name: "whitespace-only comment fails", rating: 3, comment: " \t\n", want: []string{"Comment can't be blank"}},
		{
			name: "both fail with rating message first", rating: 0, comment: "",
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
	t.Run("valid input builds the review", func(t *testing.T) {
		review, err := domain.NewReview(4, " Tasty ", 7, 9)
		if err != nil {
			t.Fatalf("NewReview returned error: %v", err)
		}
		if review.Rating != 4 || review.AuthorID != 7 || review.BurgerID != 9 {
			t.Errorf("review = %+v, want rating 4, author 7, burger 9", review)
		}
		if review.Comment == nil || *review.Comment != " Tasty " {
			t.Errorf("comment = %v, want verbatim \" Tasty \"", review.Comment)
		}
	})

	t.Run("invalid input fails validation", func(t *testing.T) {
		_, err := domain.NewReview(0, "", 7, 9)
		var vErr *domain.ValidationError
		if !errors.As(err, &vErr) {
			t.Fatalf("error = %v, want *domain.ValidationError", err)
		}
	})
}

// TestReviewCanBeModifiedBy は所有権ルールを固定する（issue #14 AC3）。
// author だけが編集または削除でき、admin にも例外はない。
func TestReviewCanBeModifiedBy(t *testing.T) {
	review := domain.Review{ID: 1, AuthorID: 7}

	tests := []struct {
		name   string
		viewer domain.User
		want   bool
	}{
		{name: "author may modify", viewer: domain.User{ID: 7}, want: true},
		{name: "other user may not", viewer: domain.User{ID: 8}, want: false},
		{name: "admin gets no pass", viewer: domain.User{ID: 9, Admin: true}, want: false},
		{name: "admin author may modify", viewer: domain.User{ID: 7, Admin: true}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := review.CanBeModifiedBy(tt.viewer); got != tt.want {
				t.Errorf("CanBeModifiedBy(%+v) = %v, want %v", tt.viewer, got, tt.want)
			}
		})
	}
}
