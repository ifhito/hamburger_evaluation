// Package smtptest は、テストで使うプロセス内の SMTP サーバーを提供する。暗黙の TLS・STARTTLS・
// 認証なしの 3 つの接続の方式と、AUTH PLAIN を試せる。実際のプロバイダー（Resend など）には依存しない。
package smtptest

import (
	"bufio"
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
	"strings"
	"sync"
	"testing"
	"time"
)

// Mail は、テスト用の SMTP サーバーが受け取った 1 通である。
type Mail struct {
	From   string
	To     []string
	Data   string
	TLS    bool // 暗号化された接続で受け取ったか
	Authed bool
}

// Server は、プロセス内のテスト用の SMTP サーバーである。暗黙の TLS・STARTTLS・
// 認証なしの 3 つの方式を試せる。実際の Resend などには依存しない。
type Server struct {
	// TLSConfig は、暗黙の TLS・STARTTLS で使うサーバーの証明書の設定である。
	TLSConfig *tls.Config
	// ImplicitTLS は、接続の直後から TLS で話す（ポート 465 相当）。
	ImplicitTLS bool
	// OfferSTARTTLS は、STARTTLS を広告する（ポート 587 相当）。
	OfferSTARTTLS bool
	// User と Pass は、空でなければ AUTH PLAIN を要求する。
	User, Pass string
	// Hang は、接続を受けても応答を返さない。
	Hang bool

	ln net.Listener

	mu           sync.Mutex
	mails        []Mail
	connections  int
	authAttempts int
}

// NewCert は、127.0.0.1 と localhost 用の自己署名の証明書と、それを信頼する CertPool を返す。
func NewCert(t *testing.T) (*tls.Config, *x509.CertPool) {
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

// Start は srv を 127.0.0.1 の空きポートで起動し、テストの終了時に止める。
func Start(t *testing.T, srv *Server) *Server {
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

// Received は、これまでに受け取ったメールを返す。
func (s *Server) Received() []Mail {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Mail(nil), s.mails...)
}

// Stats は、受けた接続の数と、AUTH の試行の数を返す。
func (s *Server) Stats() (connections, authAttempts int) {
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

func (s *Server) handle(raw net.Conn) {
	defer raw.Close()
	s.mu.Lock()
	s.connections++
	s.mu.Unlock()
	if s.Hang {
		buf := make([]byte, 64)
		for {
			if _, err := raw.Read(buf); err != nil {
				return
			}
		}
	}
	conn := raw
	isTLS := false
	if s.ImplicitTLS {
		conn = tls.Server(raw, s.TLSConfig)
		isTLS = true
	}
	r := bufio.NewReader(conn)
	write := func(format string, args ...any) { fmt.Fprintf(conn, format+"\r\n", args...) }
	write("220 test ESMTP")

	var cur Mail
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
			if s.OfferSTARTTLS && !isTLS {
				write("250-STARTTLS")
			}
			if s.User != "" {
				write("250-AUTH PLAIN")
			}
			write("250 8BITMIME")
		case up == "STARTTLS":
			write("220 ready to start TLS")
			conn = tls.Server(raw, s.TLSConfig)
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
			if len(fields) == 3 && fields[1] == s.User && fields[2] == s.Pass {
				authed = true
				write("235 authenticated")
			} else {
				write("535 authentication failed")
			}
		case strings.HasPrefix(up, "MAIL FROM:"):
			if s.User != "" && !authed {
				write("530 authentication required")
				continue
			}
			cur = Mail{From: angleAddr(line), TLS: isTLS, Authed: authed}
			write("250 ok")
		case strings.HasPrefix(up, "RCPT TO:"):
			cur.To = append(cur.To, angleAddr(line))
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
			cur.Data = data.String()
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

// Addr は、サーバーが待ち受けているアドレス（host:port）を返す。
func (s *Server) Addr() string { return s.ln.Addr().String() }
