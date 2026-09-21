package query

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository/sqlcgen"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// OAuthGrantQuery は、sqlc 生成のクエリ上で usecase.OAuthGrantQuery を実装する。
type OAuthGrantQuery struct {
	q *sqlcgen.Queries
}

// NewOAuthGrantQuery は db（通常は共有の pgx pool）をラップする。
func NewOAuthGrantQuery(db sqlcgen.DBTX) *OAuthGrantQuery {
	return &OAuthGrantQuery{q: sqlcgen.New(db)}
}

var _ usecase.OAuthGrantQuery = (*OAuthGrantQuery)(nil)

func toOAuthGrant(row sqlcgen.OauthGrant) domain.OAuthGrant {
	return domain.OAuthGrant{
		ID:         row.ID,
		UserID:     row.UserID,
		ClientID:   row.ClientID,
		ClientName: row.ClientName,
		Scopes:     row.Scopes,
		CreatedAt:  row.CreatedAt.Time,
		UpdatedAt:  row.UpdatedAt.Time,
	}
}

// GetOAuthGrantByUserAndClient は、利用者とアプリの組の許可を返す。なければ
// domain.ErrOAuthGrantNotFound を返す。
func (r *OAuthGrantQuery) GetOAuthGrantByUserAndClient(ctx context.Context, userID, clientID string) (domain.OAuthGrant, error) {
	row, err := r.q.GetOAuthGrantByUserAndClient(ctx, sqlcgen.GetOAuthGrantByUserAndClientParams{UserID: userID, ClientID: clientID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.OAuthGrant{}, fmt.Errorf("get oauth grant: %w", domain.ErrOAuthGrantNotFound)
		}
		return domain.OAuthGrant{}, fmt.Errorf("get oauth grant: %w", err)
	}
	return toOAuthGrant(row), nil
}

// ListOAuthGrantsByUser は、利用者が許可したアプリを、最近使ったものから順に、limit 件まで返す(offset 件を飛ばす)。
// 2 つ目の戻り値は、続き(次のページ)があるかである。続きを知るために、1 件多く取り出して、切り詰める。
func (r *OAuthGrantQuery) ListOAuthGrantsByUser(ctx context.Context, userID string, limit, offset int32) ([]domain.OAuthGrant, bool, error) {
	rows, err := r.q.ListOAuthGrantsByUser(ctx, sqlcgen.ListOAuthGrantsByUserParams{UserID: userID, PageLimit: limit + 1, PageOffset: offset})
	if err != nil {
		return nil, false, fmt.Errorf("list oauth grants: %w", err)
	}
	rows, hasMore := trimPage(rows, limit)
	grants := make([]domain.OAuthGrant, 0, len(rows))
	for _, row := range rows {
		grants = append(grants, toOAuthGrant(row))
	}
	return grants, hasMore, nil
}
