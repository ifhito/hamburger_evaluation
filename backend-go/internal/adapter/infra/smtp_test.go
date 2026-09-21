package infra

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/smtptest"
)

func newSMTPTestMailer(t *testing.T, srv *smtptest.Server, security, user, pass string, roots *x509.CertPool) *SMTPMailer {
	t.Helper()
	host, portStr, err := net.SplitHostPort(srv.Addr())
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(portStr)
	m, err := NewSMTPMailer(Config{
		SMTPHost: host, SMTPPort: port, SMTPUser: user, SMTPPassword: pass,
		SMTPSecurity: security, MailFrom: "BurgerStack <noreply@example.com>",
	})
	if err != nil {
		t.Fatalf("NewSMTPMailer: %v", err)
	}
	if roots != nil {
		m.tlsConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
	}
	m.timeout = 3 * time.Second
	return m
}

var testMail = mailMessage{To: "a@example.com", Subject: "Confirm your email address", Body: "Please confirm:\n\nhttps://app.example.com/signup/confirm?token=abc\n\n.line starting with a dot\n"}

// TestSMTPMailerModes は、3 つの接続の方式（暗黙の TLS・STARTTLS・認証なし）で、メールが
// 届くことを、プロセス内の SMTP サーバーで確認する。
func TestSMTPMailerModes(t *testing.T) {
	serverTLS, roots := smtptest.NewCert(t)
	tests := []struct {
		name         string
		server       *smtptest.Server
		security     string
		user, pass   string
		wantTLS      bool
		wantAuthed   bool
		wantAuthTrys int
	}{
		{"暗黙の TLS と認証", &smtptest.Server{TLSConfig: serverTLS, ImplicitTLS: true, User: "resend", Pass: "test-key"}, "tls", "resend", "test-key", true, true, 1},
		{"STARTTLS と認証", &smtptest.Server{TLSConfig: serverTLS, OfferSTARTTLS: true, User: "resend", Pass: "test-key"}, "starttls", "resend", "test-key", true, true, 1},
		{"STARTTLS で認証なし", &smtptest.Server{TLSConfig: serverTLS, OfferSTARTTLS: true}, "starttls", "", "", true, false, 0},
		{"認証なしの平文(開発用の Mailpit など)", &smtptest.Server{}, "none", "", "", false, false, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := smtptest.Start(t, tt.server)
			m := newSMTPTestMailer(t, srv, tt.security, tt.user, tt.pass, roots)
			if err := m.Deliver(context.Background(), testMail); err != nil {
				t.Fatalf("Deliver returned error: %v", err)
			}
			mails := srv.Received()
			if len(mails) != 1 {
				t.Fatalf("received mails = %d, want 1", len(mails))
			}
			got := mails[0]
			if got.From != "noreply@example.com" || len(got.To) != 1 || got.To[0] != "a@example.com" {
				t.Errorf("envelope = from %q to %v", got.From, got.To)
			}
			if got.TLS != tt.wantTLS || got.Authed != tt.wantAuthed {
				t.Errorf("tls=%v authed=%v, want tls=%v authed=%v", got.TLS, got.Authed, tt.wantTLS, tt.wantAuthed)
			}
			for _, want := range []string{
				"From: BurgerStack <noreply@example.com>\r\n",
				"To: a@example.com\r\n",
				"Subject: Confirm your email address\r\n",
				"Content-Type: text/plain; charset=UTF-8\r\n",
				"https://app.example.com/signup/confirm?token=abc",
				"\r\n.line starting with a dot", // 行頭のドットが、詰められて戻る
			} {
				if !strings.Contains(got.Data, want) {
					t.Errorf("メッセージに %q がない:\n%s", want, got.Data)
				}
			}
			if _, tries := srv.Stats(); tries != tt.wantAuthTrys {
				t.Errorf("AUTH の試行 = %d, want %d", tries, tt.wantAuthTrys)
			}
		})
	}
}

func TestSMTPMailerRefusals(t *testing.T) {
	serverTLS, roots := smtptest.NewCert(t)

	t.Run("STARTTLS を広告しないサーバーには、認証情報も本文も送らない", func(t *testing.T) {
		srv := smtptest.Start(t, &smtptest.Server{User: "resend", Pass: "test-key"})
		m := newSMTPTestMailer(t, srv, "starttls", "resend", "test-key", roots)
		err := m.Deliver(context.Background(), testMail)
		if err == nil || !strings.Contains(err.Error(), "STARTTLS") {
			t.Fatalf("error = %v, want an error about STARTTLS", err)
		}
		if got := classifyDeliveryError(err); got != domain.MailFailurePermanent {
			t.Errorf("classifyDeliveryError = %q, want permanent（暗号化できないサーバーは、直らない）", got)
		}
		if _, tries := srv.Stats(); tries != 0 {
			t.Errorf("AUTH の試行 = %d, want 0", tries)
		}
		if len(srv.Received()) != 0 {
			t.Error("メールが届いている")
		}
	})

	t.Run("認証に失敗したら送らず、エラーにパスワードを含めない", func(t *testing.T) {
		srv := smtptest.Start(t, &smtptest.Server{TLSConfig: serverTLS, OfferSTARTTLS: true, User: "resend", Pass: "right-key"})
		m := newSMTPTestMailer(t, srv, "starttls", "resend", "wrong-key", roots)
		err := m.Deliver(context.Background(), testMail)
		if err == nil {
			t.Fatal("Deliver returned nil error, want auth error")
		}
		if strings.Contains(err.Error(), "wrong-key") || strings.Contains(err.Error(), "right-key") {
			t.Errorf("エラーにパスワードが含まれている: %v", err)
		}
		if got := classifyDeliveryError(err); got != domain.MailFailurePermanent {
			t.Errorf("classifyDeliveryError = %q, want permanent（認証の失敗 535 は、直らない）", got)
		}
		if len(srv.Received()) != 0 {
			t.Error("メールが届いている")
		}
	})

	t.Run("信頼できない証明書のサーバーには接続しない(検証は有効)", func(t *testing.T) {
		srv := smtptest.Start(t, &smtptest.Server{TLSConfig: serverTLS, ImplicitTLS: true})
		m := newSMTPTestMailer(t, srv, "tls", "", "", nil) // 自己署名の証明書を信頼させない
		err := m.Deliver(context.Background(), testMail)
		if err == nil {
			t.Fatal("Deliver returned nil error, want certificate error")
		}
		if got := classifyDeliveryError(err); got != domain.MailFailurePermanent {
			t.Errorf("classifyDeliveryError = %q, want permanent（証明書の検証の失敗）", got)
		}
		if len(srv.Received()) != 0 {
			t.Error("メールが届いている")
		}
	})

	t.Run("ヘッダーに改行を含む値は、接続する前に拒否する", func(t *testing.T) {
		srv := smtptest.Start(t, &smtptest.Server{})
		m := newSMTPTestMailer(t, srv, "none", "", "", nil)
		for name, mail := range map[string]mailMessage{
			"宛先に Bcc を差し込む":      {To: "a@example.com\r\nBcc: x@example.com", Subject: "s", Body: "b"},
			"宛先に表示名がある":          {To: "Name <a@example.com>", Subject: "s", Body: "b"},
			"件名に別のヘッダーを差し込む":     {To: "a@example.com", Subject: "s\r\nX-Injected: 1", Body: "b"},
			"件名に LF だけを差し込む":     {To: "a@example.com", Subject: "s\nX-Injected: 1", Body: "b"},
			"宛先が複数":              {To: "a@example.com, b@example.com", Subject: "s", Body: "b"},
			"宛先が空":               {To: "", Subject: "s", Body: "b"},
			"宛先にヘッダー用の改行を含む(CR)": {To: "a@example.com\r", Subject: "s", Body: "b"},
		} {
			err := m.Deliver(context.Background(), mail)
			if err == nil {
				t.Errorf("%s: Deliver returned nil error, want error", name)
			} else if got := classifyDeliveryError(err); got != domain.MailFailurePermanent {
				t.Errorf("%s: classifyDeliveryError = %q, want permanent", name, got)
			}
		}
		if conns, _ := srv.Stats(); conns != 0 {
			t.Errorf("接続 = %d, want 0（拒否は接続の前に行う）", conns)
		}
	})

	t.Run("応答しないサーバーは、timeout で打ち切る", func(t *testing.T) {
		srv := smtptest.Start(t, &smtptest.Server{Hang: true})
		m := newSMTPTestMailer(t, srv, "none", "", "", nil)
		m.timeout = 300 * time.Millisecond
		start := time.Now()
		err := m.Deliver(context.Background(), testMail)
		if err == nil {
			t.Fatal("Deliver returned nil error, want timeout error")
		}
		if got := classifyDeliveryError(err); got != domain.MailFailureTemporary {
			t.Errorf("classifyDeliveryError = %q, want temporary（応答しないのは、時間をおけば直りうる）", got)
		}
		if elapsed := time.Since(start); elapsed > 3*time.Second {
			t.Errorf("Deliver が %v かかった(timeout で打ち切られていない)", elapsed)
		}
	})

	t.Run("接続できない(閉じたポート)のは、一時的な失敗になる", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		host, portStr, _ := net.SplitHostPort(ln.Addr().String())
		_ = ln.Close() // 待ち受けをやめたポートへは、接続できない。
		port, _ := strconv.Atoi(portStr)
		m, err := NewSMTPMailer(Config{SMTPHost: host, SMTPPort: port, SMTPSecurity: "none", MailFrom: "noreply@example.com"})
		if err != nil {
			t.Fatal(err)
		}
		m.timeout = time.Second
		err = m.Deliver(context.Background(), testMail)
		if err == nil {
			t.Fatal("Deliver returned nil error, want connection error")
		}
		if got := classifyDeliveryError(err); got != domain.MailFailureTemporary {
			t.Errorf("classifyDeliveryError = %q, want temporary（接続できない）", got)
		}
	})
}
