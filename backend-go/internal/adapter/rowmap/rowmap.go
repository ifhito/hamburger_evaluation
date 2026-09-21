// Package rowmap は、sqlc の行（sqlcgen）と domain のエンティティとの間の
// 写像を 1 か所にまとめる。読み取りの adapter/query と書き込みの
// adapter/repository の両方が同じ写像を使うので、片方だけがずれることを防ぐ。
//
// import してよいのは sqlcgen、domain、pgx（pgtype）だけである。adapter/query や
// adapter/repository を import してはならない（循環になる）。
package rowmap

import (
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/repository/sqlcgen"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// ShopStatusCode は domain.ShopStatus を smallint の保存用コードにエンコードする。
// Shop の逆である。この対応づけが adapter の外に出ることはない。
func ShopStatusCode(status domain.ShopStatus) (int16, error) {
	switch status {
	case domain.ShopStatusPending:
		return 0, nil
	case domain.ShopStatusActive:
		return 1, nil
	case domain.ShopStatusRejected:
		return 2, nil
	default:
		// 呼び出し側が domain の定数からしか status を作らない限り到達
		// しない。ゴミを永続化する代わりに fail loudly する。
		return 0, fmt.Errorf("unknown shop status %q", status)
	}
}

// Shop は sqlc の shop のカラムを domain のエンティティに変換し、
// smallint の status をデコードする（0=pending、1=active、2=rejected）。
func Shop(id int64, name string, status int16, note pgtype.Text, creatorID *string) (domain.Shop, error) {
	shop := domain.Shop{ID: id, Name: name}
	switch status {
	case 0:
		shop.Status = domain.ShopStatusPending
	case 1:
		shop.Status = domain.ShopStatusActive
	case 2:
		shop.Status = domain.ShopStatusRejected
	default:
		// CHECK 制約が保たれている限り到達しない。保たれていなければ
		// fail loudly する。
		return domain.Shop{}, fmt.Errorf("shop %d: unknown status code %d", id, status)
	}
	if note.Valid {
		n := note.String
		shop.ModerationNote = &n
	}
	shop.CreatorID = creatorID
	return shop, nil
}

// ShopReviewBurger は、stats つき burger の sqlc のカラムを domain の
// ペイロードに変換する。stats のカラムは LEFT JOIN 由来で NULL になりうる。
func ShopReviewBurger(id int64, name string, averageRating pgtype.Float8, reviewCount pgtype.Int8, weightedScore, confidence pgtype.Float8) domain.ShopReviewBurger {
	return domain.ShopReviewBurger{
		ID:   id,
		Name: name,
		// stats 行がまだ存在しないことがある。その場合はゼロ値になる。
		AverageRating: averageRating.Float64,
		ReviewCount:   reviewCount.Int64,
		WeightedScore: weightedScore.Float64,
		Confidence:    confidence.Float64,
	}
}

// User は sqlc の行を domain のエンティティに変換し、password digest
// と、ストレージ専用のカラムを落とす。
func User(row sqlcgen.User) domain.User {
	return domain.User{
		ID:       row.ID,
		Username: row.Username,
		Bio:      row.Bio,
		Email:    row.Email,
		Admin:    row.Admin,
	}
}
