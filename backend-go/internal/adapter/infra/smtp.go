package infra

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// このパッケージ内で起こす、送っても直らない種類のエラー（classifyDeliveryError が恒久的な失敗に翻訳する）。
var (
	errInvalidMessage      = errors.New("smtp: invalid header value")
	errSTARTTLSUnsupported = errors.New("smtp: server does not support STARTTLS")
	errAuthUnsupported     = errors.New("smtp: server does not support AUTH")
)

// classifyDeliveryError は、SMTP の実装のエラーを、domain の言葉（一時的な失敗・恒久的な失敗）に
// 翻訳する。SMTP の応答コード（4xx は一時的、5xx は恒久的）や、TLS の証明書の検証の失敗、
// 送る前に拒否した不正なヘッダー・暗号化できないサーバーは、恒久的な失敗である。接続できない・
// タイムアウトなど、それ以外は一時的な失敗として扱う。プロバイダーのエラーの型は、ここから外へ出さない。
func classifyDeliveryError(err error) domain.MailFailure {
	var protoErr *textproto.Error
	var certErr *tls.CertificateVerificationError
	switch {
	case errors.As(err, &protoErr):
		if protoErr.Code >= 500 {
			return domain.MailFailurePermanent
		}
		return domain.MailFailureTemporary
	case errors.Is(err, errInvalidMessage), errors.Is(err, errSTARTTLSUnsupported), errors.Is(err, errAuthUnsupported),
		errors.As(err, &certErr):
		return domain.MailFailurePermanent
	default:
		return domain.MailFailureTemporary
	}
}

// defaultSMTPTimeout は、1 通の送信（接続から QUIT まで）の上限である。
const defaultSMTPTimeout = 15 * time.Second

// SMTPMailer は、汎用の SMTP でメールを 1 通ずつ同期的に送る。プロバイダー固有の実装は
// 持たない（Resend も Amazon SES も、ホスト・ポート・認証の設定だけで使える）。接続の保護は
// 3 つの方式に対応する: "tls"（暗黙の TLS。ポート 465）、"starttls"（ポート 587）、
// "none"（開発用。認証なし）。認証情報は、暗号化された接続でしか送らない。
// 呼び出しを待たせない非同期化は AsyncMailer が担う。
type SMTPMailer struct {
	host      string
	addr      string
	user      string
	password  string
	security  string
	from      string // ヘッダーの From（表示名つきでもよい）
	envelope  string // MAIL FROM に使うアドレスだけ
	timeout   time.Duration
	tlsConfig *tls.Config // テストで自己署名の証明書を信頼させるためのもの。nil なら標準の検証
}

// NewSMTPMailer は cfg の SMTP_* と MAIL_FROM から SMTPMailer を作る。
func NewSMTPMailer(cfg Config) (*SMTPMailer, error) {
	from, err := mail.ParseAddress(cfg.MailFrom)
	if err != nil {
		return nil, fmt.Errorf("parse MAIL_FROM: %w", err)
	}
	return &SMTPMailer{
		host:     cfg.SMTPHost,
		addr:     net.JoinHostPort(cfg.SMTPHost, strconv.Itoa(cfg.SMTPPort)),
		user:     cfg.SMTPUser,
		password: cfg.SMTPPassword,
		security: cfg.SMTPSecurity,
		from:     cfg.MailFrom,
		envelope: from.Address,
		timeout:  defaultSMTPTimeout,
	}, nil
}

// Deliver は m を 1 通送る。接続から送信までを、ctx と timeout のうち早い方で打ち切る。
// ヘッダーに入る値（宛先・件名・送信元）に改行が含まれていれば、接続する前に拒否する
// （ヘッダーインジェクションの防止）。エラーに、パスワードや本文は含めない。
func (m *SMTPMailer) Deliver(ctx context.Context, msg mailMessage) error {
	to, err := mail.ParseAddress(msg.To)
	if err != nil || to.Address != msg.To || strings.ContainsAny(msg.To+msg.Subject+m.from, "\r\n") {
		return errInvalidMessage
	}
	data, err := m.buildMessage(msg)
	if err != nil {
		return err
	}

	deadline := time.Now().Add(m.timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	conn, err := m.dial(ctx)
	if err != nil {
		return fmt.Errorf("smtp: connect: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(deadline)

	c, err := smtp.NewClient(conn, m.host)
	if err != nil {
		return fmt.Errorf("smtp: greeting: %w", err)
	}
	defer c.Close()

	if m.security == smtpSecurityStartTLS {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			// 暗号化できないサーバーには、認証情報も本文も送らない（平文への格下げを拒否する）。
			return errSTARTTLSUnsupported
		}
		if err := c.StartTLS(m.tlsConfigFor()); err != nil {
			return fmt.Errorf("smtp: starttls: %w", err)
		}
	}
	if m.user != "" {
		if ok, _ := c.Extension("AUTH"); !ok {
			return errAuthUnsupported
		}
		if err := c.Auth(smtp.PlainAuth("", m.user, m.password, m.host)); err != nil {
			return fmt.Errorf("smtp: auth: %w", err)
		}
	}
	if err := c.Mail(m.envelope); err != nil {
		return fmt.Errorf("smtp: mail from: %w", err)
	}
	if err := c.Rcpt(to.Address); err != nil {
		return fmt.Errorf("smtp: rcpt to: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("smtp: data: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("smtp: write body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp: end data: %w", err)
	}
	// QUIT の失敗は、メールがすでに受理された後なので、送信の失敗として扱わない。
	_ = c.Quit()
	return nil
}

func (m *SMTPMailer) dial(ctx context.Context) (net.Conn, error) {
	d := &net.Dialer{Timeout: m.timeout}
	if m.security == smtpSecurityTLS {
		td := tls.Dialer{NetDialer: d, Config: m.tlsConfigFor()}
		return td.DialContext(ctx, "tcp", m.addr)
	}
	return d.DialContext(ctx, "tcp", m.addr)
}

// tlsConfigFor は、サーバー名の検証を有効にした TLS の設定を返す。
func (m *SMTPMailer) tlsConfigFor() *tls.Config {
	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if m.tlsConfig != nil {
		cfg = m.tlsConfig.Clone()
	}
	if cfg.ServerName == "" {
		cfg.ServerName = m.host
	}
	return cfg
}

// buildMessage は、プレーンテキスト（UTF-8）のメッセージを組み立てる。本文の改行は、
// net/smtp の Data() の書き込みが CRLF に変換し、行頭の "." も処理する。
func (m *SMTPMailer) buildMessage(msg mailMessage) ([]byte, error) {
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return nil, fmt.Errorf("smtp: message id: %w", err)
	}
	domain := "localhost"
	if i := strings.LastIndex(m.envelope, "@"); i >= 0 {
		domain = m.envelope[i+1:]
	}
	headers := []string{
		"From: " + m.from,
		"To: " + msg.To,
		"Subject: " + msg.Subject,
		"Date: " + time.Now().UTC().Format(time.RFC1123Z),
		"Message-ID: <" + hex.EncodeToString(id) + "@" + domain + ">",
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"Content-Transfer-Encoding: 8bit",
	}
	return []byte(strings.Join(headers, "\r\n") + "\r\n\r\n" + msg.Body), nil
}
