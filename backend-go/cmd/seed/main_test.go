package main

import (
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// TestDevPasswordSatisfiesPolicy は、seed のパスワードが signup と同じ強度ルールを
// 満たすことを固定する。規則が変わったとき、開発用 fixture だけが現行の規則で
// 作れない値のまま残るのを防ぐ。
func TestDevPasswordSatisfiesPolicy(t *testing.T) {
	if msgs := domain.ValidatePassword(devPassword); len(msgs) != 0 {
		t.Fatalf("devPassword violates the password policy: %q", msgs)
	}
}
