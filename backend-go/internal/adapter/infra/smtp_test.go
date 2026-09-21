package infra

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"fmt"
	"math/big"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// receivedMail は、テスト用の SMTP サーバーが受け取った 1 通である。
type receivedMail struct {
	from   string
	to     []string
	data   string
	tls    bool // 暗号化された接続で受け取ったか
	authed bool
}

// smtpTestServer は、プロセス内のテスト用の SMTP サーバーである。暗黙の TLS・STARTTLS・
// 認証なしの 3 つの方式を試せる。実際の Resend などには依存しない。
type smtpTestServer struct {
	ln          net.Listener
	tlsConf     *tls.Config
	implicitTLS bool
	offerTLS    bool   // STARTTLS を広告する
	user, pass  string // 空でなければ AUTH PLAIN を要求する
	hang        bool   // 接続を受けても、応答を返さない

	mu           sync.Mutex
	mails        []receivedMail
	connections  int
	authAttempts int
}

// newTestCert は、127.0.0.1 と localhost 用の自己署名の証明書と、それを信頼する CertPool を返す。
func newTestCert(t *testing.T) (*tls.Config, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "smtp test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:              []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}, MinVersion: tls.VersionTLS12}, pool
}

func startSMTPServer(t *testing.T, srv *smtpTestServer) *smtpTestServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv.ln = ln
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go srv.handle(c)
		}
	}()
	return srv
}

func (s *smtpTestServer) received() []receivedMail {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]receivedMail(nil), s.mails...)
}

func (s *smtpTestServer) stats() (connections, authAttempts int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.connections, s.authAttempts
}

func angleAddr(line string) string {
	i, j := strings.Index(line, "<"), strings.Index(line, ">")
	if i < 0 || j < i {
		return ""
	}
	return line[i+1 : j]
}

func (s *smtpTestServer) handle(raw net.Conn) {
	defer raw.Close()
	s.mu.Lock()
	s.connections++
	s.mu.Unlock()
	if s.hang {
		buf := make([]byte, 64)
		for {
			if _, err := raw.Read(buf); err != nil {
				return
			}
		}
	}
	conn := raw
	isTLS := false
	if s.implicitTLS {
		conn = tls.Server(raw, s.tlsConf)
		isTLS = true
	}
	r := bufio.NewReader(conn)
	write := func(format string, args ...any) { fmt.Fprintf(conn, format+"\r\n", args...) }
	write("220 test ESMTP")

	var cur receivedMail
	authed := false
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		up := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(up, "EHLO"), strings.HasPrefix(up, "HELO"):
			write("250-test")
			if s.offerTLS && !isTLS {
				write("250-STARTTLS")
			}
			if s.user != "" {
				write("250-AUTH PLAIN")
			}
			write("250 8BITMIME")
		case up == "STARTTLS":
			write("220 ready to start TLS")
			conn = tls.Server(raw, s.tlsConf)
			r = bufio.NewReader(conn)
			isTLS = true
			authed = false
		case strings.HasPrefix(up, "AUTH PLAIN"):
			s.mu.Lock()
			s.authAttempts++
			s.mu.Unlock()
			parts := strings.Fields(line)
			dec, _ := base64.StdEncoding.DecodeString(parts[len(parts)-1])
			fields := strings.Split(string(dec), "\x00")
			if len(fields) == 3 && fields[1] == s.user && fields[2] == s.pass {
				authed = true
				write("235 authenticated")
			} else {
				write("535 authentication failed")
			}
		case strings.HasPrefix(up, "MAIL FROM:"):
			if s.user != "" && !authed {
				write("530 authentication required")
				continue
			}
			cur = receivedMail{from: angleAddr(line), tls: isTLS, authed: authed}
			write("250 ok")
		case strings.HasPrefix(up, "RCPT TO:"):
			cur.to = append(cur.to, angleAddr(line))
			write("250 ok")
		case up == "DATA":
			write("354 end with <CRLF>.<CRLF>")
			var data strings.Builder
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" {
					break
				}
				data.WriteString(strings.TrimPrefix(l, ".")) // ドット詰めを戻す
			}
			cur.data = data.String()
			s.mu.Lock()
			s.mails = append(s.mails, cur)
			s.mu.Unlock()
			write("250 queued")
		case up == "QUIT":
			write("221 bye")
			return
		default:
			write("502 not supported")
		}
	}
}

func newSMTPTestMailer(t *testing.T, srv *smtpTestServer, security, user, pass string, roots *x509.CertPool) *SMTPMailer {
	t.Helper()
	host, portStr, err := net.SplitHostPort(srv.ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(portStr)
	m, err := NewSMTPMailer(Config{
		SMTPHost: host, SMTPPort: port, SMTPUser: user, SMTPPassword: pass,
		SMTPSecurity: security, MailFrom: "Hamburger <noreply@example.com>",
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

var testMail = usecase.Mail{To: "a@example.com", Subject: "Confirm your email address", Body: "Please confirm:\n\nhttps://app.example.com/signup/confirm?token=abc\n\n.line starting with a dot\n"}

// TestSMTPMailerModes は、3 つの接続の方式（暗黙の TLS・STARTTLS・認証なし）で、メールが
// 届くことを、プロセス内の SMTP サーバーで確認する（AC15）。
func TestSMTPMailerModes(t *testing.T) {
	serverTLS, roots := newTestCert(t)
	tests := []struct {
		name         string
		server       *smtpTestServer
		security     string
		user, pass   string
		wantTLS      bool
		wantAuthed   bool
		wantAuthTrys int
	}{
		{"暗黙の TLS と認証", &smtpTestServer{tlsConf: serverTLS, implicitTLS: true, user: "resend", pass: "test-key"}, "tls", "resend", "test-key", true, true, 1},
		{"STARTTLS と認証", &smtpTestServer{tlsConf: serverTLS, offerTLS: true, user: "resend", pass: "test-key"}, "starttls", "resend", "test-key", true, true, 1},
		{"STARTTLS で認証なし", &smtpTestServer{tlsConf: serverTLS, offerTLS: true}, "starttls", "", "", true, false, 0},
		{"認証なしの平文(開発用の Mailpit など)", &smtpTestServer{}, "none", "", "", false, false, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := startSMTPServer(t, tt.server)
			m := newSMTPTestMailer(t, srv, tt.security, tt.user, tt.pass, roots)
			if err := m.Deliver(context.Background(), testMail); err != nil {
				t.Fatalf("Deliver returned error: %v", err)
			}
			mails := srv.received()
			if len(mails) != 1 {
				t.Fatalf("received mails = %d, want 1", len(mails))
			}
			got := mails[0]
			if got.from != "noreply@example.com" || len(got.to) != 1 || got.to[0] != "a@example.com" {
				t.Errorf("envelope = from %q to %v", got.from, got.to)
			}
			if got.tls != tt.wantTLS || got.authed != tt.wantAuthed {
				t.Errorf("tls=%v authed=%v, want tls=%v authed=%v", got.tls, got.authed, tt.wantTLS, tt.wantAuthed)
			}
			for _, want := range []string{
				"From: Hamburger <noreply@example.com>\r\n",
				"To: a@example.com\r\n",
				"Subject: Confirm your email address\r\n",
				"Content-Type: text/plain; charset=UTF-8\r\n",
				"https://app.example.com/signup/confirm?token=abc",
				"\r\n.line starting with a dot", // 行頭のドットが、詰められて戻る
			} {
				if !strings.Contains(got.data, want) {
					t.Errorf("メッセージに %q がない:\n%s", want, got.data)
				}
			}
			if _, tries := srv.stats(); tries != tt.wantAuthTrys {
				t.Errorf("AUTH の試行 = %d, want %d", tries, tt.wantAuthTrys)
			}
		})
	}
}

func TestSMTPMailerRefusals(t *testing.T) {
	serverTLS, roots := newTestCert(t)

	t.Run("STARTTLS を広告しないサーバーには、認証情報も本文も送らない", func(t *testing.T) {
		srv := startSMTPServer(t, &smtpTestServer{user: "resend", pass: "test-key"})
		m := newSMTPTestMailer(t, srv, "starttls", "resend", "test-key", roots)
		err := m.Deliver(context.Background(), testMail)
		if err == nil || !strings.Contains(err.Error(), "STARTTLS") {
			t.Fatalf("error = %v, want an error about STARTTLS", err)
		}
		if _, tries := srv.stats(); tries != 0 {
			t.Errorf("AUTH の試行 = %d, want 0", tries)
		}
		if len(srv.received()) != 0 {
			t.Error("メールが届いている")
		}
	})

	t.Run("認証に失敗したら送らず、エラーにパスワードを含めない", func(t *testing.T) {
		srv := startSMTPServer(t, &smtpTestServer{tlsConf: serverTLS, offerTLS: true, user: "resend", pass: "right-key"})
		m := newSMTPTestMailer(t, srv, "starttls", "resend", "wrong-key", roots)
		err := m.Deliver(context.Background(), testMail)
		if err == nil {
			t.Fatal("Deliver returned nil error, want auth error")
		}
		if strings.Contains(err.Error(), "wrong-key") || strings.Contains(err.Error(), "right-key") {
			t.Errorf("エラーにパスワードが含まれている: %v", err)
		}
		if len(srv.received()) != 0 {
			t.Error("メールが届いている")
		}
	})

	t.Run("信頼できない証明書のサーバーには接続しない(検証は有効)", func(t *testing.T) {
		srv := startSMTPServer(t, &smtpTestServer{tlsConf: serverTLS, implicitTLS: true})
		m := newSMTPTestMailer(t, srv, "tls", "", "", nil) // 自己署名の証明書を信頼させない
		if err := m.Deliver(context.Background(), testMail); err == nil {
			t.Fatal("Deliver returned nil error, want certificate error")
		}
		if len(srv.received()) != 0 {
			t.Error("メールが届いている")
		}
	})

	t.Run("ヘッダーに改行を含む値は、接続する前に拒否する", func(t *testing.T) {
		srv := startSMTPServer(t, &smtpTestServer{})
		m := newSMTPTestMailer(t, srv, "none", "", "", nil)
		for name, mail := range map[string]usecase.Mail{
			"宛先に Bcc を差し込む":      {To: "a@example.com\r\nBcc: x@example.com", Subject: "s", Body: "b"},
			"宛先に表示名がある":          {To: "Name <a@example.com>", Subject: "s", Body: "b"},
			"件名に別のヘッダーを差し込む":     {To: "a@example.com", Subject: "s\r\nX-Injected: 1", Body: "b"},
			"件名に LF だけを差し込む":     {To: "a@example.com", Subject: "s\nX-Injected: 1", Body: "b"},
			"宛先が複数":              {To: "a@example.com, b@example.com", Subject: "s", Body: "b"},
			"宛先が空":               {To: "", Subject: "s", Body: "b"},
			"宛先にヘッダー用の改行を含む(CR)": {To: "a@example.com\r", Subject: "s", Body: "b"},
		} {
			if err := m.Deliver(context.Background(), mail); err == nil {
				t.Errorf("%s: Deliver returned nil error, want error", name)
			}
		}
		if conns, _ := srv.stats(); conns != 0 {
			t.Errorf("接続 = %d, want 0（拒否は接続の前に行う）", conns)
		}
	})

	t.Run("応答しないサーバーは、timeout で打ち切る", func(t *testing.T) {
		srv := startSMTPServer(t, &smtpTestServer{hang: true})
		m := newSMTPTestMailer(t, srv, "none", "", "", nil)
		m.timeout = 300 * time.Millisecond
		start := time.Now()
		if err := m.Deliver(context.Background(), testMail); err == nil {
			t.Fatal("Deliver returned nil error, want timeout error")
		}
		if elapsed := time.Since(start); elapsed > 3*time.Second {
			t.Errorf("Deliver が %v かかった(timeout で打ち切られていない)", elapsed)
		}
	})
}
