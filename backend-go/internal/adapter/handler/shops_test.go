package handler_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/handler"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/adapter/storage"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// shopRepoFake is an in-memory usecase.ShopRepository. Visibility is
// applied through the domain descriptor itself (vis.CanView), so the rule
// is not re-implemented here; keyword matching is a simple case-fold
// substring (metacharacter semantics are covered by the repository
// integration tests). Setting err fails every operation (500 paths).
type shopRepoFake struct {
	shops   []domain.ShopDetail // Reviews unset; served via reviews below
	reviews map[int64][]domain.ShopReview
	err     error
}

func (f *shopRepoFake) ListShops(_ context.Context, vis domain.ShopVisibility, keyword string, limit, offset int32) ([]domain.Shop, error) {
	if f.err != nil {
		return nil, f.err
	}
	var out []domain.Shop
	for _, d := range f.shops {
		if !vis.CanView(d.Shop) {
			continue
		}
		if keyword != "" && !strings.Contains(strings.ToLower(d.Name), strings.ToLower(keyword)) {
			continue
		}
		out = append(out, d.Shop)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].ID < out[j].ID
	})
	lo := min(int(offset), len(out))
	hi := min(lo+int(limit), len(out))
	return out[lo:hi], nil
}

func (f *shopRepoFake) GetShopWithCreator(_ context.Context, id int64) (domain.ShopDetail, error) {
	if f.err != nil {
		return domain.ShopDetail{}, f.err
	}
	for _, d := range f.shops {
		if d.ID == id {
			return d, nil
		}
	}
	return domain.ShopDetail{}, domain.ErrShopNotFound
}

func (f *shopRepoFake) ListShopReviews(_ context.Context, shopID int64) ([]domain.ShopReview, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.reviews[shopID], nil
}

// newShopsRouter wires the router with the auth kit and the given shop
// fake, returning issued Bearer headers for a regular user and an admin.
func newShopsRouter(t *testing.T, repo *shopRepoFake) (router http.Handler, aliceAuth, adminAuth string, aliceID int64) {
	t.Helper()
	users, auth, codec := newAuthKit()
	alice := users.seed("alice", "alice@example.com", "password123")
	admin := users.seed("root", "root@example.com", "password123")
	users.users[admin.ID].user.Admin = true
	aliceToken, err := codec.Issue(alice.ID)
	if err != nil {
		t.Fatalf("issue alice token: %v", err)
	}
	adminToken, err := codec.Issue(admin.ID)
	if err != nil {
		t.Fatalf("issue admin token: %v", err)
	}
	return handler.NewRouter(okPinger, auth, usecase.NewShops(repo),
			usecase.NewReviews(newReviewRepoFake(), storage.NewDisk(t.TempDir(), "/photos")),
			usecase.NewUsers(users, hasherFake{}), nil),
		"Bearer " + aliceToken, "Bearer " + adminToken, alice.ID
}

func shopPtr[T any](v T) *T { return &v }

// seedShops returns a fake with one active, one pending (created by
// creatorID), and one rejected shop.
func seedShops(creatorID int64) *shopRepoFake {
	return &shopRepoFake{
		shops: []domain.ShopDetail{
			{Shop: domain.Shop{ID: 1, Name: "Active Diner", Status: domain.ShopStatusActive}},
			{
				Shop:    domain.Shop{ID: 2, Name: "Alice Pending", Status: domain.ShopStatusPending, CreatorID: shopPtr(creatorID)},
				Creator: &domain.UserRef{ID: creatorID, Username: "alice"},
			},
			{Shop: domain.Shop{ID: 3, Name: "Rejected Grill", Status: domain.ShopStatusRejected, CreatorID: shopPtr(creatorID + 100)}},
		},
		reviews: map[int64][]domain.ShopReview{},
	}
}

// TestListShops covers AC1–AC3 at the HTTP level: the visible set depends
// on the OptionalAuth viewer, and the body is a top-level snake_case
// array ordered by name.
func TestListShops(t *testing.T) {
	repo := seedShops(1)
	router, aliceAuth, adminAuth, _ := newShopsRouter(t, repo)

	tests := []struct {
		name       string
		authHeader string
		wantBody   string
	}{
		{
			name:     "AC1 anonymous sees active shops only",
			wantBody: `[{"id":1,"name":"Active Diner","status":"active"}]`,
		},
		{
			name:       "AC2 creator additionally sees own pending shop with status",
			authHeader: aliceAuth,
			wantBody:   `[{"id":1,"name":"Active Diner","status":"active"},{"id":2,"name":"Alice Pending","status":"pending"}]`,
		},
		{
			name:       "AC3 admin sees all statuses",
			authHeader: adminAuth,
			wantBody:   `[{"id":1,"name":"Active Diner","status":"active"},{"id":2,"name":"Alice Pending","status":"pending"},{"id":3,"name":"Rejected Grill","status":"rejected"}]`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(router, http.MethodGet, "/shops", "", tt.authHeader)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body)
			}
			if got := rec.Body.String(); got != tt.wantBody {
				t.Errorf("body = %s, want %s", got, tt.wantBody)
			}
		})
	}
}

// TestListShopsParams covers keyword filtering and the pagination
// fallbacks at the HTTP level: non-numeric values fall back to defaults
// instead of erroring, and out-of-range pages yield an empty array.
func TestListShopsParams(t *testing.T) {
	repo := seedShops(1)
	router, _, adminAuth, _ := newShopsRouter(t, repo)

	tests := []struct {
		name       string
		query      string
		authHeader string
		wantBody   string
	}{
		{
			name:     "keyword filters by substring",
			query:    "?keyword=diner",
			wantBody: `[{"id":1,"name":"Active Diner","status":"active"}]`,
		},
		{
			name:     "keyword with no match yields empty array",
			query:    "?keyword=nope",
			wantBody: `[]`,
		},
		{
			name:       "per_page=1 page=2 returns the second shop",
			query:      "?per_page=1&page=2",
			authHeader: adminAuth,
			wantBody:   `[{"id":2,"name":"Alice Pending","status":"pending"}]`,
		},
		{
			name:     "non-numeric page and per_page fall back to defaults",
			query:    "?page=abc&per_page=xyz",
			wantBody: `[{"id":1,"name":"Active Diner","status":"active"}]`,
		},
		{
			name:     "page beyond the data yields empty array",
			query:    "?page=99",
			wantBody: `[]`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(router, http.MethodGet, "/shops"+tt.query, "", tt.authHeader)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body)
			}
			if got := rec.Body.String(); got != tt.wantBody {
				t.Errorf("body = %s, want %s", got, tt.wantBody)
			}
		})
	}
}

// TestListShopsRepoFailure: a repository failure surfaces as 500.
func TestListShopsRepoFailure(t *testing.T) {
	router, _, _, _ := newShopsRouter(t, &shopRepoFake{err: io.ErrUnexpectedEOF})
	rec := do(router, http.MethodGet, "/shops", "", "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusInternalServerError, rec.Body)
	}
	if got := rec.Body.String(); got != `{"error":"internal server error"}` {
		t.Errorf("body = %q, want the 500 JSON error", got)
	}
}

// TestGetShopDetail asserts the exact detail body: snake_case fields,
// creator object, and reviews with user, burger, and stats inline.
func TestGetShopDetail(t *testing.T) {
	repo := seedShops(1)
	repo.reviews[1] = []domain.ShopReview{
		{
			ID:        9,
			Rating:    4,
			Comment:   shopPtr("Tasty"),
			CreatedAt: time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC),
			User:      &domain.UserRef{ID: 3, Username: "bob"},
			Burger: &domain.ShopReviewBurger{
				ID: 5, Name: "Cheese", AverageRating: 4.5, ReviewCount: 2, WeightedScore: 4.1, Confidence: 0.8,
			},
		},
		{
			ID:        8,
			Rating:    2,
			CreatedAt: time.Date(2024, 4, 1, 12, 0, 0, 0, time.UTC),
			User:      &domain.UserRef{ID: 3, Username: "bob"},
			Burger:    &domain.ShopReviewBurger{ID: 6, Name: "Plain"},
		},
	}
	router, _, _, _ := newShopsRouter(t, repo)

	rec := do(router, http.MethodGet, "/shops/1", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body)
	}
	want := `{"id":1,"name":"Active Diner","status":"active","moderation_note":null,"creator":null,"reviews":[` +
		`{"id":9,"rating":4,"comment":"Tasty","created_at":"2024-05-01T12:00:00Z","photo_url":null,"user":{"id":3,"username":"bob"},` +
		`"burger":{"id":5,"name":"Cheese","average_rating":4.5,"review_count":2,"weighted_score":4.1,"confidence":0.8}},` +
		`{"id":8,"rating":2,"comment":null,"created_at":"2024-04-01T12:00:00Z","photo_url":null,"user":{"id":3,"username":"bob"},` +
		`"burger":{"id":6,"name":"Plain","average_rating":0,"review_count":0,"weighted_score":0,"confidence":0}}]}`
	if got := rec.Body.String(); got != want {
		t.Errorf("body = %s, want %s", got, want)
	}
}

// TestGetShopVisibility covers AC4 and AC6: pending shops 404 for
// anonymous viewers but open for the creator and admins, unknown and
// non-numeric ids 404 with the identical body, failures 500.
func TestGetShopVisibility(t *testing.T) {
	repo := seedShops(1)
	router, aliceAuth, adminAuth, _ := newShopsRouter(t, repo)
	const notFoundBody = `{"error":"Shop not found"}`

	tests := []struct {
		name       string
		path       string
		authHeader string
		wantStatus int
	}{
		{name: "AC4 anonymous viewer gets 404 for pending shop", path: "/shops/2", wantStatus: http.StatusNotFound},
		{name: "AC4 creator gets 200 for own pending shop", path: "/shops/2", authHeader: aliceAuth, wantStatus: http.StatusOK},
		{name: "AC4 admin gets 200 for pending shop", path: "/shops/2", authHeader: adminAuth, wantStatus: http.StatusOK},
		{name: "non-creator gets 404 for rejected shop", path: "/shops/3", authHeader: aliceAuth, wantStatus: http.StatusNotFound},
		{name: "AC6 unknown id gets 404", path: "/shops/999", wantStatus: http.StatusNotFound},
		{name: "non-numeric id gets the same 404", path: "/shops/abc", wantStatus: http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(router, http.MethodGet, tt.path, "", tt.authHeader)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body)
			}
			if tt.wantStatus == http.StatusNotFound {
				if got := rec.Body.String(); got != notFoundBody {
					t.Errorf("body = %q, want %q", got, notFoundBody)
				}
			}
		})
	}

	t.Run("repository failure returns 500", func(t *testing.T) {
		failRouter, _, _, _ := newShopsRouter(t, &shopRepoFake{err: fmt.Errorf("db down")})
		rec := do(failRouter, http.MethodGet, "/shops/1", "", "")
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusInternalServerError, rec.Body)
		}
	})
}
