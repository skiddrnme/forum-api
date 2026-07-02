//go:build e2e

package tests

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"

	. "stepik.leoscode.http/internal/gen/api"
)

// Негативные тесты на [GET] /api/v1/threads/{thread_id}
func Test_Threads_GetByID_Negative(t *testing.T) {
	// Чистим БД, чтобы тесты были воспроизводимыми.
	t.Run("[setup] truncate", func(t *testing.T) {
		c := newTestClient(t)
		truncateDB(t, c)
	})

	// Логинимся (авторегистрация) и получаем X-User-Id (для optional header).
	var author uuid.UUID
	t.Run("[setup] login", func(t *testing.T) {
		c := newTestClient(t)
		author = login(t, c, "user_"+randomSuffix(), "password123")
	})

	// Создаём тред (чтобы можно было проверить валидный thread_id при raw кейсах).
	var orig *Thread
	t.Run("[setup] create thread", func(t *testing.T) {
		c := newTestClient(t)

		reqID := "thread" + "-" + randomSuffix()
		params := &CreateThreadParams{
			XUserId:         UserIdHeader(author),
			XIdempotencyKey: IdempotencyKeyHeader(reqID),
		}
		body := ThreadCreate{
			Title:   "thread for get negative " + randomSuffix(),
			Content: "content for get negative " + randomSuffix(),
			Tags:    &[]string{"go", "negative"},
		}

		orig = createThread(t, c, params, body)
	})

	baseURL := getBaseURL()

	t.Run("not_found_returns_404", func(t *testing.T) {
		c := newTestClient(t)

		// гарантированно несуществующий id
		missingID := orig.Id + 10_000

		resp, err := c.GetThreadWithResponse(t.Context(), missingID, nil)
		if err != nil {
			t.Fatalf("request error: %v", err)
		}
		if resp.StatusCode() != http.StatusNotFound {
			t.Fatalf("expected 404, got %d; body=%s", resp.StatusCode(), string(resp.Body))
		}
	})

	t.Run("bad_thread_id_zero_returns_400", func(t *testing.T) {
		status, body := rawGetThread(t, baseURL, "0", nil, "application/json")
		if status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d; body=%s", status, body)
		}
	})

	t.Run("bad_thread_id_not_number_returns_400", func(t *testing.T) {

		status, body := rawGetThread(t, baseURL, "not-a-number", nil, "application/json")
		// если роутер отдаёт 404 — тоже ок, но по нашим тестам ожидаем 400 как и в других методах
		if status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d; body=%s", status, body)
		}
	})

	t.Run("invalid_optional_X_User_Id_returns_401", func(t *testing.T) {
		h := map[string]string{
			"X-User-Id": "not-a-uuid",
		}
		status, body := rawGetThread(t, baseURL, itoa64(orig.Id), h, "application/json")
		if status != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d; body=%s", status, body)
		}
	})

	t.Run("missing_optional_X_User_Id_is_ok_200", func(t *testing.T) {
		status, body := rawGetThread(t, baseURL, itoa64(orig.Id), nil, "application/json")
		if status != http.StatusOK {
			t.Fatalf("expected 200, got %d; body=%s", status, body)
		}
		// базовая проверка, что вернулся именно нужный тред
		if !strings.Contains(string(body), `"id":`+itoa64(orig.Id)) {
			t.Fatalf("expected response to contain id=%d, body=%s", orig.Id, body)
		}
	})
}

func itoa64(v int64) string { return strconv.FormatInt(v, 10) }

// rawGetThread выполняет raw HTTP запрос и возвращает status + body (строкой, удобной для логов).
func rawGetThread(
	t *testing.T,
	baseURL string,
	threadID string,
	headers map[string]string,
	contentType string,
) (int, []byte) {
	t.Helper()

	u := strings.TrimRight(baseURL, "/") + "/api/v1/threads/" + threadID
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, u, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	return do(t, req)
}
