package handler_test

import (
	"context"
	"net/http"
	"sort"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// The moderation methods of shopRepoFake (declared in shops_test.go). The
// fake has no timestamps, so id desc stands in for the repository's
// created_at desc, id desc ordering.

func (f *shopRepoFake) CreateShop(_ context.Context, shop domain.Shop) (domain.Shop, error) {
	if f.err != nil {
		return domain.Shop{}, f.err
	}
	for _, d := range f.shops {
		if d.ID >= shop.ID {
			shop.ID = d.ID + 1
		}
	}
	f.shops = append(f.shops, domain.ShopDetail{Shop: shop})
	return shop, nil
}

func (f *shopRepoFake) ListShopsForModeration(_ context.Context, status *domain.ShopStatus) ([]domain.ShopDetail, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make([]domain.ShopDetail, 0, len(f.shops))
	for _, d := range f.shops {
		if status != nil && d.Status != *status {
			continue
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out, nil
}

func (f *shopRepoFake) UpdateShop(_ context.Context, shop domain.Shop) (domain.Shop, error) {
	if f.err != nil {
		return domain.Shop{}, f.err
	}
	for i, d := range f.shops {
		if d.ID == shop.ID {
			f.shops[i].Shop = shop
			return shop, nil
		}
	}
	return domain.Shop{}, domain.ErrShopNotFound
}

// TestCreateShop covers POST /shops: RequireAuth gates it, a blank or
// missing name is the Rails-parity 422, and success answers 201 with the
// admin shop shape carrying the viewer as creator.
func TestCreateShop(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		auth       bool
		wantStatus int
		wantBody   string
	}{
		{
			name:       "unauthenticated returns 401",
			body:       `{"shop":{"name":"New Shack"}}`,
			wantStatus: http.StatusUnauthorized,
			wantBody:   `{"error":"Unauthorized"}`,
		},
		{
			name:       "whitespace-only name returns 422",
			body:       `{"shop":{"name":"   "}}`,
			auth:       true,
			wantStatus: http.StatusUnprocessableEntity,
			wantBody:   `{"errors":["Name can't be blank"]}`,
		},
		{
			name:       "missing shop wrapper returns the same 422",
			body:       `{}`,
			auth:       true,
			wantStatus: http.StatusUnprocessableEntity,
			wantBody:   `{"errors":["Name can't be blank"]}`,
		},
		{
			name:       "malformed JSON returns 400",
			body:       `{"shop":`,
			auth:       true,
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"error":"invalid JSON body"}`,
		},
		{
			name:       "valid name returns 201 pending with creator",
			body:       `{"shop":{"name":"New Shack"}}`,
			auth:       true,
			wantStatus: http.StatusCreated,
			wantBody:   `{"id":4,"name":"New Shack","status":"pending","moderation_note":null,"creator":{"id":1,"username":"alice"}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, aliceAuth, _, _ := newShopsRouter(t, seedShops(1))
			authHeader := ""
			if tt.auth {
				authHeader = aliceAuth
			}
			rec := do(router, http.MethodPost, "/shops", tt.body, authHeader)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body)
			}
			if got := rec.Body.String(); got != tt.wantBody {
				t.Errorf("body = %s, want %s", got, tt.wantBody)
			}
		})
	}

	t.Run("created pending shop is listed for its creator but not anonymously", func(t *testing.T) {
		router, aliceAuth, _, _ := newShopsRouter(t, seedShops(1))
		if rec := do(router, http.MethodPost, "/shops", `{"shop":{"name":"New Shack"}}`, aliceAuth); rec.Code != http.StatusCreated {
			t.Fatalf("create status = %d (body %s)", rec.Code, rec.Body)
		}
		anon := do(router, http.MethodGet, "/shops?keyword=New+Shack", "", "").Body.String()
		if anon != `[]` {
			t.Errorf("anonymous list = %s, want []", anon)
		}
		own := do(router, http.MethodGet, "/shops?keyword=New+Shack", "", aliceAuth).Body.String()
		if want := `[{"id":4,"name":"New Shack","status":"pending"}]`; own != want {
			t.Errorf("creator list = %s, want %s", own, want)
		}
	})
}

// TestAdminShopsForbidden pins the HTTP mapping of the usecase-side
// authorization: every admin endpoint answers 403 for an authenticated
// non-admin (with no way to probe shop existence) and 401 without a
// token.
func TestAdminShopsForbidden(t *testing.T) {
	endpoints := []struct {
		method string
		path   string
		body   string // PUT needs a well-formed body; malformed JSON is 400 by convention
	}{
		{method: http.MethodGet, path: "/admin/shops"},
		{method: http.MethodPut, path: "/admin/shops/1", body: `{"shop":{"name":"x"}}`},
		{method: http.MethodPost, path: "/admin/shops/1/approve"},
		{method: http.MethodPost, path: "/admin/shops/1/reject"},
	}
	router, aliceAuth, _, _ := newShopsRouter(t, seedShops(1))
	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path+" non-admin gets 403", func(t *testing.T) {
			rec := do(router, ep.method, ep.path, ep.body, aliceAuth)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusForbidden, rec.Body)
			}
			if got := rec.Body.String(); got != `{"error":"Forbidden"}` {
				t.Errorf("body = %q, want the Forbidden JSON", got)
			}
		})
		t.Run(ep.method+" "+ep.path+" unauthenticated gets 401", func(t *testing.T) {
			rec := do(router, ep.method, ep.path, "", "")
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusUnauthorized, rec.Body)
			}
		})
	}

	t.Run("non-admin gets 403 even for an unknown shop id", func(t *testing.T) {
		rec := do(router, http.MethodPost, "/admin/shops/999/approve", "", aliceAuth)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d (existence must not leak)", rec.Code, http.StatusForbidden)
		}
	})
}

// TestAdminListShops covers GET /admin/shops: every shop newest first in
// the admin shop shape, the pending/active/rejected filter, and an
// unknown filter value degrading to an empty array.
func TestAdminListShops(t *testing.T) {
	repo := seedShops(1)
	repo.shops[2].Shop.ModerationNote = shopPtr("needs fixes")
	router, _, adminAuth, _ := newShopsRouter(t, repo)

	tests := []struct {
		name     string
		query    string
		wantBody string
	}{
		{
			name:  "all shops newest first",
			query: "",
			wantBody: `[{"id":3,"name":"Rejected Grill","status":"rejected","moderation_note":"needs fixes","creator":null},` +
				`{"id":2,"name":"Alice Pending","status":"pending","moderation_note":null,"creator":{"id":1,"username":"alice"}},` +
				`{"id":1,"name":"Active Diner","status":"active","moderation_note":null,"creator":null}]`,
		},
		{
			name:     "status=pending filters",
			query:    "?status=pending",
			wantBody: `[{"id":2,"name":"Alice Pending","status":"pending","moderation_note":null,"creator":{"id":1,"username":"alice"}}]`,
		},
		{
			name:     "unknown status yields empty array",
			query:    "?status=bogus",
			wantBody: `[]`,
		},
		{
			name:  "empty status means all",
			query: "?status=",
			wantBody: `[{"id":3,"name":"Rejected Grill","status":"rejected","moderation_note":"needs fixes","creator":null},` +
				`{"id":2,"name":"Alice Pending","status":"pending","moderation_note":null,"creator":{"id":1,"username":"alice"}},` +
				`{"id":1,"name":"Active Diner","status":"active","moderation_note":null,"creator":null}]`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(router, http.MethodGet, "/admin/shops"+tt.query, "", adminAuth)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body)
			}
			if got := rec.Body.String(); got != tt.wantBody {
				t.Errorf("body = %s, want %s", got, tt.wantBody)
			}
		})
	}
}

// TestAdminUpdateShop covers PUT /admin/shops/{id}: rename with the admin
// shop shape, the Rails-parity 422 for a blank name, and the uniform 404
// for unknown and non-numeric ids.
func TestAdminUpdateShop(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		body       string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "rename returns 200 with unchanged status",
			path:       "/admin/shops/2",
			body:       `{"shop":{"name":"Renamed Shack"}}`,
			wantStatus: http.StatusOK,
			wantBody:   `{"id":2,"name":"Renamed Shack","status":"pending","moderation_note":null,"creator":{"id":1,"username":"alice"}}`,
		},
		{
			name:       "blank name returns 422",
			path:       "/admin/shops/2",
			body:       `{"shop":{"name":""}}`,
			wantStatus: http.StatusUnprocessableEntity,
			wantBody:   `{"errors":["Name can't be blank"]}`,
		},
		{
			name:       "unknown id returns 404",
			path:       "/admin/shops/999",
			body:       `{"shop":{"name":"Renamed"}}`,
			wantStatus: http.StatusNotFound,
			wantBody:   `{"error":"Shop not found"}`,
		},
		{
			name:       "non-numeric id returns the same 404",
			path:       "/admin/shops/abc",
			body:       `{"shop":{"name":"Renamed"}}`,
			wantStatus: http.StatusNotFound,
			wantBody:   `{"error":"Shop not found"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, _, adminAuth, _ := newShopsRouter(t, seedShops(1))
			rec := do(router, http.MethodPut, tt.path, tt.body, adminAuth)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body)
			}
			if got := rec.Body.String(); got != tt.wantBody {
				t.Errorf("body = %s, want %s", got, tt.wantBody)
			}
		})
	}
}

// TestAdminApproveShop covers POST /admin/shops/{id}/approve: 200 with
// status active and the note cleared (also from rejected — re-approval),
// any request body ignored, 404 for unknown ids, and the approved shop
// becoming anonymously visible.
func TestAdminApproveShop(t *testing.T) {
	repo := seedShops(1)
	repo.shops[2].Shop.ModerationNote = shopPtr("needs fixes")
	router, _, adminAuth, _ := newShopsRouter(t, repo)

	t.Run("approves a rejected shop and clears the note", func(t *testing.T) {
		rec := do(router, http.MethodPost, "/admin/shops/3/approve", "", adminAuth)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body)
		}
		want := `{"id":3,"name":"Rejected Grill","status":"active","moderation_note":null,"creator":null}`
		if got := rec.Body.String(); got != want {
			t.Errorf("body = %s, want %s", got, want)
		}
	})

	t.Run("approved shop appears in the anonymous list", func(t *testing.T) {
		got := do(router, http.MethodGet, "/shops?keyword=Rejected+Grill", "", "").Body.String()
		if want := `[{"id":3,"name":"Rejected Grill","status":"active"}]`; got != want {
			t.Errorf("anonymous list = %s, want %s", got, want)
		}
	})

	t.Run("request body is ignored", func(t *testing.T) {
		rec := do(router, http.MethodPost, "/admin/shops/2/approve", `not even json`, adminAuth)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body)
		}
	})

	t.Run("unknown id returns 404", func(t *testing.T) {
		rec := do(router, http.MethodPost, "/admin/shops/999/approve", "", adminAuth)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusNotFound, rec.Body)
		}
		if got := rec.Body.String(); got != `{"error":"Shop not found"}` {
			t.Errorf("body = %q, want the uniform shop 404", got)
		}
	})
}

// TestAdminRejectShop covers POST /admin/shops/{id}/reject: the top-level
// optional moderation_note is echoed, an entirely empty body rejects with
// a null note, and the rejected shop leaves the anonymous list.
func TestAdminRejectShop(t *testing.T) {
	router, _, adminAuth, _ := newShopsRouter(t, seedShops(1))

	t.Run("rejects with the note", func(t *testing.T) {
		rec := do(router, http.MethodPost, "/admin/shops/1/reject", `{"moderation_note":"spam"}`, adminAuth)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body)
		}
		want := `{"id":1,"name":"Active Diner","status":"rejected","moderation_note":"spam","creator":null}`
		if got := rec.Body.String(); got != want {
			t.Errorf("body = %s, want %s", got, want)
		}
	})

	t.Run("rejected shop leaves the anonymous list", func(t *testing.T) {
		got := do(router, http.MethodGet, "/shops?keyword=Active+Diner", "", "").Body.String()
		if got != `[]` {
			t.Errorf("anonymous list = %s, want []", got)
		}
	})

	t.Run("empty body rejects with null note", func(t *testing.T) {
		rec := do(router, http.MethodPost, "/admin/shops/2/reject", "", adminAuth)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body)
		}
		want := `{"id":2,"name":"Alice Pending","status":"rejected","moderation_note":null,"creator":{"id":1,"username":"alice"}}`
		if got := rec.Body.String(); got != want {
			t.Errorf("body = %s, want %s", got, want)
		}
	})

	t.Run("malformed JSON body returns 400", func(t *testing.T) {
		rec := do(router, http.MethodPost, "/admin/shops/2/reject", `{"moderation_note":`, adminAuth)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusBadRequest, rec.Body)
		}
	})

	t.Run("unknown id returns 404", func(t *testing.T) {
		rec := do(router, http.MethodPost, "/admin/shops/999/reject", "", adminAuth)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusNotFound, rec.Body)
		}
	})
}
