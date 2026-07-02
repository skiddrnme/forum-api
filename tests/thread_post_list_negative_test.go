//go:build e2e

package tests

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/google/uuid"

	. "stepik.leoscode.http/internal/gen/api"
)

func Test_Posts_List_Negative(t *testing.T) {
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

	var thread *Thread
	t.Run("[setup] create thread", func(t *testing.T) {
		c := newTestClient(t)

		reqID := "thread-" + randomSuffix()
		params := &CreateThreadParams{
			XUserId:         UserIdHeader(author),
			XIdempotencyKey: IdempotencyKeyHeader(reqID),
		}
		body := ThreadCreate{
			Title:   "thread for posts list negative " + randomSuffix(),
			Content: "content " + randomSuffix(),
			Tags:    &[]string{"go", "posts"},
		}
		thread = createThread(t, c, params, body)
	})

	// создадим пару постов, чтобы sanity был осмысленным
	t.Run("[setup] create posts", func(t *testing.T) {
		c := newTestClient(t)

		_ = createPost(t, c, author, thread.Id, "post #1 "+randomSuffix())
		_ = createPost(t, c, author, thread.Id, "post #2 "+randomSuffix())
	})

	baseURL := getBaseURL()

	// ---- negative: bad thread_id ----
	t.Run("400_thread_id_zero", func(t *testing.T) {
		status, body := rawListPosts(t, baseURL, "0", nil)
		if status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", status, string(body))
		}
	})

	t.Run("400_thread_id_non_int", func(t *testing.T) {
		status, body := rawListPosts(t, baseURL, "abc", nil)
		if status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", status, string(body))
		}
	})

	// ---- negative: not found ----
	t.Run("404_thread_not_found", func(t *testing.T) {
		status, body := rawListPosts(t, baseURL, "999999999", nil)
		if status != http.StatusNotFound {
			t.Fatalf("expected 404, got %d body=%s", status, string(body))
		}
	})

	// ---- negative: bad limit ----
	t.Run("400_limit_zero", func(t *testing.T) {
		status, body := rawListPosts(t, baseURL, strconv.FormatInt(thread.Id, 10), map[string]string{
			"Query": "limit=0",
		})
		if status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", status, string(body))
		}
	})

	t.Run("400_limit_too_big", func(t *testing.T) {
		status, body := rawListPosts(t, baseURL, strconv.FormatInt(thread.Id, 10), map[string]string{
			"Query": "limit=101",
		})
		if status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", status, string(body))
		}
	})

	t.Run("400_limit_not_a_number", func(t *testing.T) {
		status, body := rawListPosts(t, baseURL, strconv.FormatInt(thread.Id, 10), map[string]string{
			"Query": "limit=abc",
		})
		if status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", status, string(body))
		}
	})

	// ---- negative: bad offset ----
	t.Run("400_offset_negative", func(t *testing.T) {
		status, body := rawListPosts(t, baseURL, strconv.FormatInt(thread.Id, 10), map[string]string{
			"Query": "offset=-1",
		})
		if status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", status, string(body))
		}
	})

	t.Run("400_offset_not_a_number", func(t *testing.T) {
		status, body := rawListPosts(t, baseURL, strconv.FormatInt(thread.Id, 10), map[string]string{
			"Query": "offset=abc",
		})
		if status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", status, string(body))
		}
	})

	// ---- negative: optional X-User-Id header validation (если ты сделал это в wrap/middleware) ----
	t.Run("401_invalid_X-User-Id_format_optional_header", func(t *testing.T) {
		// важно: этот тест нужен только если middleware валидирует формат X-User-Id
		status, body := rawListPosts(t, baseURL, strconv.FormatInt(thread.Id, 10), map[string]string{
			"X-User-Id": "not-a-uuid",
		})
		if status != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d body=%s", status, string(body))
		}
	})

	// ---- sanity ----
	t.Run("200_offset_out_of_range_returns_empty_items", func(t *testing.T) {
		// offset сильно больше total => должен вернуться пустой items, но 200
		status, body := rawListPosts(t, baseURL, strconv.FormatInt(thread.Id, 10), map[string]string{
			"Query": "limit=20&offset=9999",
		})
		if status != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", status, string(body))
		}

		var out struct {
			Items []Post `json:"items"`
			Meta  struct {
				Total int64 `json:"total"`
			} `json:"meta"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			t.Fatalf("decode body: %v body=%s", err, string(body))
		}
		if len(out.Items) != 0 {
			t.Fatalf("expected empty items, got %d", len(out.Items))
		}
	})
}

// rawListPosts — сырой GET, чтобы удобно дергать кривые query/path.
// headers:
//   - обычные заголовки (например X-User-Id)
//   - спец-ключ "Query": строка query без '?', например "limit=0&offset=1"
func rawListPosts(t *testing.T, baseURL, threadID string, headers map[string]string) (status int, respBody []byte) {
	t.Helper()

	q := ""
	if headers != nil {
		if v, ok := headers["Query"]; ok && v != "" {
			q = "?" + v
			delete(headers, "Query")
		}
	}

	u := stringsTrimRightSlash(baseURL) + "/api/v1/threads/" + threadID + "/posts" + q
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, u, nil)
	if err != nil {
		t.Fatalf("[raw_list_posts] new request: %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	return do(t, req)
}

func stringsTrimRightSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}
