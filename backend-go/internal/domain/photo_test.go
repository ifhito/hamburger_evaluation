package domain_test

import (
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// 写真の上限は API の契約(GET /meta とエラーメッセージに現れる)なので、変えたときに気づけるように固定する。
func TestPhotoRuleConstants(t *testing.T) {
	if domain.MaxPhotoBytes != 5*1024*1024 {
		t.Errorf("MaxPhotoBytes = %d, want 5 MiB (5242880)", domain.MaxPhotoBytes)
	}
	if domain.MaxPhotoEdge != 1600 {
		t.Errorf("MaxPhotoEdge = %d, want 1600", domain.MaxPhotoEdge)
	}
}
