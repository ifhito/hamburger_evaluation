package uow

import (
	"context"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/query"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository/sqlcgen"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// beginnerDBTX は、クエリを実行でき、トランザクションも開始できる接続である。
// 本番の接続プール(*pgxpool.Pool)と、テストの単一の接続(*pgx.Conn)のどちらも満たす。
type beginnerDBTX interface {
	sqlcgen.DBTX
	beginner
}

// OAuthTokenSessionStore は、usecase.OAuthTokenSessionStore を pgx で実装したものである。トークンの記録の
// 書き込み(domain の書き込みオブジェクト)と読み取り(query)を、同じ接続、または同じトランザクションに
// 結び付ける。repository と query を組み合わせるのは、このパッケージの役目である。
type OAuthTokenSessionStore struct {
	db beginnerDBTX
}

// NewOAuthTokenSessionStore は、db(通常は共有の pgx pool)を使う OAuthTokenSessionStore を返す。
func NewOAuthTokenSessionStore(db beginnerDBTX) *OAuthTokenSessionStore {
	return &OAuthTokenSessionStore{db: db}
}

var _ usecase.OAuthTokenSessionStore = (*OAuthTokenSessionStore)(nil)

func scopeOn(db sqlcgen.DBTX) usecase.OAuthTokenSessionScope {
	return usecase.OAuthTokenSessionScope{
		Writes: domain.NewOAuthTokenSessions(repository.NewOAuthTokenSessionRepository(db)),
		Reads:  query.NewOAuthTokenSessionQuery(db),
	}
}

// Scope は、トランザクションの外で使う組を返す(操作ごとに確定する)。
func (s *OAuthTokenSessionStore) Scope() usecase.OAuthTokenSessionScope {
	return scopeOn(s.db)
}

// Begin は、トランザクションを開始し、それに結び付いた組と、確定・取り消しを返す。
func (s *OAuthTokenSessionStore) Begin(ctx context.Context) (usecase.OAuthTokenSessionTx, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return usecase.OAuthTokenSessionTx{}, err
	}
	return usecase.OAuthTokenSessionTx{OAuthTokenSessionScope: scopeOn(tx), Commit: tx.Commit, Rollback: tx.Rollback}, nil
}
