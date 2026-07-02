//go:build e2e

package tests

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"

	. "stepik.leoscode.http/internal/gen/api"
)

func Test_Search_Happy(t *testing.T) {
	// ---- setup ----
	t.Run("[setup] truncate", func(t *testing.T) {
		c := newTestClient(t)
		truncateDB(t, c)
	})

	var author uuid.UUID
	t.Run("[setup] login", func(t *testing.T) {
		c := newTestClient(t)
		author = login(t, c, "user_"+randomSuffix(), "password123")
	})

	var thr *Thread
	t.Run("[setup] seed data", func(t *testing.T) {
		c := newTestClient(t)

		thr = createThread(t, c, &CreateThreadParams{
			XUserId:         UserIdHeader(author),
			XIdempotencyKey: IdempotencyKeyHeader("thread-" + randomSuffix()),
		}, ThreadCreate{
			Title:   "How to learn Go " + randomSuffix(),
			Content: "This thread is about golang fulltext search",
			Tags:    &[]string{"go", "search"},
		})

		_ = createPost(t, c, author, thr.Id, "I like golang because it is simple "+randomSuffix())
	})

	baseURL := getBaseURL()

	t.Run("200_search_all_returns_thread_and_post", func(t *testing.T) {
		q := url.Values{}
		q.Set("search_query", "golang")
		q.Set("type", "all")

		resp := rawGET(t, baseURL+"/api/v1/search?"+q.Encode(), nil)
		expectStatus(t, resp, http.StatusOK)

		var out SearchResponse
		readJSONBody(t, resp, &out)

		if out.Items == nil || len(*out.Items) == 0 {
			t.Fatalf("expected non-empty search items")
		}

		// sanity: среди результатов должен быть хотя бы один ThreadSearchResult ИЛИ PostSearchResult
		var (
			foundThread bool
			foundPost   bool
		)
		for _, it := range *out.Items {
			if v, err := it.AsThreadSearchResult(); err == nil && v.Id == thr.Id {
				foundThread = true
			}
			if v, err := it.AsPostSearchResult(); err == nil && v.ThreadId == thr.Id {
				foundPost = true
			}
		}

		if !foundThread {
			t.Fatalf("expected thread result for thread_id=%d", thr.Id)
		}
		if !foundPost {
			t.Fatalf("expected post result for thread_id=%d", thr.Id)
		}
	})

	t.Run("200_search_type_thread_only", func(t *testing.T) {
		q := url.Values{}
		q.Set("search_query", "golang")
		q.Set("type", "thread")

		resp := rawGET(t, baseURL+"/api/v1/search?"+q.Encode(), nil)
		expectStatus(t, resp, http.StatusOK)

		var out SearchResponse
		readJSONBody(t, resp, &out)

		if out.Items == nil {
			t.Fatalf("expected items, got nil")
		}

		for _, it := range *out.Items {
			if r, err := it.AsThreadSearchResult(); err != nil {
				t.Fatalf("expected thread: %s", err)
			} else {
				t.Log(r)
			}
		}
	})

	t.Run("200_search_type_post_only", func(t *testing.T) {
		q := url.Values{}
		q.Set("search_query", "golang")
		q.Set("type", "post")

		resp := rawGET(t, baseURL+"/api/v1/search?"+q.Encode(), nil)
		expectStatus(t, resp, http.StatusOK)

		var out SearchResponse
		readJSONBody(t, resp, &out)

		if out.Items == nil {
			t.Fatalf("expected items, got nil")
		}

		for _, it := range *out.Items {
			if r, err := it.AsPostSearchResult(); err != nil {
				t.Fatalf("expected post: %s", err)
			} else {
				t.Log(r)
			}
		}
	})

	t.Run("200_empty_result", func(t *testing.T) {
		q := url.Values{}
		q.Set("search_query", "definitely_not_found_"+strconv.Itoa(int(thr.Id)))

		resp := rawGET(t, baseURL+"/api/v1/search?"+q.Encode(), nil)
		expectStatus(t, resp, http.StatusOK)

		var out SearchResponse
		readJSONBody(t, resp, &out)

		if out.Items == nil {
			// ok: items may be nil or empty slice (по твоему стилю)
			return
		}
		if len(*out.Items) != 0 {
			t.Fatalf("expected empty search result, got %d", len(*out.Items))
		}
	})
}

func Test_Search_Negative(t *testing.T) {
	baseURL := getBaseURL()

	t.Run("400_missing_search_query", func(t *testing.T) {
		resp := rawGET(t, baseURL+"/api/v1/search", nil)
		expectStatus(t, resp, http.StatusBadRequest)
	})

	t.Run("400_search_query_empty", func(t *testing.T) {
		resp := rawGET(t, baseURL+"/api/v1/search?search_query=", nil)
		expectStatus(t, resp, http.StatusBadRequest)
	})

	t.Run("400_search_query_spaces", func(t *testing.T) {
		resp := rawGET(t, baseURL+"/api/v1/search?search_query=+++ ", nil)
		expectStatus(t, resp, http.StatusBadRequest)
	})

	t.Run("400_search_query_too_long_1025", func(t *testing.T) {
		q := url.Values{}
		q.Set("search_query", strings.Repeat("a", 1025))
		resp := rawGET(t, baseURL+"/api/v1/search?"+q.Encode(), nil)
		expectStatus(t, resp, http.StatusBadRequest)
	})

	t.Run("400_type_invalid", func(t *testing.T) {
		q := url.Values{}
		q.Set("search_query", "go")
		q.Set("type", "nope")
		resp := rawGET(t, baseURL+"/api/v1/search?"+q.Encode(), nil)
		expectStatus(t, resp, http.StatusBadRequest)
	})
}

func expectStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()

	if resp.StatusCode != want {
		body := mustReadBody(t, resp)
		t.Fatalf("expected status %d, got %d, body=%s", want, resp.StatusCode, string(body))
	}
}

func readJSONBody(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	defer resp.Body.Close()

	dec := json.NewDecoder(resp.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(v); err != nil {
		body, _ := io.ReadAll(resp.Body) // вдруг что-то осталось
		t.Fatalf("decode json: %v (tail=%q)", err, string(body))
	}

	if dec.More() {
		t.Fatalf("decode json: unexpected extra tokens in response body")
	}
}

func mustReadBody(t *testing.T, resp *http.Response) []byte {
	t.Helper()
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return b
}
