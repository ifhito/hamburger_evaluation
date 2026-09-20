package infra

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool は cfg からプロセス全体で唯一の pgx pool を構築する。明示的な
// MaxConns を設定し、すぐには接続しない（pgxpool v5 は遅延して接続する）。
// 呼び出し側は Ping を通して接続性を確認する。pool は main で 1 回だけ作成
// され、shutdown 時に close される。
func NewPool(ctx context.Context, cfg Config) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	poolCfg.MaxConns = cfg.DBMaxConns
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create pgx pool: %w", err)
	}
	return pool, nil
}
