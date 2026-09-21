package usecase_test

import (
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// 以下のヘルパーは、テストの fake の repository を domain の書き込みオブジェクトで
// 包んで usecase を組み立てる。usecase は repository に依存せず、書き込みは domain の
// 書き込みオブジェクトを通すため、テストでも同じ形で組み立てる。

func newShops(query usecase.ShopQuery, repo domain.ShopRepository) *usecase.Shops {
	return usecase.NewShops(query, domain.NewShops(repo))
}

func newReviews(query usecase.ReviewQuery, repo domain.ReviewRepository, photos usecase.PhotoStorage) *usecase.Reviews {
	return usecase.NewReviews(query, domain.NewReviews(repo), photos)
}

func newUsers(query usecase.UserQuery, repo domain.UserRepository, hasher usecase.PasswordHasher) *usecase.Users {
	return usecase.NewUsers(query, domain.NewUsers(repo), hasher)
}

func newAuth(query usecase.UserQuery, repo domain.UserRepository, hasher usecase.PasswordHasher, issuer usecase.TokenIssuer, verifier usecase.TokenVerifier) *usecase.Auth {
	return usecase.NewAuth(query, domain.NewUsers(repo), hasher, issuer, verifier)
}
