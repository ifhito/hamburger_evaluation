package domain

import "testing"

func TestIsUUID(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"小文字の正規形は有効", "0b0e3a5c-8d54-4c1a-9f33-2a9d6f1c7e10", true},
		{"すべて 0 の正規形は有効(バージョンは見ない)", "00000000-0000-0000-0000-000000000000", true},
		{"大文字は不正な形式", "0B0E3A5C-8D54-4C1A-9F33-2A9D6F1C7E10", false},
		{"ハイフンなしは不正な形式", "0b0e3a5c8d544c1a9f332a9d6f1c7e10", false},
		{"波括弧つきは不正な形式", "{0b0e3a5c-8d54-4c1a-9f33-2a9d6f1c7e10}", false},
		{"前後の空白は不正な形式", " 0b0e3a5c-8d54-4c1a-9f33-2a9d6f1c7e10", false},
		{"1 文字短いと不正な形式", "0b0e3a5c-8d54-4c1a-9f33-2a9d6f1c7e1", false},
		{"1 文字長いと不正な形式", "0b0e3a5c-8d54-4c1a-9f33-2a9d6f1c7e100", false},
		{"16 進数でない文字は不正な形式", "0b0e3a5c-8d54-4c1a-9f33-2a9d6f1c7e1g", false},
		{"ハイフンの位置がずれると不正な形式", "0b0e3a5c8-d54-4c1a-9f33-2a9d6f1c7e10", false},
		{"整数の id は不正な形式", "1", false},
		{"数値でも uuid でもない文字列は不正な形式", "abc", false},
		{"空文字列は不正な形式", "", false},
		{"全角の 16 進数は不正な形式", "0b0e3a5c-8d54-4c1a-9f33-2a9d6f1c7e１0", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsUUID(tt.in); got != tt.want {
				t.Errorf("IsUUID(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
