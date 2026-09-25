package usecase_test

import (
	"context"
	"errors"
	"io"
	"reflect"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/testutil/uid"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// fakeBurgerQuery は、手書きの usecase.BurgerQuery の test double である。
type fakeBurgerQuery struct {
	detail    domain.BurgerDetail
	detailErr error
	shops     []domain.Shop

	rankings              []domain.BurgerRanking
	rankingsHasMore       bool
	rankingsErr           error
	lastLimit, lastOffset int32
}

func (f *fakeBurgerQuery) GetBurgerWithStats(context.Context, string) (domain.BurgerDetail, error) {
	if f.detailErr != nil {
		return domain.BurgerDetail{}, f.detailErr
	}
	return f.detail, nil
}

func (f *fakeBurgerQuery) ListBurgerShops(context.Context, string) ([]domain.Shop, error) {
	return f.shops, nil
}

func (f *fakeBurgerQuery) ListBurgerRankings(_ context.Context, _ usecase.BurgerListFilter, limit, offset int32) ([]domain.BurgerRanking, bool, error) {
	f.lastLimit, f.lastOffset = limit, offset
	if f.rankingsErr != nil {
		return nil, false, f.rankingsErr
	}
	return f.rankings, f.rankingsHasMore, nil
}

var _ usecase.BurgerQuery = (*fakeBurgerQuery)(nil)

// TestBurgersGetNotFound は、存在しない burger の id で domain.ErrBurgerNotFound が
// そのまま(wrap されて)伝播することを確かめる。
func TestBurgersGetNotFound(t *testing.T) {
	query := &fakeBurgerQuery{detailErr: domain.ErrBurgerNotFound}
	_, err := usecase.NewBurgers(query, stubPhotoURLs{}).Get(context.Background(), nil, uid.N(1))
	if !errors.Is(err, domain.ErrBurgerNotFound) {
		t.Fatalf("error = %v, want %v", err, domain.ErrBurgerNotFound)
	}
}

// TestBurgersGetShopVisibility は、Shops が viewer ごとの可視性(domain.ShopVisibility)で
// 絞り込まれることを確かめる：active な shop は誰にでも見え、pending な shop はその creator と
// admin にだけ見え、匿名や無関係な他人には見えない。
func TestBurgersGetShopVisibility(t *testing.T) {
	creatorID := uid.N(1)
	otherID := uid.N(2)
	activeShop := domain.Shop{ID: uid.N(10), Name: "Active Diner", Status: domain.ShopStatusActive}
	pendingShop := domain.Shop{ID: uid.N(11), Name: "Pending Diner", Status: domain.ShopStatusPending, CreatorID: &creatorID}
	burgerID := uid.N(20)

	newQuery := func() *fakeBurgerQuery {
		return &fakeBurgerQuery{
			detail: domain.BurgerDetail{ID: burgerID, Name: "Cheese"},
			shops:  []domain.Shop{activeShop, pendingShop},
		}
	}

	tests := []struct {
		name   string
		viewer *domain.User
		want   []domain.ShopRef
	}{
		{
			name:   "匿名の viewer には active な shop だけが見える",
			viewer: nil,
			want:   []domain.ShopRef{{ID: activeShop.ID, Name: activeShop.Name}},
		},
		{
			name:   "無関係な他人には active な shop だけが見える",
			viewer: &domain.User{ID: otherID},
			want:   []domain.ShopRef{{ID: activeShop.ID, Name: activeShop.Name}},
		},
		{
			name:   "pending な shop の creator には、pending な shop も追加で見える",
			viewer: &domain.User{ID: creatorID},
			want:   []domain.ShopRef{{ID: activeShop.ID, Name: activeShop.Name}, {ID: pendingShop.ID, Name: pendingShop.Name}},
		},
		{
			name:   "admin には、creator でなくても pending な shop が見える",
			viewer: &domain.User{ID: otherID, Admin: true},
			want:   []domain.ShopRef{{ID: activeShop.ID, Name: activeShop.Name}, {ID: pendingShop.ID, Name: pendingShop.Name}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detail, err := usecase.NewBurgers(newQuery(), stubPhotoURLs{}).Get(context.Background(), tt.viewer, burgerID)
			if err != nil {
				t.Fatalf("Get returned error: %v", err)
			}
			if !reflect.DeepEqual(detail.Shops, tt.want) {
				t.Errorf("Shops = %+v, want %+v", detail.Shops, tt.want)
			}
		})
	}
}

// TestBurgersListPagination は、範囲外の page/perPage が clampPage の規則で補正されて
// query に渡ることを確かめる(usecase.Shops の TestShopsListPagination と同じ形)。
func TestBurgersListPagination(t *testing.T) {
	tests := []struct {
		name                  string
		page, perPage         int
		wantLimit, wantOffset int32
	}{
		{name: "既定値の範囲内はそのまま", page: 2, perPage: 10, wantLimit: 10, wantOffset: 10},
		{name: "page が 1 未満なら 1 に補正", page: 0, perPage: 10, wantLimit: 10, wantOffset: 0},
		{name: "perPage が 1 未満なら既定の 20 に補正", page: 1, perPage: 0, wantLimit: 20, wantOffset: 0},
		{name: "perPage が上限を超えたら 100 に補正", page: 1, perPage: 500, wantLimit: 100, wantOffset: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := &fakeBurgerQuery{}
			if _, _, err := usecase.NewBurgers(query, stubPhotoURLs{}).List(context.Background(), usecase.BurgerListFilter{}, tt.page, tt.perPage); err != nil {
				t.Fatalf("List returned error: %v", err)
			}
			if query.lastLimit != tt.wantLimit || query.lastOffset != tt.wantOffset {
				t.Errorf("limit/offset = %d/%d, want %d/%d", query.lastLimit, query.lastOffset, tt.wantLimit, tt.wantOffset)
			}
		})
	}
}

// TestBurgersListError は、query の失敗が wrap されてそのまま伝播することを確かめる。
func TestBurgersListError(t *testing.T) {
	query := &fakeBurgerQuery{rankingsErr: io.ErrUnexpectedEOF}
	_, _, err := usecase.NewBurgers(query, stubPhotoURLs{}).List(context.Background(), usecase.BurgerListFilter{}, 1, 20)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("error = %v, want wrapping %v", err, io.ErrUnexpectedEOF)
	}
}

// TestBurgersPhotos は、代表写真の URL と閲覧できない店舗の写真の非表示を検証する。
func TestBurgersPhotos(t *testing.T) {
	key := "reviews/burger.jpg"
	creator := uid.N(1)
	for _, tt := range []struct {
		name   string
		status domain.ShopStatus
		viewer *domain.User
		want   bool
	}{
		{"公開店舗の写真は匿名でも表示する", domain.ShopStatusActive, nil, true},
		{"非公開店舗しかない場合は匿名に写真を返さない", domain.ShopStatusPending, nil, false},
		{"非公開店舗の申請者には写真を表示する", domain.ShopStatusPending, &domain.User{ID: creator}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			q := &fakeBurgerQuery{detail: domain.BurgerDetail{ID: uid.N(2), PhotoKey: &key}, shops: []domain.Shop{{ID: uid.N(3), Status: tt.status, CreatorID: &creator}}}
			got, err := usecase.NewBurgers(q, stubPhotoURLs{}).Get(context.Background(), tt.viewer, uid.N(2))
			if err != nil {
				t.Fatal(err)
			}
			if tt.want {
				if got.PhotoURL == nil || *got.PhotoURL != "https://photos.test/reviews/burger.jpg" {
					t.Fatalf("photo = %v", got.PhotoURL)
				}
			} else if got.PhotoURL != nil {
				t.Fatalf("hidden photo = %v", *got.PhotoURL)
			}
		})
	}
	q := &fakeBurgerQuery{rankings: []domain.BurgerRanking{{PhotoKey: &key}, {}}}
	got, _, err := usecase.NewBurgers(q, stubPhotoURLs{}).List(context.Background(), usecase.BurgerListFilter{}, 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].PhotoURL == nil || *got[0].PhotoURL != "https://photos.test/reviews/burger.jpg" || got[1].PhotoURL != nil {
		t.Fatalf("photos = %+v", got)
	}
}
