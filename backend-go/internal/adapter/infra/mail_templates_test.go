package infra

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

func TestRenderSignupConfirmation(t *testing.T) {
	msg := renderSignupConfirmation(usecase.SignupConfirmation{
		To:             "a@example.com",
		ConfirmURL:     "https://app.example.com/signup/confirm?token=abc123",
		ValidFor:       domain.SignupTokenTTL,
		IdempotencyKey: "k",
	})
	if msg.To != "a@example.com" || msg.Subject != "Confirm your email address" {
		t.Errorf("宛先・件名 = %q / %q", msg.To, msg.Subject)
	}
	for _, want := range []string{
		"https://app.example.com/signup/confirm?token=abc123",
		"This link expires in 24 hours.",
		"If you didn't sign up, you can safely ignore this email.",
	} {
		if !strings.Contains(msg.Body, want) {
			t.Errorf("本文に %q がない:\n%s", want, msg.Body)
		}
	}
	if strings.Contains(msg.Body, "k\n") && strings.Contains(msg.Body, "IdempotencyKey") {
		t.Errorf("本文に冪等キーが含まれている:\n%s", msg.Body)
	}
}

func TestRenderAlreadyRegistered(t *testing.T) {
	msg := renderAlreadyRegistered(usecase.AlreadyRegisteredNotice{To: "a@example.com", SignInURL: "https://app.example.com/signin", IdempotencyKey: "k"})
	if msg.To != "a@example.com" || msg.Subject != "You already have an account" {
		t.Errorf("宛先・件名 = %q / %q", msg.To, msg.Subject)
	}
	if !strings.Contains(msg.Body, "https://app.example.com/signin") || !strings.Contains(msg.Body, "an account already exists") {
		t.Errorf("本文にログイン画面へのリンクと登録済みの説明がない:\n%s", msg.Body)
	}
	if lower := strings.ToLower(msg.Body); strings.Contains(lower, "token") || strings.Contains(lower, "confirm") {
		t.Errorf("通知メールに確認トークン・確認リンクの言葉が含まれている:\n%s", msg.Body)
	}
}

func TestHumanizeDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{24 * time.Hour, "24 hours"},
		{time.Hour, "1 hour"},
		{90 * time.Minute, "90 minutes"},
		{time.Minute, "1 minute"},
		{30 * time.Second, "1 minute"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := humanizeDuration(tt.d); got != tt.want {
				t.Errorf("humanizeDuration(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}

// TestClassifyDeliveryError は、送信の実装のエラーが、domain の言葉（一時的・恒久的な失敗）に翻訳されることを確かめる。
func TestClassifyDeliveryError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want domain.MailFailure
	}{
		{"550(宛先の拒否)は恒久的", fmt.Errorf("smtp: rcpt to: %w", &textproto.Error{Code: 550, Msg: "no such user"}), domain.MailFailurePermanent},
		{"535(認証の失敗)は恒久的", fmt.Errorf("smtp: auth: %w", &textproto.Error{Code: 535, Msg: "bad credentials"}), domain.MailFailurePermanent},
		{"451(一時的な拒否)は一時的", fmt.Errorf("smtp: mail from: %w", &textproto.Error{Code: 451, Msg: "try again later"}), domain.MailFailureTemporary},
		{"421(サービスの一時停止)は一時的", &textproto.Error{Code: 421, Msg: "service not available"}, domain.MailFailureTemporary},
		{"証明書の検証の失敗は恒久的", fmt.Errorf("smtp: connect: %w", &tls.CertificateVerificationError{Err: errors.New("x509: unknown authority")}), domain.MailFailurePermanent},
		{"送る前に拒否した不正なヘッダーは恒久的", errInvalidMessage, domain.MailFailurePermanent},
		{"STARTTLS 非対応のサーバーは恒久的", fmt.Errorf("wrapped: %w", errSTARTTLSUnsupported), domain.MailFailurePermanent},
		{"AUTH 非対応のサーバーは恒久的", errAuthUnsupported, domain.MailFailurePermanent},
		{"接続できないのは一時的", fmt.Errorf("smtp: connect: %w", &net.OpError{Op: "dial", Err: errors.New("connection refused")}), domain.MailFailureTemporary},
		{"タイムアウトは一時的", fmt.Errorf("smtp: greeting: %w", errors.New("i/o timeout")), domain.MailFailureTemporary},
		{"未知のエラーは一時的", errors.New("something odd"), domain.MailFailureTemporary},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyDeliveryError(tt.err); got != tt.want {
				t.Errorf("classifyDeliveryError = %q, want %q", got, tt.want)
			}
		})
	}
}
