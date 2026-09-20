// Command seed は開発用 database に idempotent な fixture データを投入する
// （issue #17 R5）。admin ユーザー 1 人、一般ユーザー 3 人、承認済みの shop
// 3 件と pending の shop 1 件、承認済みの shop ごとに burger 2 件、そして
// 固定された review 群である。その後、アプリが使うのと同じ domain の
// calculator で burger_stats を再計算するので、GET のレスポンスは整合する。
//
// すべての fixture のパスワードは "Password123!" である（signup と同じ強度ルールを
// 満たす）。これらはよく知られた開発用 fixture であり、secret ではない。
// docker compose で実行する：
//
//	docker compose run --rm migrate up
//	docker compose run --rm seed
//
// 再実行しても安全である。ユーザーは email、shop は name、burger は
// (shop, name)、review は (user, burger) をキーとするので、何も重複しない。
// seed 全体は 1 つの transaction で実行され、いかなるエラーでも fail-loud
// する（終了コードは 0 以外）。
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// devPassword は、すべての fixture ユーザーに共通のパスワードである
// （開発専用）。domain.ValidatePassword を満たす値でなければならない
// （main_test.go が固定する）。
const devPassword = "Password123!"

func main() {
	if err := run(context.Background()); err != nil {
		log.Fatalf("seed: %v", err)
	}
	fmt.Println("seed: done")
}

func run(ctx context.Context) error {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return errors.New("DATABASE_URL is not set")
	}

	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer conn.Close(ctx)

	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // commit 後は no-op になる

	if err := seed(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// seed は、与えられた transaction の中ですべての fixture を挿入する。
// review は（Rails の seeds のサンプリングとは異なり）決定的に割り当てられる
// ので、繰り返し実行しても同じ状態に収束する。
func seed(ctx context.Context, tx pgx.Tx) error {
	admin, err := seedUser(ctx, tx, "admin@example.com", "admin", true)
	if err != nil {
		return err
	}
	alice, err := seedUser(ctx, tx, "alice@example.com", "alice", false)
	if err != nil {
		return err
	}
	bob, err := seedUser(ctx, tx, "bob@example.com", "bob", false)
	if err != nil {
		return err
	}
	charlie, err := seedUser(ctx, tx, "charlie@example.com", "charlie", false)
	if err != nil {
		return err
	}

	// shop の status のエンコーディングは db/migrations/000002 に対応する：
	// 0=pending, 1=active。
	shakeShack, err := seedShop(ctx, tx, "Shake Shack 渋谷", 1, admin)
	if err != nil {
		return err
	}
	jsBurgers, err := seedShop(ctx, tx, "J.S. BURGERS CAFE 新宿", 1, admin)
	if err != nil {
		return err
	}
	freshness, err := seedShop(ctx, tx, "フレッシュネスバーガー 原宿", 1, admin)
	if err != nil {
		return err
	}
	// moderation のフローを試すための pending の shop 1 件。burger は持たない。
	if _, err := seedShop(ctx, tx, "バーガースタンド 下北沢（審査待ち）", 0, alice); err != nil {
		return err
	}

	type burgerSpec struct {
		shopID int64
		name   string
	}
	specs := []burgerSpec{
		{shakeShack, "クラシックバーガー"},
		{shakeShack, "チーズバーガー"},
		{jsBurgers, "クラシックバーガー"},
		{jsBurgers, "アボカドバーガー"},
		{freshness, "クラシックバーガー"},
		{freshness, "テリヤキバーガー"},
	}
	burgers := make([]int64, len(specs))
	for i, spec := range specs {
		id, err := seedBurger(ctx, tx, spec.shopID, spec.name)
		if err != nil {
			return err
		}
		burgers[i] = id
	}

	type reviewSpec struct {
		userID  int64
		burger  int // burgers へのインデックス
		rating  int16
		comment string
	}
	reviews := []reviewSpec{
		{alice, 0, 5, "肉汁たっぷりで最高でした！"},
		{alice, 2, 4, "バンズがふわふわで美味しかった。"},
		{alice, 5, 4, "チーズの量がちょうどよかった。"},
		{bob, 0, 4, "また絶対行きたいです。"},
		{bob, 1, 5, "ボリューム満点でコスパ良し。"},
		{bob, 3, 3, "ちょっとしょっぱかったけど美味しい。"},
		{charlie, 2, 5, "肉汁たっぷりで最高でした！"},
		{charlie, 4, 4, "バンズがふわふわで美味しかった。"},
		{charlie, 5, 5, "また絶対行きたいです。"},
	}
	for _, spec := range reviews {
		if err := seedReview(ctx, tx, spec.userID, burgers[spec.burger], spec.rating, spec.comment); err != nil {
			return err
		}
	}

	// burger の id の昇順。review repository における複数 burger の再計算の
	// 規約に合わせている。
	for _, burgerID := range burgers {
		if err := recalculateBurgerStats(ctx, tx, burgerID); err != nil {
			return err
		}
	}
	return nil
}

// seedUser は email でユーザーを探し、なければ、signup とまったく同じように
// ハッシュ化した共通の開発用パスワードでユーザーを作成する（デフォルトの cost
// での bcrypt。internal/adapter/infra/password.go を参照）。
func seedUser(ctx context.Context, tx pgx.Tx, email, username string, admin bool) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `SELECT id FROM users WHERE email = $1`, email).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("find user %s: %w", email, err)
	}
	digest, err := bcrypt.GenerateFromPassword([]byte(devPassword), bcrypt.DefaultCost)
	if err != nil {
		return 0, fmt.Errorf("hash password for %s: %w", email, err)
	}
	err = tx.QueryRow(ctx,
		`INSERT INTO users (email, username, password_digest, admin) VALUES ($1, $2, $3, $4) RETURNING id`,
		email, username, string(digest), admin,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert user %s: %w", email, err)
	}
	return id, nil
}

// seedShop は name で shop を探し（seed の natural key である。schema には
// これに対する unique 制約がない）、なければ与えられた status で作成する。
func seedShop(ctx context.Context, tx pgx.Tx, name string, status int16, creatorID int64) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `SELECT id FROM shops WHERE name = $1`, name).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("find shop %s: %w", name, err)
	}
	err = tx.QueryRow(ctx,
		`INSERT INTO shops (name, status, creator_id) VALUES ($1, $2, $3) RETURNING id`,
		name, status, creatorID,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert shop %s: %w", name, err)
	}
	return id, nil
}

// seedBurger は shops_burgers を経由して name で shop の burger を探し
// （CreateReviewForNamedBurger が使うのと同じ shop ごとの natural key）、
// なければ burger とその link を作成する。
func seedBurger(ctx context.Context, tx pgx.Tx, shopID int64, name string) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx,
		`SELECT b.id FROM burgers b
		 JOIN shops_burgers sb ON sb.burger_id = b.id
		 WHERE sb.shop_id = $1 AND b.name = $2`,
		shopID, name,
	).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("find burger %s of shop %d: %w", name, shopID, err)
	}
	err = tx.QueryRow(ctx, `INSERT INTO burgers (name) VALUES ($1) RETURNING id`, name).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert burger %s: %w", name, err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO shops_burgers (shop_id, burger_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		shopID, id,
	); err != nil {
		return 0, fmt.Errorf("link burger %d to shop %d: %w", id, shopID, err)
	}
	return id, nil
}

// seedReview は、ユーザーがその burger に対する kept な review をすでに持って
// いない限り review を挿入する（seed の natural key。アプリ自体は複数件を
// 許可する）。
func seedReview(ctx context.Context, tx pgx.Tx, userID, burgerID int64, rating int16, comment string) error {
	var exists bool
	err := tx.QueryRow(ctx,
		`SELECT EXISTS (
		   SELECT 1 FROM reviews
		   WHERE user_id = $1 AND burger_id = $2 AND discarded_at IS NULL
		 )`,
		userID, burgerID,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("find review (user %d, burger %d): %w", userID, burgerID, err)
	}
	if exists {
		return nil
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO reviews (rating, comment, user_id, burger_id) VALUES ($1, $2, $3, $4)`,
		rating, comment, userID, burgerID,
	); err != nil {
		return fmt.Errorf("insert review (user %d, burger %d): %w", userID, burgerID, err)
	}
	return nil
}

// recalculateBurgerStats は、1 つの burger の stats 行を、その burger の kept な
// review のうち author（user）が discard されていないものから再計算して
// upsert する。repository の recalculateBurgerStats を再現して
// おり、db/queries/burger_stats.sql と同じ SQL、同じ純粋な domain の
// calculator を使うので、seed された stats はアプリが保存するものと一致する。
// FOR UPDATE ロックはない。seed は 1 回限りのツールで、同時に書き込むものが
// いないからである。
func recalculateBurgerStats(ctx context.Context, tx pgx.Tx, burgerID int64) error {
	rows, err := tx.Query(ctx,
		`SELECT r.rating, r.created_at, r.user_id
		 FROM reviews r
		 JOIN users u ON u.id = r.user_id
		 WHERE r.burger_id = $1 AND r.discarded_at IS NULL AND u.discarded_at IS NULL
		 ORDER BY r.id`,
		burgerID,
	)
	if err != nil {
		return fmt.Errorf("list facts for burger %d: %w", burgerID, err)
	}
	type factRow struct {
		rating    int16
		createdAt time.Time
		userID    int64
	}
	var factRows []factRow
	for rows.Next() {
		var row factRow
		if err := rows.Scan(&row.rating, &row.createdAt, &row.userID); err != nil {
			rows.Close()
			return fmt.Errorf("scan fact for burger %d: %w", burgerID, err)
		}
		factRows = append(factRows, row)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("list facts for burger %d: %w", burgerID, err)
	}

	// Reviewer-trust の履歴：各 fact の author がすべての burger にわたって
	// つけた kept な rating を、ユーザーごとにまとめたもの。
	historyByUser := make(map[int64][]float64, len(factRows))
	userIDs := make([]int64, 0, len(factRows))
	for _, row := range factRows {
		if _, seen := historyByUser[row.userID]; !seen {
			historyByUser[row.userID] = nil
			userIDs = append(userIDs, row.userID)
		}
	}
	if len(userIDs) > 0 {
		ratingRows, err := tx.Query(ctx,
			`SELECT r.user_id, r.rating FROM reviews r
			 WHERE r.user_id = ANY($1::bigint[]) AND r.discarded_at IS NULL
			 ORDER BY r.id`,
			userIDs,
		)
		if err != nil {
			return fmt.Errorf("list reviewer ratings: %w", err)
		}
		for ratingRows.Next() {
			var userID int64
			var rating int16
			if err := ratingRows.Scan(&userID, &rating); err != nil {
				ratingRows.Close()
				return fmt.Errorf("scan reviewer rating: %w", err)
			}
			historyByUser[userID] = append(historyByUser[userID], float64(rating))
		}
		ratingRows.Close()
		if err := ratingRows.Err(); err != nil {
			return fmt.Errorf("list reviewer ratings: %w", err)
		}
	}

	facts := make([]domain.ReviewFact, 0, len(factRows))
	for _, row := range factRows {
		facts = append(facts, domain.ReviewFact{
			Rating:          float64(row.rating),
			CreatedAt:       row.createdAt,
			ReviewerHistory: domain.ReviewerHistory{Ratings: historyByUser[row.userID]},
		})
	}
	now := time.Now().Truncate(time.Microsecond)
	score := domain.CalculateBurgerScore(facts, now)
	if _, err := tx.Exec(ctx,
		`INSERT INTO burger_stats (burger_id, review_count, average_rating, weighted_score, confidence, calculated_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (burger_id) DO UPDATE
		 SET review_count = EXCLUDED.review_count,
		     average_rating = EXCLUDED.average_rating,
		     weighted_score = EXCLUDED.weighted_score,
		     confidence = EXCLUDED.confidence,
		     calculated_at = EXCLUDED.calculated_at`,
		burgerID, int64(len(facts)), domain.AverageRating(facts), score.WeightedAverage, score.Confidence, now,
	); err != nil {
		return fmt.Errorf("upsert stats for burger %d: %w", burgerID, err)
	}
	return nil
}
