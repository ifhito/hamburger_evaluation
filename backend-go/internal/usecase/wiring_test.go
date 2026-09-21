package usecase_test

import (
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uowtest"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// 以下のヘルパーは、テストの fake の repository を domain の書き込みオブジェクトで
// 包んで usecase を組み立てる。usecase は repository に依存せず、書き込みは domain の
// 書き込みオブジェクトを通すため、テストでも同じ形で組み立てる。レビューの書き込みと統計の
// 再計算、ユーザーの退会と統計の再計算のように、1 つのトランザクションで行う書き込みは、
// UnitOfWork(ここからここまでをまとめて 1 つのトランザクションにする範囲を、usecase が指定する
// 仕組み)の代役(uowtest.UoW)が、渡した repository の代役を書き込みオブジェクトで包んで、
// トランザクションの代わりをする。

func newShops(query usecase.ShopQuery, repo domain.ShopRepository) *usecase.Shops {
	return usecase.NewShops(query, domain.NewShops(repo))
}

func newReviews(query usecase.ReviewQuery, repo domain.ReviewRepository, photos usecase.PhotoStorage) *usecase.Reviews {
	return usecase.NewReviews(query, &uowtest.UoW{Reviews: repo}, usecase.NewBurgerStatsRecalculator(uowtest.Clock{}), photos)
}

func newUsers(query usecase.UserQuery, repo domain.UserRepository, hasher usecase.PasswordHasher) *usecase.Users {
	return usecase.NewUsers(query, domain.NewUsers(repo), &uowtest.UoW{Users: repo}, usecase.NewBurgerStatsRecalculator(uowtest.Clock{}), hasher)
}

func newAuth(query usecase.UserQuery, hasher usecase.PasswordHasher, issuer usecase.TokenIssuer, verifier usecase.TokenVerifier) *usecase.Auth {
	return usecase.NewAuth(query, hasher, issuer, verifier)
}

func newSignups(query usecase.UserQuery, repo domain.SignupVerificationRepository, hasher usecase.PasswordHasher, mailer usecase.Mailer, issuer usecase.TokenIssuer, cfg usecase.SignupConfig) *usecase.Signups {
	return usecase.NewSignups(query, domain.NewSignupVerifications(repo), hasher, mailer, issuer, cfg)
}
