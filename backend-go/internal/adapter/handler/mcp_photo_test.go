package handler_test

import (
	"encoding/base64"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

func TestMCPReviewPhotos(t *testing.T) {
	k := newMCPKit(t)
	alice := k.connect(t, k.token(k.alice, readScope, writeScope))
	bob := k.connect(t, k.token(k.bob, readScope, writeScope))
	var review struct {
		ID       string
		PhotoURL string `json:"photo_url"`
	}
	// 旧来の JSON 上限を超える写真も、写真自体の上限までは投稿できる。
	result, failed := call(t, alice, "create_review", map[string]any{
		"shop_id": activeShopID, "burger_id": cheeseBurgerID, "rating": 4, "comment": "写真つき",
		"photo_base64": base64.StdEncoding.EncodeToString(jpegBytes(t, int(domain.MaxPhotoBytes))),
	})
	if failed {
		t.Fatalf("create: %s", result)
	}
	mustJSON(t, result, &review)
	original := photoPath(t, k.photoDir, review.PhotoURL)
	if _, err := os.Stat(original); err != nil {
		t.Fatal(err)
	}
	got, failed := call(t, alice, "get_review", map[string]any{"review_id": review.ID})
	if want := k.get(t, "/reviews/"+review.ID, k.aliceJWT); failed || got != want {
		t.Fatalf("REST = %s, MCP = %s", want, got)
	}

	t.Run("写真を省略した編集では現在の写真が残る", func(t *testing.T) {
		result, failed := call(t, alice, "update_review", map[string]any{
			"review_id": review.ID, "rating": 5, "comment": "本文のみ",
		})
		if failed {
			t.Fatal(result)
		}
		var updated struct {
			PhotoURL string `json:"photo_url"`
		}
		mustJSON(t, result, &updated)
		if updated.PhotoURL != review.PhotoURL {
			t.Fatalf("photo changed: %s", result)
		}
	})

	t.Run("他人のレビューの写真は差し替えられない", func(t *testing.T) {
		before := k.get(t, "/reviews/"+review.ID, k.aliceJWT)
		result, failed := call(t, bob, "update_review", map[string]any{
			"review_id": review.ID, "rating": 1, "comment": "変更",
			"photo_base64": base64.StdEncoding.EncodeToString(pngBytes(t)),
		})
		if !failed || result != "Forbidden" {
			t.Fatalf("update: %s, error=%v", result, failed)
		}
		if after := k.get(t, "/reviews/"+review.ID, k.aliceJWT); before != after {
			t.Fatal("review changed")
		}
	})

	t.Run("写真を指定した編集では保存ファイルを差し替える", func(t *testing.T) {
		result, failed := call(t, alice, "update_review", map[string]any{
			"review_id": review.ID, "rating": 5, "comment": "写真更新",
			"photo_base64": base64.StdEncoding.EncodeToString(pngBytes(t)),
		})
		if failed {
			t.Fatal(result)
		}
		mustJSON(t, result, &review)
		replacement := photoPath(t, k.photoDir, review.PhotoURL)
		if replacement == original || !strings.HasSuffix(replacement, ".png") {
			t.Fatalf("replacement = %s", replacement)
		}
		if _, err := os.Stat(replacement); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(original); !os.IsNotExist(err) {
			t.Fatalf("old photo remains: %v", err)
		}
	})

	t.Run("不正な写真は新規投稿も編集も保存しない", func(t *testing.T) {
		cases := map[string]string{
			"空文字":       "",
			"Base64でない": "%%%invalid",
			"画像でない":     base64.StdEncoding.EncodeToString([]byte("plain text")),
			"末尾が壊れている":  base64.StdEncoding.EncodeToString(pngBytes(t)) + "!",
			"写真上限を超える":  base64.StdEncoding.EncodeToString(jpegBytes(t, int(domain.MaxPhotoBytes)+3)),
			"同じBase64長でも復号後に上限を超える": base64.StdEncoding.EncodeToString(jpegBytes(t, int(domain.MaxPhotoBytes)+1)),
		}
		for name, encoded := range cases {
			t.Run(name, func(t *testing.T) {
				before := k.get(t, "/reviews/"+review.ID, k.aliceJWT)
				count := len(k.reviews.reviews)
				for _, tool := range []string{"create_review", "update_review"} {
					args := map[string]any{"rating": 1, "comment": "保存されない", "photo_base64": encoded}
					if tool == "create_review" {
						args["shop_id"] = activeShopID
						args["burger_id"] = cheeseBurgerID
					} else {
						args["review_id"] = review.ID
					}
					result, failed := call(t, alice, tool, args)
					if !failed {
						t.Fatalf("%s accepted: %s", tool, result)
					}
				}
				if len(k.reviews.reviews) != count {
					t.Fatal("review created")
				}
				if after := k.get(t, "/reviews/"+review.ID, k.aliceJWT); before != after {
					t.Fatal("review changed")
				}
			})
		}
	})
}

func TestMCPPhotoBodyLimit(t *testing.T) {
	for _, chunked := range []bool{false, true} {
		name := "Content-Lengthあり"
		if chunked {
			name = "Content-Lengthなし"
		}
		t.Run(name, func(t *testing.T) {
			k := newMCPKit(t)
			req := k.rpcRequest(t, k.token(k.alice, writeScope), strings.Repeat("a", 8<<20))
			if chunked {
				req.ContentLength = -1
			}
			resp, body := k.do(t, req)
			if resp.StatusCode != http.StatusRequestEntityTooLarge {
				t.Fatalf("status=%d body=%s", resp.StatusCode, body)
			}
		})
	}
}
