package repository_test

import (
	"context"
	"strings"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
)

// TestMailDeliveryRepository は、メール送信の記録を実際の PostgreSQL に対して検証する。
// TEST_DATABASE_URL がなければ、dbtest.New の内部で skip される。
func TestMailDeliveryRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed repository test in short mode")
	}
	ctx := context.Background()
	params := func(key string) domain.CreateMailDeliveryParams {
		return domain.CreateMailDeliveryParams{Kind: domain.MailKindSignupConfirmation, Recipient: "alice@example.com", IdempotencyKey: key}
	}

	t.Run("同じ冪等キーの要求は 1 行にしかならず、2 回目は作られない", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		repo := repository.NewMailDeliveryRepository(conn)

		id, created, err := repo.CreateMailDelivery(ctx, params("key-1"))
		if err != nil || !created || !domain.IsUUID(id) {
			t.Fatalf("1 回目 = (%q, %v, %v), want UUID の id と created=true", id, created, err)
		}
		for i := 0; i < 3; i++ {
			again, created, err := repo.CreateMailDelivery(ctx, params("key-1"))
			if err != nil || created || again != "" {
				t.Fatalf("繰り返し %d 回目 = (%q, %v, %v), want (\"\", false, nil)", i+2, again, created, err)
			}
		}
		if _, created, err := repo.CreateMailDelivery(ctx, params("key-2")); err != nil || !created {
			t.Fatalf("別の冪等キー = (%v, %v), want created=true", created, err)
		}
		if n := countRows(ctx, t, conn, "mail_deliveries"); n != 2 {
			t.Errorf("行数 = %d, want 2（key-1 が 1 行、key-2 が 1 行）", n)
		}
	})

	t.Run("作った記録は pending・試行 0 回で、結果の列は空である", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		repo := repository.NewMailDeliveryRepository(conn)
		if _, _, err := repo.CreateMailDelivery(ctx, params("key-1")); err != nil {
			t.Fatalf("作成: %v", err)
		}
		var status string
		var attempts int
		var failureKind, lastError *string
		var sentAtNull bool
		if err := conn.QueryRow(ctx,
			"SELECT status, attempts, failure_kind, last_error, sent_at IS NULL FROM mail_deliveries").
			Scan(&status, &attempts, &failureKind, &lastError, &sentAtNull); err != nil {
			t.Fatalf("select: %v", err)
		}
		if status != "pending" || attempts != 0 || failureKind != nil || lastError != nil || !sentAtNull {
			t.Errorf("pending の記録 = %q %d %v %v sentAtNull=%v", status, attempts, failureKind, lastError, sentAtNull)
		}
	})

	t.Run("送れたら sent・試行 1 回・sent_at が入り、失敗の列は空になる", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		repo := repository.NewMailDeliveryRepository(conn)
		id, _, _ := repo.CreateMailDelivery(ctx, params("key-1"))
		if err := repo.UpdateMailDeliverySent(ctx, id); err != nil {
			t.Fatalf("UpdateMailDeliverySent: %v", err)
		}
		var status string
		var attempts int
		var failureKind, lastError *string
		var sentAtSet bool
		if err := conn.QueryRow(ctx,
			"SELECT status, attempts, failure_kind, last_error, sent_at IS NOT NULL FROM mail_deliveries WHERE id = $1", id).
			Scan(&status, &attempts, &failureKind, &lastError, &sentAtSet); err != nil {
			t.Fatalf("select: %v", err)
		}
		if status != "sent" || attempts != 1 || failureKind != nil || lastError != nil || !sentAtSet {
			t.Errorf("sent の記録 = %q %d %v %v sentAtSet=%v", status, attempts, failureKind, lastError, sentAtSet)
		}
	})

	t.Run("失敗したら failed・試行 1 回・失敗の種類と理由が入り、sent_at は空のまま", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		repo := repository.NewMailDeliveryRepository(conn)
		id, _, _ := repo.CreateMailDelivery(ctx, params("key-1"))
		if err := repo.UpdateMailDeliveryFailed(ctx, id, domain.MailFailurePermanent, "550 no such user"); err != nil {
			t.Fatalf("UpdateMailDeliveryFailed: %v", err)
		}
		var status, failureKind, lastError string
		var attempts int
		var sentAtNull bool
		if err := conn.QueryRow(ctx,
			"SELECT status, attempts, failure_kind, last_error, sent_at IS NULL FROM mail_deliveries WHERE id = $1", id).
			Scan(&status, &attempts, &failureKind, &lastError, &sentAtNull); err != nil {
			t.Fatalf("select: %v", err)
		}
		if status != "failed" || attempts != 1 || failureKind != "permanent" || lastError != "550 no such user" || !sentAtNull {
			t.Errorf("failed の記録 = %q %d %q %q sentAtNull=%v", status, attempts, failureKind, lastError, sentAtNull)
		}
	})

	t.Run("制約: 種類・状態・失敗の種類・長さ・結果の列の食い違いは DB が拒否する", func(t *testing.T) {
		conn, _ := dbtest.New(t)
		const base = "INSERT INTO mail_deliveries (kind, recipient, idempotency_key, status, failure_kind, sent_at, last_error) VALUES "
		tests := []struct {
			name, values, constraint string
		}{
			{"未知の kind", "('newsletter', 'a@example.com', 'k1', 'pending', NULL, NULL, NULL)", "mail_deliveries_kind_check"},
			{"未知の status", "('signup_confirmation', 'a@example.com', 'k2', 'queued', NULL, NULL, NULL)", "mail_deliveries_status_check"},
			{"未知の failure_kind", "('signup_confirmation', 'a@example.com', 'k3', 'failed', 'weird', NULL, NULL)", "mail_deliveries_failure_kind_check"},
			{"sent なのに sent_at がない", "('signup_confirmation', 'a@example.com', 'k4', 'sent', NULL, NULL, NULL)", "mail_deliveries_sent_at_check"},
			{"pending なのに sent_at がある", "('signup_confirmation', 'a@example.com', 'k5', 'pending', NULL, now(), NULL)", "mail_deliveries_sent_at_check"},
			{"failed なのに failure_kind がない", "('signup_confirmation', 'a@example.com', 'k6', 'failed', NULL, NULL, NULL)", "mail_deliveries_failure_kind_status_check"},
			{"sent なのに failure_kind がある", "('signup_confirmation', 'a@example.com', 'k7', 'sent', 'temporary', now(), NULL)", "mail_deliveries_failure_kind_status_check"},
			{"last_error が 200 文字を超える", "('signup_confirmation', 'a@example.com', 'k8', 'failed', 'temporary', NULL, '" + strings.Repeat("x", 201) + "')", "mail_deliveries_last_error_length_check"},
			{"recipient が 254 文字を超える", "('signup_confirmation', '" + strings.Repeat("a", 255) + "', 'k9', 'pending', NULL, NULL, NULL)", "mail_deliveries_recipient_length_check"},
		}
		for _, tt := range tests {
			_, err := conn.Exec(ctx, base+tt.values)
			if err == nil || !strings.Contains(err.Error(), tt.constraint) {
				t.Errorf("%s: error = %v, want 制約 %s の違反", tt.name, err, tt.constraint)
			}
		}
		if n := countRows(ctx, t, conn, "mail_deliveries"); n != 0 {
			t.Errorf("行数 = %d, want 0（制約に反する行は入らない）", n)
		}
	})
}
