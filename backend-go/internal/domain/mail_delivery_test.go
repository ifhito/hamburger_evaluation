package domain

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestSignupConfirmationMailKey(t *testing.T) {
	a := SignupConfirmationMailKey("11111111-1111-4111-8111-111111111111", 1)
	if a != "signup_confirmation:11111111-1111-4111-8111-111111111111:1" {
		t.Errorf("key = %q", a)
	}
	if a != SignupConfirmationMailKey("11111111-1111-4111-8111-111111111111", 1) {
		t.Error("同じ確認待ちの同じ世代で、キーが変わった")
	}
	if a == SignupConfirmationMailKey("11111111-1111-4111-8111-111111111111", 2) {
		t.Error("世代が進んだのに、キーが同じ")
	}
	if a == SignupConfirmationMailKey("22222222-2222-4222-8222-222222222222", 1) {
		t.Error("別の確認待ちなのに、キーが同じ")
	}
}

func TestAlreadyRegisteredMailKey(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC) // 窓の先頭（60 秒の倍数）
	tests := []struct {
		name      string
		email     string
		at        time.Time
		sameAsKey bool // base の a@example.com と同じキーになるか
	}{
		{"同じ窓の先頭", "a@example.com", base, true},
		{"同じ窓の終わり（59 秒後）", "a@example.com", base.Add(59 * time.Second), true},
		{"大文字小文字だけが違う宛先は同じキー", "A@Example.COM", base.Add(10 * time.Second), true},
		{"次の窓（60 秒後）は別のキー", "a@example.com", base.Add(60 * time.Second), false},
		{"前の窓（1 秒前）は別のキー", "a@example.com", base.Add(-time.Second), false},
		{"別の宛先は別のキー", "b@example.com", base, false},
	}
	want := AlreadyRegisteredMailKey("a@example.com", base)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AlreadyRegisteredMailKey(tt.email, tt.at)
			if (got == want) != tt.sameAsKey {
				t.Errorf("key = %q, base の key = %q, 同じキーになる = %v を期待", got, want, tt.sameAsKey)
			}
		})
	}
	if !strings.HasPrefix(want, "already_registered:a@example.com:") {
		t.Errorf("key = %q", want)
	}
}

func TestTruncateMailError(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"短い理由はそのまま", "550 no such user", "550 no such user"},
		{"改行と連続する空白は 1 つの空白にまとめる", "smtp: rcpt to:\r\n  550   no\tsuch user\n", "smtp: rcpt to: 550 no such user"},
		{"空は空", "", ""},
		{"ちょうど上限の長さはそのまま", strings.Repeat("x", MaxMailErrorLength), strings.Repeat("x", MaxMailErrorLength)},
		{"上限を超えたら文字数(rune)で切る", strings.Repeat("あ", MaxMailErrorLength+50), strings.Repeat("あ", MaxMailErrorLength)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TruncateMailError(tt.in); got != tt.want {
				t.Errorf("TruncateMailError(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// recordingMailRepo は、MailDeliveries が repository へ渡した値を記録する fake である。
type recordingMailRepo struct {
	created []CreateMailDeliveryParams
	sent    []string
	failed  []struct {
		id      string
		failure MailFailure
		reason  string
	}
}

func (r *recordingMailRepo) CreateMailDelivery(_ context.Context, p CreateMailDeliveryParams) (string, bool, error) {
	r.created = append(r.created, p)
	return "id-1", true, nil
}

func (r *recordingMailRepo) UpdateMailDeliverySent(_ context.Context, id string) error {
	r.sent = append(r.sent, id)
	return nil
}

func (r *recordingMailRepo) UpdateMailDeliveryFailed(_ context.Context, id string, failure MailFailure, reason string) error {
	r.failed = append(r.failed, struct {
		id      string
		failure MailFailure
		reason  string
	}{id, failure, reason})
	return nil
}

func TestMailDeliveriesMarkFailed(t *testing.T) {
	ctx := context.Background()
	t.Run("失敗の種類はそのまま、理由は切り詰めて記録する", func(t *testing.T) {
		repo := &recordingMailRepo{}
		if err := NewMailDeliveries(repo).MarkFailed(ctx, "id-1", MailFailurePermanent, strings.Repeat("e", 500)); err != nil {
			t.Fatalf("MarkFailed returned error: %v", err)
		}
		if len(repo.failed) != 1 || repo.failed[0].failure != MailFailurePermanent || len([]rune(repo.failed[0].reason)) != MaxMailErrorLength {
			t.Errorf("記録した値 = %+v", repo.failed)
		}
	})
	t.Run("未知の失敗の種類は temporary として記録する", func(t *testing.T) {
		repo := &recordingMailRepo{}
		_ = NewMailDeliveries(repo).MarkFailed(ctx, "id-1", MailFailure("weird"), "x")
		if len(repo.failed) != 1 || repo.failed[0].failure != MailFailureTemporary {
			t.Errorf("記録した値 = %+v", repo.failed)
		}
	})
	t.Run("Record と MarkSent は、そのまま repository へ渡す", func(t *testing.T) {
		repo := &recordingMailRepo{}
		m := NewMailDeliveries(repo)
		id, recorded, err := m.Record(ctx, CreateMailDeliveryParams{Kind: MailKindAlreadyRegistered, Recipient: "a@example.com", IdempotencyKey: "k"})
		if err != nil || !recorded || id != "id-1" {
			t.Fatalf("Record = (%q, %v, %v)", id, recorded, err)
		}
		_ = m.MarkSent(ctx, id)
		if len(repo.created) != 1 || repo.created[0].IdempotencyKey != "k" || len(repo.sent) != 1 || repo.sent[0] != "id-1" {
			t.Errorf("repository へ渡した値 = %+v / %v", repo.created, repo.sent)
		}
	})
}

func TestSignupTokenTTLIsTheBusinessRule(t *testing.T) {
	if SignupTokenTTL != 24*time.Hour {
		t.Errorf("SignupTokenTTL = %v, want 24h（有効期間は業務のルールで、設定では変えない）", SignupTokenTTL)
	}
	if SignupResendInterval != MailResendInterval {
		t.Errorf("SignupResendInterval = %v, MailResendInterval = %v, want 同じ幅（確認メールの間隔と、通知の窓）", SignupResendInterval, MailResendInterval)
	}
}
