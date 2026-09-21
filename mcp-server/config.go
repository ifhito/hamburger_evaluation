package main

import (
	"errors"
	"net/url"
	"strings"
	"time"
)

const (
	envAPIURL     = "HAMBURGER_API_URL"
	envAPIToken   = "HAMBURGER_API_TOKEN"
	envAllowWrite = "HAMBURGER_MCP_ALLOW_WRITE"

	defaultAPIURL           = "http://localhost:8080"
	defaultTimeout          = 15 * time.Second
	defaultMaxResponseBytes = 64 << 10
)

// config は環境変数から決まる設定である。timeout と maxResponseBytes は
// 環境変数にはせず、テストからだけ差し替える。
type config struct {
	apiURL           string
	token            string
	allowWrite       bool
	timeout          time.Duration
	maxResponseBytes int
}

// loadConfig は getenv から設定を読む。書き込みの tool は、値がちょうど "true"
// のときだけ有効にする（"1" や "yes" では有効にしない）。
func loadConfig(getenv func(string) string) (config, error) {
	cfg := config{
		apiURL:           defaultAPIURL,
		token:            strings.TrimSpace(getenv(envAPIToken)),
		allowWrite:       strings.EqualFold(strings.TrimSpace(getenv(envAllowWrite)), "true"),
		timeout:          defaultTimeout,
		maxResponseBytes: defaultMaxResponseBytes,
	}
	if raw := strings.TrimSpace(getenv(envAPIURL)); raw != "" {
		cfg.apiURL = raw
	}
	u, err := url.Parse(cfg.apiURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return config{}, errors.New(envAPIURL + " must be an http(s) URL such as http://localhost:8080")
	}
	if u.User != nil {
		return config{}, errors.New(envAPIURL + " must not contain credentials; use " + envAPIToken)
	}
	cfg.apiURL = strings.TrimRight(cfg.apiURL, "/")
	return cfg, nil
}
