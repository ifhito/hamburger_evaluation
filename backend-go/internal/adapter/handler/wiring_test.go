package handler_test

import (
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uowtest"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// 以下のヘルパーは、テストの fake（読み取りと書き込みを兼ねる）から、review と user の usecase を
// 組み立てる。トランザクションをまたぐ書き込み（review の書き込みと統計の再計算、退会）は、
// フェイクの UnitOfWork（uowtest.UoW）が、渡された fake の repository を書き込みオブジェクトで包んで
// Tx にする。統計の再計算は、空の facts に対して DB なしで走る（統計そのものの検証は、本物の DB を
// 使うテストが行う）。

func reviewsUsecase(repo *reviewStoreFake, photos usecase.PhotoStorage) *usecase.Reviews {
	return usecase.NewReviews(repo, &uowtest.UoW{Reviews: repo}, usecase.NewBurgerStatsRecalculator(uowtest.Clock{}), photos)
}

func usersUsecase(store *userStoreFake, hasher usecase.PasswordHasher) *usecase.Users {
	return usecase.NewUsers(store, domain.NewUsers(store), &uowtest.UoW{Users: store}, usecase.NewBurgerStatsRecalculator(uowtest.Clock{}), hasher)
}
