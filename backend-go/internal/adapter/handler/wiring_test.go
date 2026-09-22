package handler_test

import (
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/storage"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uowtest"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// 以下のヘルパーは、テスト用の代役(フェイク)から、レビューとユーザーの usecase を組み立てる。
// 渡す代役は、読み取り(Query)と書き込み(repository)の両方を兼ねている。
//
// レビューの書き込みと統計の再計算、ユーザーの退会と統計の再計算は、1 つのトランザクションで
// 行う必要がある。そのためにトランザクションの範囲を指定する仕組みが UnitOfWork(ここからここまでを
// まとめて 1 つのトランザクションにする範囲を、usecase が指定する仕組み)である。このテストでは、
// 代役の UnitOfWork(uowtest.UoW)が、渡した代役の repository を domain の書き込みオブジェクトで
// 包んで、そのトランザクションの代わりをする。統計の再計算は、
// 統計の元データが空のまま、データベースなしで最後まで走る。統計の値そのものは、本物の
// データベースを使うテスト(adapter/uow など)で確かめている。

// shopsUsecase は、ショップの use case を組み立てる。写真の保存先(キーを公開 URL に直すだけ)は、
// ファイルに触れない代役(ルートなしの disk)である。
func shopsUsecase(query usecase.ShopQuery, repo domain.ShopRepository) *usecase.Shops {
	return usecase.NewShops(query, domain.NewShops(repo), storage.NewDisk("", "/photos"))
}

func reviewsUsecase(repo *reviewStoreFake, photos usecase.PhotoStorage) *usecase.Reviews {
	return usecase.NewReviews(repo, &uowtest.UoW{Reviews: repo}, usecase.NewBurgerStatsRecalculator(uowtest.Clock{}), usecase.NewShopStatsRecalculator(uowtest.Clock{}), photos)
}

func usersUsecase(store *userStoreFake, hasher usecase.PasswordHasher) *usecase.Users {
	return usecase.NewUsers(store, domain.NewUsers(store), &uowtest.UoW{Users: store}, usecase.NewBurgerStatsRecalculator(uowtest.Clock{}), usecase.NewShopStatsRecalculator(uowtest.Clock{}), hasher)
}
