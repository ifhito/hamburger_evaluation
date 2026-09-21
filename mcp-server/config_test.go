package main

import (
	"strings"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name       string
		env        map[string]string
		wantURL    string
		wantToken  string
		wantWrite  bool
		wantErrSub string
	}{
		{name: "何も設定しなければ既定値で、書き込みは無効", env: map[string]string{}, wantURL: "http://localhost:8080"},
		{name: "URL の末尾のスラッシュは取り除く", env: map[string]string{"HAMBURGER_API_URL": "https://api.example.com/"}, wantURL: "https://api.example.com"},
		{name: "トークンの前後の空白と改行は取り除く", env: map[string]string{"HAMBURGER_API_TOKEN": " abc\n"}, wantURL: "http://localhost:8080", wantToken: "abc"},
		{name: "true なら書き込みが有効", env: map[string]string{"HAMBURGER_MCP_ALLOW_WRITE": "true"}, wantURL: "http://localhost:8080", wantWrite: true},
		{name: "TRUE でも書き込みが有効", env: map[string]string{"HAMBURGER_MCP_ALLOW_WRITE": "TRUE"}, wantURL: "http://localhost:8080", wantWrite: true},
		{name: "1 では書き込みは有効にならない", env: map[string]string{"HAMBURGER_MCP_ALLOW_WRITE": "1"}, wantURL: "http://localhost:8080"},
		{name: "yes では書き込みは有効にならない", env: map[string]string{"HAMBURGER_MCP_ALLOW_WRITE": "yes"}, wantURL: "http://localhost:8080"},
		{name: "http(s) 以外の URL は起動時に拒否する", env: map[string]string{"HAMBURGER_API_URL": "ftp://example.com"}, wantErrSub: "http(s)"},
		{name: "ホストのない URL は拒否する", env: map[string]string{"HAMBURGER_API_URL": "localhost:8080"}, wantErrSub: "http(s)"},
		{name: "認証情報を含む URL は拒否する", env: map[string]string{"HAMBURGER_API_URL": "http://user:pass@localhost:8080"}, wantErrSub: "credentials"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := loadConfig(func(k string) string { return tt.env[k] })
			if tt.wantErrSub != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrSub) {
					t.Fatalf("エラーが %q を含むはず: %v", tt.wantErrSub, err)
				}
				if strings.Contains(err.Error(), "user:pass") {
					t.Fatalf("エラーに URL の認証情報を出してはいけない: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("loadConfig: %v", err)
			}
			if cfg.apiURL != tt.wantURL || cfg.token != tt.wantToken || cfg.allowWrite != tt.wantWrite {
				t.Fatalf("got url=%q token=%q write=%t, want url=%q token=%q write=%t",
					cfg.apiURL, cfg.token, cfg.allowWrite, tt.wantURL, tt.wantToken, tt.wantWrite)
			}
		})
	}
}
