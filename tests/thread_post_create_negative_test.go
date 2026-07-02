//go:build e2e

package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"

	. "stepik.leoscode.http/internal/gen/api"
)

func Test_Threads_Posts_Create_Negative(t *testing.T) {
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

		params := &CreateThreadParams{
			XUserId:         UserIdHeader(author),
			XIdempotencyKey: IdempotencyKeyHeader("thread-" + randomSuffix()),
		}
		body := ThreadCreate{
			Title:   "thread for posts create negative " + randomSuffix(),
			Content: "content for posts create negative " + randomSuffix(),
			Tags:    &[]string{"go", "negative"},
		}
		thread = createThread(t, c, params, body)
	})

	baseURL := getBaseURL()

	// ---- 401 ----
	t.Run("401_missing_X-User-Id", func(t *testing.T) {
		body, _ := json.Marshal(PostCreate{Content: "ok"})
		status, respBody := rawCreatePostInThread(t, baseURL, strconv.FormatInt(thread.Id, 10), map[string]string{
			"X-Request-Id": "post-" + randomSuffix(),
			// no X-User-Id
		}, body, "application/json")

		if status != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d body=%s", status, string(respBody))
		}
	})

	t.Run("401_invalid_X-User-Id_format", func(t *testing.T) {
		body, _ := json.Marshal(PostCreate{Content: "ok"})
		status, respBody := rawCreatePostInThread(t, baseURL, strconv.FormatInt(thread.Id, 10), map[string]string{
			"X-User-Id":    "not-a-uuid",
			"X-Request-Id": "post-" + randomSuffix(),
		}, body, "application/json")

		if status != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d body=%s", status, string(respBody))
		}
	})

	// ---- 400: X-Request-Id ----
	t.Run("400_missing_X-Request-Id", func(t *testing.T) {
		body, _ := json.Marshal(PostCreate{Content: "ok"})
		status, respBody := rawCreatePostInThread(t, baseURL, strconv.FormatInt(thread.Id, 10), map[string]string{
			"X-User-Id": author.String(),
			// no X-Request-Id
		}, body, "application/json")

		if status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", status, string(respBody))
		}
	})

	t.Run("400_X-Request-Id_empty", func(t *testing.T) {
		body, _ := json.Marshal(PostCreate{Content: "ok"})
		status, respBody := rawCreatePostInThread(t, baseURL, strconv.FormatInt(thread.Id, 10), map[string]string{
			"X-User-Id":    author.String(),
			"X-Request-Id": "   ",
		}, body, "application/json")

		if status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", status, string(respBody))
		}
	})

	t.Run("400_X-Request-Id_too_long_129", func(t *testing.T) {
		body, _ := json.Marshal(PostCreate{Content: "ok"})
		status, respBody := rawCreatePostInThread(t, baseURL, strconv.FormatInt(thread.Id, 10), map[string]string{
			"X-User-Id":    author.String(),
			"X-Request-Id": strings.Repeat("a", 129),
		}, body, "application/json")

		if status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", status, string(respBody))
		}
	})

	// ---- 400: thread_id ----
	t.Run("400_invalid_thread_id_zero", func(t *testing.T) {
		body, _ := json.Marshal(PostCreate{Content: "ok"})
		status, respBody := rawCreatePostInThread(t, baseURL, "0", map[string]string{
			"X-User-Id":    author.String(),
			"X-Request-Id": "post-" + randomSuffix(),
		}, body, "application/json")

		if status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", status, string(respBody))
		}
	})

	t.Run("400_invalid_thread_id_non_int", func(t *testing.T) {
		body, _ := json.Marshal(PostCreate{Content: "ok"})
		status, respBody := rawCreatePostInThread(t, baseURL, "abc", map[string]string{
			"X-User-Id":    author.String(),
			"X-Request-Id": "post-" + randomSuffix(),
		}, body, "application/json")

		if status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", status, string(respBody))
		}
	})

	// ---- 400: content-type / json ----
	t.Run("400_wrong_content_type", func(t *testing.T) {
		body := []byte(`content=ok`)
		status, respBody := rawCreatePostInThread(t, baseURL, strconv.FormatInt(thread.Id, 10), map[string]string{
			"X-User-Id":    author.String(),
			"X-Request-Id": "post-" + randomSuffix(),
		}, body, "application/x-www-form-urlencoded")

		if status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", status, string(respBody))
		}
	})

	t.Run("400_bad_json", func(t *testing.T) {
		status, respBody := rawCreatePostInThread(t, baseURL, strconv.FormatInt(thread.Id, 10), map[string]string{
			"X-User-Id":    author.String(),
			"X-Request-Id": "post-" + randomSuffix(),
		}, []byte(`{"content":`), "application/json")

		if status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", status, string(respBody))
		}
	})

	t.Run("400_missing_required_fields", func(t *testing.T) {
		// PostCreate требует content
		status, respBody := rawCreatePostInThread(t, baseURL, strconv.FormatInt(thread.Id, 10), map[string]string{
			"X-User-Id":    author.String(),
			"X-Request-Id": "post-" + randomSuffix(),
		}, []byte(`{}`), "application/json")

		if status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", status, string(respBody))
		}
	})

	t.Run("400_content_wrong_type", func(t *testing.T) {
		// content должен быть строкой, а тут число
		status, respBody := rawCreatePostInThread(t, baseURL, strconv.FormatInt(thread.Id, 10), map[string]string{
			"X-User-Id":    author.String(),
			"X-Request-Id": "post-" + randomSuffix(),
		}, []byte(`{"content":123}`), "application/json")

		if status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", status, string(respBody))
		}
	})

	t.Run("400_content_empty_or_spaces", func(t *testing.T) {
		body, _ := json.Marshal(PostCreate{Content: "   "})
		status, respBody := rawCreatePostInThread(t, baseURL, strconv.FormatInt(thread.Id, 10), map[string]string{
			"X-User-Id":    author.String(),
			"X-Request-Id": "post-" + randomSuffix(),
		}, body, "application/json")

		if status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", status, string(respBody))
		}
	})

	// ---- 404 ----
	t.Run("404_thread_not_found", func(t *testing.T) {
		body, _ := json.Marshal(PostCreate{Content: "ok"})
		status, respBody := rawCreatePostInThread(t, baseURL, "999999999", map[string]string{
			"X-User-Id":         author.String(),
			"X-Idempotency-Key": "post-" + randomSuffix(),
		}, body, "application/json")

		if status != http.StatusNotFound {
			t.Fatalf("expected 404, got %d body=%s", status, string(respBody))
		}
	})

	// ---- idempotency ----
	t.Run("idempotency_same_X-Request-Id_same_body_returns_same_post", func(t *testing.T) {
		reqID := "post-" + randomSuffix()

		body1, _ := json.Marshal(PostCreate{Content: "IDEMPOTENT POST " + randomSuffix()})

		// 1) created
		st1, rb1 := rawCreatePostInThread(t, baseURL, strconv.FormatInt(thread.Id, 10), map[string]string{
			"X-User-Id":         author.String(),
			"X-Idempotency-Key": reqID,
		}, body1, "application/json")

		if st1 != http.StatusCreated {
			t.Fatalf("expected 201, got %d body=%s", st1, string(rb1))
		}

		var p1 Post
		if err := json.Unmarshal(rb1, &p1); err != nil {
			t.Fatalf("unmarshal post1: %v body=%s", err, string(rb1))
		}

		// 2) repeat same request-id + same body => 200 OK and same post id
		st2, rb2 := rawCreatePostInThread(t, baseURL, strconv.FormatInt(thread.Id, 10), map[string]string{
			"X-User-Id":         author.String(),
			"X-Idempotency-Key": reqID,
		}, body1, "application/json")

		if st2 != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", st2, string(rb2))
		}

		var p2 Post
		if err := json.Unmarshal(rb2, &p2); err != nil {
			t.Fatalf("unmarshal post2: %v body=%s", err, string(rb2))
		}

		if p1.Id != p2.Id {
			t.Fatalf("expected same post id, got p1=%d p2=%d", p1.Id, p2.Id)
		}
		if p2.ThreadId != thread.Id {
			t.Fatalf("expected thread_id=%d, got %d", thread.Id, p2.ThreadId)
		}
	})

	t.Run("idempotency_same_X-Request-Id_different_body_returns_409", func(t *testing.T) {
		reqID := "post-" + randomSuffix()

		bodyA, _ := json.Marshal(PostCreate{Content: "A " + randomSuffix()})
		bodyB, _ := json.Marshal(PostCreate{Content: "B " + randomSuffix()})

		// first ok
		st1, rb1 := rawCreatePostInThread(t, baseURL, strconv.FormatInt(thread.Id, 10), map[string]string{
			"X-User-Id":         author.String(),
			"X-Idempotency-Key": reqID,
		}, bodyA, "application/json")
		if st1 != http.StatusCreated {
			t.Fatalf("expected 201, got %d body=%s", st1, string(rb1))
		}

		// repeat with different body => 409
		st2, rb2 := rawCreatePostInThread(t, baseURL, strconv.FormatInt(thread.Id, 10), map[string]string{
			"X-User-Id":         author.String(),
			"X-Idempotency-Key": reqID,
		}, bodyB, "application/json")

		if st2 != http.StatusConflict {
			t.Fatalf("expected 409, got %d body=%s", st2, string(rb2))
		}
	})
}

// rawCreatePostInThread отправляет POST /api/v1/threads/{thread_id}/posts без типизированного клиента,
// чтобы проверять отсутствие/неверный формат заголовков, неправильный Content-Type, idempotency и т.п.
func rawCreatePostInThread(
	t *testing.T,
	baseURL string,
	threadID string, // строкой, чтобы можно было передать "0", "-1", "abc"
	headers map[string]string,
	body []byte,
	contentType string,
) (status int, respBody []byte) {
	t.Helper()

	u := strings.TrimRight(baseURL, "/") + "/api/v1/threads/" + threadID + "/posts"
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("[raw_create_post] new request: %v", err)
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	return do(t, req)
}
