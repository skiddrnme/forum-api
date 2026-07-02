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

func Test_Threads_Replace_Negative(t *testing.T) {
	// Чистим БД, чтобы тесты были воспроизводимыми.
	t.Run("[setup] truncate", func(t *testing.T) {
		c := newTestClient(t)

		truncateDB(t, c)
	})

	// Логинимся (авторегистрация) и получаем X-User-Id.
	var author, otherUser uuid.UUID
	t.Run("[setup] login", func(t *testing.T) {
		c := newTestClient(t)

		username := "user_" + randomSuffix()
		author = login(t, c, username, "password123")

		username = "user_" + randomSuffix()
		otherUser = login(t, c, username, "password123")
	})

	var orig *Thread
	t.Run("[setup] create thread", func(t *testing.T) {
		c := newTestClient(t)

		reqID := "thread" + "-" + randomSuffix()
		params := &CreateThreadParams{
			XUserId:         UserIdHeader(author),
			XIdempotencyKey: IdempotencyKeyHeader(reqID),
		}
		body := ThreadCreate{
			Title:   "thread for replace negative " + randomSuffix(),
			Content: "content for replace negative " + randomSuffix(),
			Tags:    &[]string{"go", "negative"},
		}

		orig = createThread(t, c, params, body)
	})

	baseURL := getBaseURL()

	t.Run("401_missing_X-User-Id", func(t *testing.T) {
		body, _ := json.Marshal(ThreadCreate{
			Title:   "x",
			Content: "y",
			Tags:    &[]string{"a"},
		})

		status, respBody := rawReplaceThread(t, baseURL, strconv.FormatInt(orig.Id, 10), map[string]string{
			// no X-User-Id
		}, body, "application/json")

		if status != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d body=%s", status, string(respBody))
		}
	})

	t.Run("401_invalid_X-User-Id_format", func(t *testing.T) {
		body, _ := json.Marshal(ThreadCreate{
			Title:   "x",
			Content: "y",
		})

		status, respBody := rawReplaceThread(t, baseURL, strconv.FormatInt(orig.Id, 10), map[string]string{
			"X-User-Id": "not-a-uuid",
		}, body, "application/json")

		if status != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d body=%s", status, string(respBody))
		}
	})

	t.Run("400_invalid_thread_id_zero", func(t *testing.T) {
		body, _ := json.Marshal(ThreadCreate{
			Title:   "x",
			Content: "y",
		})

		status, respBody := rawReplaceThread(t, baseURL, "0", map[string]string{
			"X-User-Id": author.String(),
		}, body, "application/json")

		if status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", status, string(respBody))
		}
	})

	t.Run("400_invalid_thread_id_non_int", func(t *testing.T) {
		body, _ := json.Marshal(ThreadCreate{
			Title:   "x",
			Content: "y",
		})

		status, respBody := rawReplaceThread(t, baseURL, "abc", map[string]string{
			"X-User-Id": author.String(),
		}, body, "application/json")

		if status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", status, string(respBody))
		}
	})

	t.Run("400_wrong_content_type", func(t *testing.T) {
		body := []byte(`title=x&content=y`)

		status, respBody := rawReplaceThread(t, baseURL, strconv.FormatInt(orig.Id, 10), map[string]string{
			"X-User-Id": author.String(),
		}, body, "application/x-www-form-urlencoded")

		if status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", status, string(respBody))
		}
	})

	t.Run("400_bad_json", func(t *testing.T) {
		status, respBody := rawReplaceThread(t, baseURL, strconv.FormatInt(orig.Id, 10), map[string]string{
			"X-User-Id": author.String(),
		}, []byte(`{"title":`), "application/json")

		if status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", status, string(respBody))
		}
	})

	t.Run("400_missing_required_fields", func(t *testing.T) {
		// ThreadCreate требует title+content
		status, respBody := rawReplaceThread(t, baseURL, strconv.FormatInt(orig.Id, 10), map[string]string{
			"X-User-Id": author.String(),
		}, []byte(`{}`), "application/json")

		if status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", status, string(respBody))
		}
	})

	t.Run("400_title_empty_or_spaces", func(t *testing.T) {
		body, _ := json.Marshal(ThreadCreate{
			Title:   "   ",
			Content: "ok",
		})

		status, respBody := rawReplaceThread(t, baseURL, strconv.FormatInt(orig.Id, 10), map[string]string{
			"X-User-Id": author.String(),
		}, body, "application/json")

		if status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", status, string(respBody))
		}
	})

	t.Run("400_title_too_long_256", func(t *testing.T) {
		title := strings.Repeat("a", 256)
		body, _ := json.Marshal(ThreadCreate{
			Title:   title,
			Content: "ok",
		})

		status, respBody := rawReplaceThread(t, baseURL, strconv.FormatInt(orig.Id, 10), map[string]string{
			"X-User-Id": author.String(),
		}, body, "application/json")

		if status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", status, string(respBody))
		}
	})

	t.Run("400_content_empty_or_spaces", func(t *testing.T) {
		body, _ := json.Marshal(ThreadCreate{
			Title:   "ok",
			Content: "   ",
		})

		status, respBody := rawReplaceThread(t, baseURL, strconv.FormatInt(orig.Id, 10), map[string]string{
			"X-User-Id": author.String(),
		}, body, "application/json")

		if status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", status, string(respBody))
		}
	})

	t.Run("400_content_too_long_10001", func(t *testing.T) {
		content := strings.Repeat("b", 10001)
		body, _ := json.Marshal(ThreadCreate{
			Title:   "ok",
			Content: content,
		})

		status, respBody := rawReplaceThread(t, baseURL, strconv.FormatInt(orig.Id, 10), map[string]string{
			"X-User-Id": author.String(),
		}, body, "application/json")

		if status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", status, string(respBody))
		}
	})

	t.Run("400_tags_wrong_type", func(t *testing.T) {
		// tags должны быть массивом строк, а тут строка
		status, respBody := rawReplaceThread(t, baseURL, strconv.FormatInt(orig.Id, 10), map[string]string{
			"X-User-Id": author.String(),
		}, []byte(`{"title":"ok","content":"ok","tags":"nope"}`), "application/json")

		if status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", status, string(respBody))
		}
	})

	t.Run("404_thread_not_found", func(t *testing.T) {
		body, _ := json.Marshal(ThreadCreate{
			Title:   "ok",
			Content: "ok",
		})

		status, respBody := rawReplaceThread(t, baseURL, "999999999", map[string]string{
			"X-User-Id": author.String(),
		}, body, "application/json")

		if status != http.StatusNotFound {
			t.Fatalf("expected 404, got %d body=%s", status, string(respBody))
		}
	})

	t.Run("403_replace_by_non_author", func(t *testing.T) {
		// В спецификации нет 403, права проверяются через X-User-Id.
		// На практике обычно возвращают 404 (скрываем факт существования ресурса).
		body, _ := json.Marshal(ThreadCreate{
			Title:   "new title " + randomSuffix(),
			Content: "new content " + randomSuffix(),
			Tags:    &[]string{"x"},
		})

		status, respBody := rawReplaceThread(t, baseURL, strconv.FormatInt(orig.Id, 10), map[string]string{
			"X-User-Id": otherUser.String(),
		}, body, "application/json")

		if status != http.StatusForbidden {
			t.Fatalf("expected 403, got %d body=%s", status, string(respBody))
		}
	})

	t.Run("idempotency_same_request_twice_returns_same_id", func(t *testing.T) {
		c := newTestClient(t)

		body := ThreadCreate{
			Title:   "PUT IDEMPOTENT " + randomSuffix(),
			Content: "PUT IDEMPOTENT CONTENT " + randomSuffix(),
			Tags:    &[]string{"put", "idem"},
		}

		updated1 := replaceThread(t, c, orig.Id, author, body)
		updated2 := replaceThread(t, c, orig.Id, author, body)

		if updated1.Id != orig.Id || updated2.Id != orig.Id {
			t.Fatalf("expected same thread id=%d, got updated1=%d updated2=%d", orig.Id, updated1.Id, updated2.Id)
		}

		// Не проверяем UpdatedAt, чтобы не завязываться на то,
		// обновляет ли сервер timestamp при повторном PUT.
		if updated2.Title != body.Title || updated2.Content != body.Content {
			t.Fatalf("expected state to match body after повторного PUT, got title=%q content=%q", updated2.Title, updated2.Content)
		}
	})
}

// rawReplaceThread отправляет PUT /api/v1/threads/{thread_id} без типизированного клиента,
// чтобы проверять отсутствие/неверный формат заголовков, неправильный Content-Type и т.п.
func rawReplaceThread(
	t *testing.T,
	baseURL string,
	threadID string, // строкой, чтобы можно было передать "0", "-1", "abc"
	headers map[string]string,
	body []byte,
	contentType string,
) (status int, respBody []byte) {
	t.Helper()

	u := strings.TrimRight(baseURL, "/") + "/api/v1/threads/" + threadID
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPut, u, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("[raw_replace_thread] new request: %v", err)
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	return do(t, req)
}
