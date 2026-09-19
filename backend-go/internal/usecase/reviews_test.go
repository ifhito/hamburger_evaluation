package usecase_test

import (
	"context"
	"errors"
	"io"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// fakeReviewRepo is a hand-written usecase.ReviewRepository test double.
// Unset behaviors panic so tests fail loudly on unexpected calls —
// especially writes outside the tested flow.
type fakeReviewRepo struct {
	listReviews                func(ctx context.Context, filter usecase.ReviewListFilter, limit, offset int32) ([]domain.ReviewDetail, error)
	getReview                  func(ctx context.Context, id int64) (domain.ReviewDetail, error)
	getShop                    func(ctx context.Context, id int64) (domain.Shop, error)
	getShopBurger              func(ctx context.Context, shopID, burgerID int64) (domain.ShopReviewBurger, error)
	createReview               func(ctx context.Context, review domain.Review) (domain.Review, error)
	createReviewForNamedBurger func(ctx context.Context, shopID int64, burgerName string, review domain.Review) (domain.Review, domain.ShopReviewBurger, error)
	updateReviewContent        func(ctx context.Context, id int64, rating int, comment string) (domain.Review, error)
	discardReview              func(ctx context.Context, id int64) error
}

func (f *fakeReviewRepo) ListReviews(ctx context.Context, filter usecase.ReviewListFilter, limit, offset int32) ([]domain.ReviewDetail, error) {
	if f.listReviews == nil {
		panic("unexpected ListReviews call")
	}
	return f.listReviews(ctx, filter, limit, offset)
}

func (f *fakeReviewRepo) GetReview(ctx context.Context, id int64) (domain.ReviewDetail, error) {
	if f.getReview == nil {
		panic("unexpected GetReview call")
	}
	return f.getReview(ctx, id)
}

func (f *fakeReviewRepo) GetShop(ctx context.Context, id int64) (domain.Shop, error) {
	if f.getShop == nil {
		panic("unexpected GetShop call")
	}
	return f.getShop(ctx, id)
}

func (f *fakeReviewRepo) GetShopBurger(ctx context.Context, shopID, burgerID int64) (domain.ShopReviewBurger, error) {
	if f.getShopBurger == nil {
		panic("unexpected GetShopBurger call")
	}
	return f.getShopBurger(ctx, shopID, burgerID)
}

func (f *fakeReviewRepo) CreateReview(ctx context.Context, review domain.Review) (domain.Review, error) {
	if f.createReview == nil {
		panic("unexpected CreateReview call")
	}
	return f.createReview(ctx, review)
}

func (f *fakeReviewRepo) CreateReviewForNamedBurger(ctx context.Context, shopID int64, burgerName string, review domain.Review) (domain.Review, domain.ShopReviewBurger, error) {
	if f.createReviewForNamedBurger == nil {
		panic("unexpected CreateReviewForNamedBurger call")
	}
	return f.createReviewForNamedBurger(ctx, shopID, burgerName, review)
}

func (f *fakeReviewRepo) UpdateReviewContent(ctx context.Context, id int64, rating int, comment string) (domain.Review, error) {
	if f.updateReviewContent == nil {
		panic("unexpected UpdateReviewContent call")
	}
	return f.updateReviewContent(ctx, id, rating, comment)
}

func (f *fakeReviewRepo) DiscardReview(ctx context.Context, id int64) error {
	if f.discardReview == nil {
		panic("unexpected DiscardReview call")
	}
	return f.discardReview(ctx, id)
}

// TestReviewsListPagination pins the same fallback rules as Shops.List:
// page defaults to 1, per_page to 20, per_page is capped at 100, huge
// pages clamp the offset instead of overflowing.
func TestReviewsListPagination(t *testing.T) {
	tests := []struct {
		name          string
		page, perPage int
		wantLimit     int32
		wantOffset    int32
	}{
		{name: "defaults", page: 0, perPage: 0, wantLimit: 20, wantOffset: 0},
		{name: "negative values fall back", page: -3, perPage: -1, wantLimit: 20, wantOffset: 0},
		{name: "explicit page and per_page", page: 3, perPage: 5, wantLimit: 5, wantOffset: 10},
		{name: "per_page above 100 is clamped", page: 1, perPage: 101, wantLimit: 100, wantOffset: 0},
		{name: "huge page clamps offset instead of overflowing", page: 1 << 40, perPage: 100, wantLimit: 100, wantOffset: 1<<31 - 1},
		{name: "page MaxInt64 with default per_page clamps offset", page: math.MaxInt64, perPage: 0, wantLimit: 20, wantOffset: math.MaxInt32},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotLimit, gotOffset int32
			repo := &fakeReviewRepo{
				listReviews: func(_ context.Context, _ usecase.ReviewListFilter, limit, offset int32) ([]domain.ReviewDetail, error) {
					gotLimit, gotOffset = limit, offset
					return []domain.ReviewDetail{}, nil
				},
			}
			if _, err := usecase.NewReviews(repo).List(context.Background(), usecase.ReviewListFilter{}, tt.page, tt.perPage); err != nil {
				t.Fatalf("List returned error: %v", err)
			}
			if gotLimit != tt.wantLimit || gotOffset != tt.wantOffset {
				t.Errorf("limit/offset = %d/%d, want %d/%d", gotLimit, gotOffset, tt.wantLimit, tt.wantOffset)
			}
		})
	}
}

// TestReviewsListFilterPassThrough pins that List hands the filter to the
// repository untouched: the usecase neither normalizes nor validates the
// filter values (the handler decides presence, the SQL does the matching).
func TestReviewsListFilterPassThrough(t *testing.T) {
	rating := 4
	shopID := int64(7)
	want := usecase.ReviewListFilter{Rating: &rating, Keyword: "tasty", ShopID: &shopID}
	var got usecase.ReviewListFilter
	repo := &fakeReviewRepo{
		listReviews: func(_ context.Context, filter usecase.ReviewListFilter, _, _ int32) ([]domain.ReviewDetail, error) {
			got = filter
			return []domain.ReviewDetail{}, nil
		},
	}
	if _, err := usecase.NewReviews(repo).List(context.Background(), want, 1, 20); err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("filter = %+v, want %+v", got, want)
	}
}

// TestReviewsListFailure: a repository failure propagates wrapped.
func TestReviewsListFailure(t *testing.T) {
	repo := &fakeReviewRepo{
		listReviews: func(_ context.Context, _ usecase.ReviewListFilter, _, _ int32) ([]domain.ReviewDetail, error) {
			return nil, io.ErrUnexpectedEOF
		},
	}
	if _, err := usecase.NewReviews(repo).List(context.Background(), usecase.ReviewListFilter{}, 1, 20); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("List error = %v, want %v", err, io.ErrUnexpectedEOF)
	}
}

// TestReviewsGet covers the detail use case: found reviews pass through,
// missing/discarded ones surface domain.ErrReviewNotFound (issue #14 AC6
// at the usecase level).
func TestReviewsGet(t *testing.T) {
	detail := domain.ReviewDetail{
		Review: domain.Review{ID: 9, Rating: 4, AuthorID: 1, BurgerID: 5, CreatedAt: time.Now()},
		User:   &domain.UserRef{ID: 1, Username: "alice"},
		Burger: &domain.ShopReviewBurger{ID: 5, Name: "Cheese"},
	}
	repo := &fakeReviewRepo{
		getReview: func(_ context.Context, id int64) (domain.ReviewDetail, error) {
			if id == detail.ID {
				return detail, nil
			}
			return domain.ReviewDetail{}, domain.ErrReviewNotFound
		},
	}
	reviews := usecase.NewReviews(repo)

	got, err := reviews.Get(context.Background(), detail.ID)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if !reflect.DeepEqual(got, detail) {
		t.Errorf("Get = %+v, want %+v", got, detail)
	}

	if _, err := reviews.Get(context.Background(), 999); !errors.Is(err, domain.ErrReviewNotFound) {
		t.Fatalf("Get error = %v, want %v", err, domain.ErrReviewNotFound)
	}
}

// TestReviewsCreate covers the submission flow and its exact check order:
// shop 404 → reviewable 403 → burger 404 → validation 422 → insert. The
// fake's unset behaviors turn out-of-order calls into panics.
func TestReviewsCreate(t *testing.T) {
	alice := domain.User{ID: 1, Username: "alice"}
	bob := domain.User{ID: 2, Username: "bob"}
	admin := domain.User{ID: 3, Username: "root", Admin: true}
	ctx := context.Background()

	activeShop := domain.Shop{ID: 10, Status: domain.ShopStatusActive}
	pendingShop := domain.Shop{ID: 11, Status: domain.ShopStatusPending, CreatorID: int64Ptr(alice.ID)}
	rejectedShop := domain.Shop{ID: 12, Status: domain.ShopStatusRejected, CreatorID: int64Ptr(alice.ID)}
	cheese := domain.ShopReviewBurger{ID: 5, Name: "Cheese", AverageRating: 4.5, ReviewCount: 2, WeightedScore: 4.1, Confidence: 0.8}

	getShop := func(_ context.Context, id int64) (domain.Shop, error) {
		for _, shop := range []domain.Shop{activeShop, pendingShop, rejectedShop} {
			if shop.ID == id {
				return shop, nil
			}
		}
		return domain.Shop{}, domain.ErrShopNotFound
	}
	getShopBurger := func(_ context.Context, shopID, burgerID int64) (domain.ShopReviewBurger, error) {
		if burgerID == cheese.ID {
			return cheese, nil
		}
		return domain.ShopReviewBurger{}, domain.ErrBurgerNotFound
	}

	t.Run("AC1 authenticated post to active shop inserts and composes the payload", func(t *testing.T) {
		var inserted domain.Review
		repo := &fakeReviewRepo{
			getShop:       getShop,
			getShopBurger: getShopBurger,
			createReview: func(_ context.Context, review domain.Review) (domain.Review, error) {
				inserted = review
				review.ID = 42
				review.CreatedAt = time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)
				return review, nil
			},
		}
		got, err := usecase.NewReviews(repo).Create(ctx, bob, activeShop.ID, cheese.ID, "", 4, "Tasty")
		if err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
		if inserted.AuthorID != bob.ID || inserted.BurgerID != cheese.ID || inserted.Rating != 4 {
			t.Errorf("inserted = %+v, want author %d, burger %d, rating 4", inserted, bob.ID, cheese.ID)
		}
		if got.ID != 42 || got.Rating != 4 || got.Comment == nil || *got.Comment != "Tasty" {
			t.Errorf("detail review = %+v, want id 42, rating 4, comment Tasty", got.Review)
		}
		if !reflect.DeepEqual(got.User, &domain.UserRef{ID: bob.ID, Username: "bob"}) {
			t.Errorf("user = %+v, want the viewer", got.User)
		}
		if !reflect.DeepEqual(got.Burger, &cheese) {
			t.Errorf("burger = %+v, want %+v", got.Burger, cheese)
		}
	})

	t.Run("unknown shop yields ErrShopNotFound before anything else", func(t *testing.T) {
		repo := &fakeReviewRepo{getShop: getShop}
		if _, err := usecase.NewReviews(repo).Create(ctx, bob, 999, cheese.ID, "", 4, "ok"); !errors.Is(err, domain.ErrShopNotFound) {
			t.Fatalf("Create error = %v, want %v", err, domain.ErrShopNotFound)
		}
	})

	t.Run("AC2 rejected shop yields ErrForbidden before the burger lookup", func(t *testing.T) {
		repo := &fakeReviewRepo{getShop: getShop} // getShopBurger unset: a lookup would panic
		for _, viewer := range []domain.User{alice, bob, admin} {
			if _, err := usecase.NewReviews(repo).Create(ctx, viewer, rejectedShop.ID, cheese.ID, "", 4, "ok"); !errors.Is(err, domain.ErrForbidden) {
				t.Errorf("viewer %s: error = %v, want %v", viewer.Username, err, domain.ErrForbidden)
			}
		}
	})

	t.Run("AC2 pending shop: creator and admin may post, others get ErrForbidden", func(t *testing.T) {
		repo := &fakeReviewRepo{
			getShop:       getShop,
			getShopBurger: getShopBurger,
			createReview: func(_ context.Context, review domain.Review) (domain.Review, error) {
				review.ID = 43
				return review, nil
			},
		}
		reviews := usecase.NewReviews(repo)
		if _, err := reviews.Create(ctx, alice, pendingShop.ID, cheese.ID, "", 4, "ok"); err != nil {
			t.Errorf("creator: Create returned error: %v", err)
		}
		if _, err := reviews.Create(ctx, admin, pendingShop.ID, cheese.ID, "", 4, "ok"); err != nil {
			t.Errorf("admin: Create returned error: %v", err)
		}
		if _, err := usecase.NewReviews(&fakeReviewRepo{getShop: getShop}).Create(ctx, bob, pendingShop.ID, cheese.ID, "", 4, "ok"); !errors.Is(err, domain.ErrForbidden) {
			t.Errorf("other user: error = %v, want %v", err, domain.ErrForbidden)
		}
	})

	t.Run("unlinked burger yields ErrBurgerNotFound without an insert", func(t *testing.T) {
		repo := &fakeReviewRepo{getShop: getShop, getShopBurger: getShopBurger} // createReview unset
		if _, err := usecase.NewReviews(repo).Create(ctx, bob, activeShop.ID, 999, "", 4, "ok"); !errors.Is(err, domain.ErrBurgerNotFound) {
			t.Fatalf("Create error = %v, want %v", err, domain.ErrBurgerNotFound)
		}
	})

	t.Run("AC4 invalid content yields ValidationError without an insert", func(t *testing.T) {
		repo := &fakeReviewRepo{getShop: getShop, getShopBurger: getShopBurger} // createReview unset
		_, err := usecase.NewReviews(repo).Create(ctx, bob, activeShop.ID, cheese.ID, "", 0, " ")
		var vErr *domain.ValidationError
		if !errors.As(err, &vErr) {
			t.Fatalf("error = %v, want *domain.ValidationError", err)
		}
		want := []string{"Rating must be in 1..5", "Comment can't be blank"}
		if !reflect.DeepEqual(vErr.Messages, want) {
			t.Errorf("messages = %v, want %v", vErr.Messages, want)
		}
	})

	t.Run("burger_name path find-or-creates through the repository and composes the payload", func(t *testing.T) {
		smash := domain.ShopReviewBurger{ID: 7, Name: " Smash "}
		var gotShopID int64
		var gotName string
		var gotReview domain.Review
		repo := &fakeReviewRepo{
			getShop: getShop, // getShopBurger and createReview unset: any call panics
			createReviewForNamedBurger: func(_ context.Context, shopID int64, burgerName string, review domain.Review) (domain.Review, domain.ShopReviewBurger, error) {
				gotShopID, gotName, gotReview = shopID, burgerName, review
				review.ID = 44
				review.BurgerID = smash.ID
				return review, smash, nil
			},
		}
		// The name reaches the repository untrimmed (Rails never trims).
		got, err := usecase.NewReviews(repo).Create(ctx, bob, activeShop.ID, 0, " Smash ", 4, "Juicy")
		if err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
		if gotShopID != activeShop.ID || gotName != " Smash " {
			t.Errorf("repo got shop %d name %q, want %d %q", gotShopID, gotName, activeShop.ID, " Smash ")
		}
		if gotReview.AuthorID != bob.ID || gotReview.Rating != 4 {
			t.Errorf("repo got review %+v, want author %d rating 4", gotReview, bob.ID)
		}
		if got.ID != 44 || got.BurgerID != smash.ID {
			t.Errorf("detail review = %+v, want id 44 for burger %d", got.Review, smash.ID)
		}
		if !reflect.DeepEqual(got.Burger, &smash) {
			t.Errorf("burger = %+v, want %+v", got.Burger, smash)
		}
	})

	t.Run("a positive burger_id wins over burger_name", func(t *testing.T) {
		repo := &fakeReviewRepo{
			getShop:       getShop,
			getShopBurger: getShopBurger, // createReviewForNamedBurger unset: a call panics
			createReview: func(_ context.Context, review domain.Review) (domain.Review, error) {
				review.ID = 45
				return review, nil
			},
		}
		got, err := usecase.NewReviews(repo).Create(ctx, bob, activeShop.ID, cheese.ID, "Ignored", 4, "ok")
		if err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
		if got.BurgerID != cheese.ID || !reflect.DeepEqual(got.Burger, &cheese) {
			t.Errorf("burger = %+v, want the burger_id one %+v", got.Burger, cheese)
		}
	})

	t.Run("neither burger_id nor a usable burger_name yields ValidationError without any write", func(t *testing.T) {
		for name, burgerName := range map[string]string{"missing": "", "whitespace-only": "  \t "} {
			t.Run(name, func(t *testing.T) {
				repo := &fakeReviewRepo{getShop: getShop} // every write unset: a call panics
				_, err := usecase.NewReviews(repo).Create(ctx, bob, activeShop.ID, 0, burgerName, 4, "ok")
				var vErr *domain.ValidationError
				if !errors.As(err, &vErr) {
					t.Fatalf("error = %v, want *domain.ValidationError", err)
				}
				if want := []string{"Burger name can't be blank"}; !reflect.DeepEqual(vErr.Messages, want) {
					t.Errorf("messages = %v, want %v", vErr.Messages, want)
				}
			})
		}
	})

	t.Run("burger_name path validates content before the write", func(t *testing.T) {
		repo := &fakeReviewRepo{getShop: getShop} // createReviewForNamedBurger unset: a call panics
		_, err := usecase.NewReviews(repo).Create(ctx, bob, activeShop.ID, 0, "Smash", 0, " ")
		var vErr *domain.ValidationError
		if !errors.As(err, &vErr) {
			t.Fatalf("error = %v, want *domain.ValidationError", err)
		}
		want := []string{"Rating must be in 1..5", "Comment can't be blank"}
		if !reflect.DeepEqual(vErr.Messages, want) {
			t.Errorf("messages = %v, want %v", vErr.Messages, want)
		}
	})
}

// reviewDetailFor builds the stored detail the edit/delete tests load.
func reviewDetailFor(authorID int64) domain.ReviewDetail {
	comment := "Old"
	return domain.ReviewDetail{
		Review: domain.Review{ID: 9, Rating: 2, Comment: &comment, AuthorID: authorID, BurgerID: 5,
			CreatedAt: time.Date(2024, 4, 1, 12, 0, 0, 0, time.UTC)},
		User:   &domain.UserRef{ID: authorID, Username: "alice"},
		Burger: &domain.ShopReviewBurger{ID: 5, Name: "Cheese", AverageRating: 4.5, ReviewCount: 2},
	}
}

// TestReviewsUpdate covers the edit flow: author-only (AC3, admin gets no
// pass), validation before the write, the column-scoped write itself
// (discardReview stays unset so any discard panics), and the 404s.
func TestReviewsUpdate(t *testing.T) {
	alice := domain.User{ID: 1, Username: "alice"}
	bob := domain.User{ID: 2, Username: "bob"}
	admin := domain.User{ID: 3, Username: "root", Admin: true}
	ctx := context.Background()
	stored := reviewDetailFor(alice.ID)
	getReview := func(_ context.Context, id int64) (domain.ReviewDetail, error) {
		if id == stored.ID {
			return stored, nil
		}
		return domain.ReviewDetail{}, domain.ErrReviewNotFound
	}

	t.Run("AC3 author edit persists only rating and comment", func(t *testing.T) {
		var gotID int64
		var gotRating int
		var gotComment string
		repo := &fakeReviewRepo{
			getReview: getReview,
			updateReviewContent: func(_ context.Context, id int64, rating int, comment string) (domain.Review, error) {
				gotID, gotRating, gotComment = id, rating, comment
				updated := stored.Review
				updated.Rating = rating
				c := comment
				updated.Comment = &c
				return updated, nil
			},
		}
		got, err := usecase.NewReviews(repo).Update(ctx, alice, stored.ID, 5, "Better")
		if err != nil {
			t.Fatalf("Update returned error: %v", err)
		}
		if gotID != stored.ID || gotRating != 5 || gotComment != "Better" {
			t.Errorf("write = (%d, %d, %q), want (%d, 5, Better)", gotID, gotRating, gotComment, stored.ID)
		}
		if got.Rating != 5 || got.Comment == nil || *got.Comment != "Better" {
			t.Errorf("detail = %+v, want rating 5, comment Better", got.Review)
		}
		if !reflect.DeepEqual(got.User, stored.User) || !reflect.DeepEqual(got.Burger, stored.Burger) {
			t.Errorf("user/burger = %+v/%+v, want kept from the load", got.User, got.Burger)
		}
	})

	t.Run("AC3 non-author gets ErrForbidden without a write", func(t *testing.T) {
		repo := &fakeReviewRepo{getReview: getReview}      // updateReviewContent unset
		for _, viewer := range []domain.User{bob, admin} { // admin gets no pass
			if _, err := usecase.NewReviews(repo).Update(ctx, viewer, stored.ID, 5, "x"); !errors.Is(err, domain.ErrForbidden) {
				t.Errorf("viewer %s: error = %v, want %v", viewer.Username, err, domain.ErrForbidden)
			}
		}
	})

	t.Run("AC4 invalid content yields ValidationError without a write", func(t *testing.T) {
		repo := &fakeReviewRepo{getReview: getReview}
		_, err := usecase.NewReviews(repo).Update(ctx, alice, stored.ID, 6, "ok")
		var vErr *domain.ValidationError
		if !errors.As(err, &vErr) {
			t.Fatalf("error = %v, want *domain.ValidationError", err)
		}
		if want := []string{"Rating must be in 1..5"}; !reflect.DeepEqual(vErr.Messages, want) {
			t.Errorf("messages = %v, want %v", vErr.Messages, want)
		}
	})

	t.Run("unknown id yields ErrReviewNotFound", func(t *testing.T) {
		repo := &fakeReviewRepo{getReview: getReview}
		if _, err := usecase.NewReviews(repo).Update(ctx, alice, 999, 5, "x"); !errors.Is(err, domain.ErrReviewNotFound) {
			t.Fatalf("Update error = %v, want %v", err, domain.ErrReviewNotFound)
		}
	})
}

// TestReviewsDelete covers the soft-delete flow: author-only (AC3),
// discard called exactly for the loaded review (updateReviewContent stays
// unset so any content write panics), and the 404s.
func TestReviewsDelete(t *testing.T) {
	alice := domain.User{ID: 1, Username: "alice"}
	bob := domain.User{ID: 2, Username: "bob"}
	admin := domain.User{ID: 3, Username: "root", Admin: true}
	ctx := context.Background()
	stored := reviewDetailFor(alice.ID)
	getReview := func(_ context.Context, id int64) (domain.ReviewDetail, error) {
		if id == stored.ID {
			return stored, nil
		}
		return domain.ReviewDetail{}, domain.ErrReviewNotFound
	}

	t.Run("AC6 author delete discards the review", func(t *testing.T) {
		var discarded int64
		repo := &fakeReviewRepo{
			getReview: getReview,
			discardReview: func(_ context.Context, id int64) error {
				discarded = id
				return nil
			},
		}
		if err := usecase.NewReviews(repo).Delete(ctx, alice, stored.ID); err != nil {
			t.Fatalf("Delete returned error: %v", err)
		}
		if discarded != stored.ID {
			t.Errorf("discarded id = %d, want %d", discarded, stored.ID)
		}
	})

	t.Run("AC3 non-author gets ErrForbidden without a write", func(t *testing.T) {
		repo := &fakeReviewRepo{getReview: getReview}      // discardReview unset
		for _, viewer := range []domain.User{bob, admin} { // admin gets no pass
			if err := usecase.NewReviews(repo).Delete(ctx, viewer, stored.ID); !errors.Is(err, domain.ErrForbidden) {
				t.Errorf("viewer %s: error = %v, want %v", viewer.Username, err, domain.ErrForbidden)
			}
		}
	})

	t.Run("unknown id yields ErrReviewNotFound", func(t *testing.T) {
		repo := &fakeReviewRepo{getReview: getReview}
		if err := usecase.NewReviews(repo).Delete(ctx, alice, 999); !errors.Is(err, domain.ErrReviewNotFound) {
			t.Fatalf("Delete error = %v, want %v", err, domain.ErrReviewNotFound)
		}
	})

	t.Run("concurrent discard race surfaces the repository not-found", func(t *testing.T) {
		repo := &fakeReviewRepo{
			getReview: getReview,
			discardReview: func(_ context.Context, _ int64) error {
				return domain.ErrReviewNotFound // discarded between load and write
			},
		}
		if err := usecase.NewReviews(repo).Delete(ctx, alice, stored.ID); !errors.Is(err, domain.ErrReviewNotFound) {
			t.Fatalf("Delete error = %v, want %v", err, domain.ErrReviewNotFound)
		}
	})
}
