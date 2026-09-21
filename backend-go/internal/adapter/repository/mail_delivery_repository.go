package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository/sqlcgen"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// MailDeliveryRepository は、sqlc 生成のクエリ上で domain.MailDeliveryRepository（書き込み）を
// 実装する。
type MailDeliveryRepository struct {
	q *sqlcgen.Queries
}

// NewMailDeliveryRepository は db（通常は共有の pgx pool）をラップする。
func NewMailDeliveryRepository(db sqlcgen.DBTX) *MailDeliveryRepository {
	return &MailDeliveryRepository{q: sqlcgen.New(db)}
}

var _ domain.MailDeliveryRepository = (*MailDeliveryRepository)(nil)

// CreateMailDelivery は送信の記録を pending で作る。同じ冪等キーがすでにあるときは、DB の
// INSERT ... ON CONFLICT DO NOTHING が行を返さないので、created=false を返す。
func (r *MailDeliveryRepository) CreateMailDelivery(ctx context.Context, params domain.CreateMailDeliveryParams) (string, bool, error) {
	id, err := r.q.CreateMailDelivery(ctx, sqlcgen.CreateMailDeliveryParams{
		Kind:           string(params.Kind),
		Recipient:      params.Recipient,
		IdempotencyKey: params.IdempotencyKey,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("create mail delivery: %w", err)
	}
	return id, true, nil
}

// UpdateMailDeliverySent は id の記録を sent にする。
func (r *MailDeliveryRepository) UpdateMailDeliverySent(ctx context.Context, id string) error {
	if err := r.q.UpdateMailDeliverySent(ctx, id); err != nil {
		return fmt.Errorf("update mail delivery sent: %w", err)
	}
	return nil
}

// UpdateMailDeliveryFailed は id の記録を failed にし、失敗の種類と理由を記録する。
func (r *MailDeliveryRepository) UpdateMailDeliveryFailed(ctx context.Context, id string, failure domain.MailFailure, reason string) error {
	err := r.q.UpdateMailDeliveryFailed(ctx, sqlcgen.UpdateMailDeliveryFailedParams{
		ID:          id,
		FailureKind: pgtype.Text{String: string(failure), Valid: true},
		LastError:   pgtype.Text{String: reason, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("update mail delivery failed: %w", err)
	}
	return nil
}
