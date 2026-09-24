package repository_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/dbtest"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
)

// このファイルは adapter/repository の DB 統合テストである。書き込み（CUD）の結果は、
// adapter/query に頼らず、SQL の SELECT で直接確かめる（読み取りは adapter/query のテストが担う）。

// shopRow は、shops の保存された行である（DB のカラムの値のまま。status は
// smallint のコード：0=pending、1=active、2=rejected）。
type shopRow struct {
	Name      string
	Status    int16
	Note      *string
	MapURL    *string
	CreatorID *string
	ClosedAt  *time.Time
}

// readShopRow は shops の行を直接読み取る。
func readShopRow(ctx context.Context, t *testing.T, conn *pgx.Conn, id string) shopRow {
	t.Helper()
	var r shopRow
	if err := conn.QueryRow(ctx,
		`SELECT name, status, moderation_note, map_url, creator_id, closed_at FROM shops WHERE id = $1`, id,
	).Scan(&r.Name, &r.Status, &r.Note, &r.MapURL, &r.CreatorID, &r.ClosedAt); err != nil {
		t.Fatalf("select shop %s: %v", id, err)
	}
	return r
}

// TestShopModerationRepository は、shop の投稿と moderation の永続化を、実際の
// PostgreSQL に対して検証する。pending の shop の作成、moderation の update
// （status・name のカラム単位の書き込み）、並行する更新を巻き戻さないこと、
// そして存在しない id の扱いである。書き込みの結果は SQL で直接確かめる
// （moderation 一覧・可視性などの読み取りは adapter/query のテストが担う）。
func TestShopModerationRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB-backed repository test in short mode")
	}
	ctx := context.Background()
	conn, _ := dbtest.New(t)
	repo := repository.NewShopRepository(conn)

	insertUser := `INSERT INTO users (email, username, password_digest, admin) VALUES ($1, $2, 'x', $3) RETURNING id`
	alice := dbtest.InsertUserRow(ctx, t, conn, insertUser, "alice@example.com", "alice", false)

	insertShop := `INSERT INTO shops (name, status, moderation_note, creator_id, created_at)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`
	// status のコード：0=pending、1=active、2=rejected。
	tNew := time.Date(2024, 2, 1, 12, 0, 0, 0, time.UTC)
	newest := dbtest.InsertUUIDRow(ctx, t, conn, insertShop, "Newest", 0, nil, alice, tNew)

	t.Run("CreateShop は creator 付きの pending な shop を永続化する", func(t *testing.T) {
		submission, err := domain.NewShopSubmission("Fresh Shack", alice, "https://maps.example.com/fresh-shack")
		if err != nil {
			t.Fatalf("NewShopSubmission returned error: %v", err)
		}
		created, err := repo.CreateShop(ctx, submission)
		if err != nil {
			t.Fatalf("CreateShop returned error: %v", err)
		}
		if created.ID == "" || created.Status != domain.ShopStatusPending || created.ModerationNote != nil {
			t.Errorf("created = %+v, want generated id, pending, nil note", created)
		}
		if created.MapURL == nil || *created.MapURL != "https://maps.example.com/fresh-shack" {
			t.Errorf("created.MapURL = %v, want https://maps.example.com/fresh-shack", created.MapURL)
		}
		// 保存された行：pending（0）・note なし・map_url あり・creator は alice。
		stored := readShopRow(ctx, t, conn, created.ID)
		if stored.Name != "Fresh Shack" || stored.Status != 0 || stored.Note != nil {
			t.Errorf("stored = %+v, want Fresh Shack, pending (0), nil note", stored)
		}
		if stored.MapURL == nil || *stored.MapURL != "https://maps.example.com/fresh-shack" {
			t.Errorf("stored.MapURL = %v, want https://maps.example.com/fresh-shack", stored.MapURL)
		}
		if stored.CreatorID == nil || *stored.CreatorID != alice {
			t.Errorf("stored creator_id = %v, want alice %s", stored.CreatorID, alice)
		}
	})

	t.Run("UpdateShopStatus は approve と reject を永続化する", func(t *testing.T) {
		if got := readShopRow(ctx, t, conn, newest); got.Status != 0 {
			t.Fatalf("before = %+v, want pending (0)", got)
		}

		next := domain.Shop{ID: newest, Name: "Newest", Status: domain.ShopStatusPending}.Approve()
		approved, err := repo.UpdateShopStatus(ctx, newest, next.Status, next.ModerationNote)
		if err != nil {
			t.Fatalf("UpdateShopStatus returned error: %v", err)
		}
		if approved.Status != domain.ShopStatusActive || approved.ModerationNote != nil {
			t.Errorf("approved = %+v, want active with nil note", approved)
		}
		if got := readShopRow(ctx, t, conn, newest); got.Status != 1 || got.Note != nil {
			t.Errorf("stored after approve = %+v, want active (1) with nil note", got)
		}

		note := "spam"
		next = approved.Reject(&note)
		rejected, err := repo.UpdateShopStatus(ctx, newest, next.Status, next.ModerationNote)
		if err != nil {
			t.Fatalf("UpdateShopStatus returned error: %v", err)
		}
		if rejected.Status != domain.ShopStatusRejected || rejected.ModerationNote == nil || *rejected.ModerationNote != note {
			t.Errorf("rejected = %+v, want rejected with the note", rejected)
		}
		stored := readShopRow(ctx, t, conn, newest)
		if stored.Status != 2 || stored.Note == nil || *stored.Note != note {
			t.Errorf("stored after reject = %+v, want rejected (2) with the note", stored)
		}
	})

	t.Run("カラム単位の書き込みは並行する更新を巻き戻さない", func(t *testing.T) {
		shop := dbtest.InsertUUIDRow(ctx, t, conn, insertShop, "Race Shack", 0, nil, alice, tNew)

		// lost-update の回帰、方向 1：古い rename 側は、並行する approve の
		// 前にスナップショットを読んだ。旧来の行全体の書き込みは status を
		// pending に戻して approve を巻き戻してしまうが、カラム単位の rename は
		// status と note に手を付けてはならない。
		if got := readShopRow(ctx, t, conn, shop); got.Status != 0 {
			t.Fatalf("snapshot status = %d, want pending (0)", got.Status)
		}
		stale := domain.Shop{ID: shop, Name: "Race Shack", Status: domain.ShopStatusPending}
		next := stale.Approve() // 並行する admin が approve する
		if _, err := repo.UpdateShopStatus(ctx, shop, next.Status, next.ModerationNote); err != nil {
			t.Fatalf("UpdateShopStatus returned error: %v", err)
		}
		renamed, err := repo.UpdateShopName(ctx, shop, "Race Shack Renamed", nil) // 古い rename 側が書き込む
		if err != nil {
			t.Fatalf("UpdateShopName returned error: %v", err)
		}
		if renamed.Name != "Race Shack Renamed" || renamed.Status != domain.ShopStatusActive || renamed.ModerationNote != nil {
			t.Errorf("after stale rename = %+v, want new name AND active with nil note", renamed)
		}

		// 方向 2：並行する rename の前に取得したスナップショットからの status の
		// 書き込みは、新しい name をそのまま保たなければならない。
		note := "spam"
		next = stale.Reject(&note)
		rejected, err := repo.UpdateShopStatus(ctx, shop, next.Status, next.ModerationNote)
		if err != nil {
			t.Fatalf("UpdateShopStatus returned error: %v", err)
		}
		if rejected.Name != "Race Shack Renamed" || rejected.Status != domain.ShopStatusRejected || rejected.ModerationNote == nil || *rejected.ModerationNote != note {
			t.Errorf("after stale status write = %+v, want kept name AND rejected with the note", rejected)
		}

		stored := readShopRow(ctx, t, conn, shop)
		if stored.Name != "Race Shack Renamed" || stored.Status != 2 {
			t.Errorf("stored = %+v, want renamed and rejected (2)", stored)
		}
	})

	t.Run("UpdateShopClosedAt は closed_at だけを永続化し、並行する rename を巻き戻さない", func(t *testing.T) {
		shop := dbtest.InsertUUIDRow(ctx, t, conn, insertShop, "Closable Shack", 1, nil, alice, tNew)

		closedAt := time.Date(2024, 3, 1, 9, 0, 0, 0, time.UTC)
		closed, err := repo.UpdateShopClosedAt(ctx, shop, &closedAt)
		if err != nil {
			t.Fatalf("UpdateShopClosedAt returned error: %v", err)
		}
		if closed.ClosedAt == nil || !closed.ClosedAt.Equal(closedAt) {
			t.Errorf("closed = %+v, want ClosedAt %v", closed, closedAt)
		}
		if got := readShopRow(ctx, t, conn, shop); got.ClosedAt == nil || !got.ClosedAt.Equal(closedAt) {
			t.Errorf("stored after close = %+v, want ClosedAt %v", got, closedAt)
		}

		// 並行する rename は、closed_at には触れない。
		renamed, err := repo.UpdateShopName(ctx, shop, "Closable Shack Renamed", nil)
		if err != nil {
			t.Fatalf("UpdateShopName returned error: %v", err)
		}
		if renamed.ClosedAt == nil || !renamed.ClosedAt.Equal(closedAt) {
			t.Errorf("renamed = %+v, want ClosedAt still %v", renamed, closedAt)
		}

		reopened, err := repo.UpdateShopClosedAt(ctx, shop, nil)
		if err != nil {
			t.Fatalf("UpdateShopClosedAt(nil) returned error: %v", err)
		}
		if reopened.ClosedAt != nil {
			t.Errorf("reopened = %+v, want ClosedAt nil", reopened)
		}
		if got := readShopRow(ctx, t, conn, shop); got.ClosedAt != nil {
			t.Errorf("stored after reopen = %+v, want ClosedAt nil", got)
		}
	})

	t.Run("UpdateShopName は map_url を設定・変更・削除できる", func(t *testing.T) {
		shop := dbtest.InsertUUIDRow(ctx, t, conn, insertShop, "Map Shack", 0, nil, alice, tNew)

		mapURL := "https://maps.example.com/one"
		set, err := repo.UpdateShopName(ctx, shop, "Map Shack", &mapURL)
		if err != nil {
			t.Fatalf("UpdateShopName returned error: %v", err)
		}
		if set.MapURL == nil || *set.MapURL != mapURL {
			t.Errorf("set.MapURL = %v, want %s", set.MapURL, mapURL)
		}

		changed := "https://maps.example.com/two"
		got, err := repo.UpdateShopName(ctx, shop, "Map Shack", &changed)
		if err != nil {
			t.Fatalf("UpdateShopName returned error: %v", err)
		}
		if got.MapURL == nil || *got.MapURL != changed {
			t.Errorf("changed.MapURL = %v, want %s", got.MapURL, changed)
		}

		cleared, err := repo.UpdateShopName(ctx, shop, "Map Shack", nil)
		if err != nil {
			t.Fatalf("UpdateShopName returned error: %v", err)
		}
		if cleared.MapURL != nil {
			t.Errorf("cleared.MapURL = %v, want nil", *cleared.MapURL)
		}
		if stored := readShopRow(ctx, t, conn, shop); stored.MapURL != nil {
			t.Errorf("stored.MapURL = %v, want nil", stored.MapURL)
		}
	})

	t.Run("UpdateShopName に存在しない id を渡すと ErrShopNotFound になる", func(t *testing.T) {
		_, err := repo.UpdateShopName(ctx, uid.N(99999), "x", nil)
		if !errors.Is(err, domain.ErrShopNotFound) {
			t.Fatalf("error = %v, want %v", err, domain.ErrShopNotFound)
		}
	})

	t.Run("UpdateShopStatus に存在しない id を渡すと ErrShopNotFound になる", func(t *testing.T) {
		_, err := repo.UpdateShopStatus(ctx, uid.N(99999), domain.ShopStatusActive, nil)
		if !errors.Is(err, domain.ErrShopNotFound) {
			t.Fatalf("error = %v, want %v", err, domain.ErrShopNotFound)
		}
	})

	t.Run("UpdateShopClosedAt に存在しない id を渡すと ErrShopNotFound になる", func(t *testing.T) {
		_, err := repo.UpdateShopClosedAt(ctx, uid.N(99999), nil)
		if !errors.Is(err, domain.ErrShopNotFound) {
			t.Fatalf("error = %v, want %v", err, domain.ErrShopNotFound)
		}
	})
}
