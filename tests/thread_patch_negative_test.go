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

func Test_Threads_Patch_Negative(t *testing.T) {
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
			Title:   "thread for patch negative " + randomSuffix(),
			Content: "content for patch negative " + randomSuffix(),
			Tags:    &[]string{"go", "negative"},
		}

		orig = createThread(t, c, params, body)
	})

	baseURL := getBaseURL()

	t.Run("401_missing_X-User-Id", func(t *testing.T) {
		body := []byte(`{"title":"x"}`)

		status, respBody := rawPatchThread(t, baseURL, strconv.FormatInt(orig.Id, 10), nil, body, "application/json")
		if status != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d body=%s", status, string(respBody))
		}
	})

	t.Run("401_invalid_X-User-Id_format", func(t *testing.T) {
		body := []byte(`{"title":"x"}`)

		status, respBody := rawPatchThread(t, baseURL, strconv.FormatInt(orig.Id, 10), map[string]string{
			"X-User-Id": "not-a-uuid",
		}, body, "application/json")
		if status != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d body=%s", status, string(respBody))
		}
	})

	t.Run("400_thread_id_not_number", func(t *testing.T) {
		body := []byte(`{"title":"x"}`)

		status, respBody := rawPatchThread(t, baseURL, "nope", map[string]string{
			"X-User-Id": author.String(),
		}, body, "application/json")
		if status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", status, string(respBody))
		}
	})

	t.Run("400_thread_id_zero", func(t *testing.T) {
		body := []byte(`{"title":"x"}`)

		status, respBody := rawPatchThread(t, baseURL, "0", map[string]string{
			"X-User-Id": author.String(),
		}, body, "application/json")
		if status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", status, string(respBody))
		}
	})

	t.Run("400_wrong_content_type", func(t *testing.T) {
		body := []byte(`{"title":"x"}`)

		status, respBody := rawPatchThread(t, baseURL, strconv.FormatInt(orig.Id, 10), map[string]string{
			"X-User-Id": author.String(),
		}, body, "text/plain")
		if status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", status, string(respBody))
		}
	})

	t.Run("400_bad_json", func(t *testing.T) {
		body := []byte(`{"title":`)

		status, respBody := rawPatchThread(t, baseURL, strconv.FormatInt(orig.Id, 10), map[string]string{
			"X-User-Id": author.String(),
		}, body, "application/json")
		if status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", status, string(respBody))
		}
	})

	t.Run("400_empty_object_does_not_match_anyOf", func(t *testing.T) {
		body := []byte(`{}`)

		status, respBody := rawPatchThread(t, baseURL, strconv.FormatInt(orig.Id, 10), map[string]string{
			"X-User-Id": author.String(),
		}, body, "application/json")
		if status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", status, string(respBody))
		}
	})

	t.Run("400_title_empty", func(t *testing.T) {
		body := []byte(`{"title":"   "}`)

		status, respBody := rawPatchThread(t, baseURL, strconv.FormatInt(orig.Id, 10), map[string]string{
			"X-User-Id": author.String(),
		}, body, "application/json")
		if status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", status, string(respBody))
		}
	})

	t.Run("400_title_too_long", func(t *testing.T) {
		long := strings.Repeat("a", 256)
		body, _ := json.Marshal(map[string]any{"title": long})

		status, respBody := rawPatchThread(t, baseURL, strconv.FormatInt(orig.Id, 10), map[string]string{
			"X-User-Id": author.String(),
		}, body, "application/json")
		if status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", status, string(respBody))
		}
	})

	t.Run("400_content_empty", func(t *testing.T) {
		body := []byte(`{"content":"   "}`)

		status, respBody := rawPatchThread(t, baseURL, strconv.FormatInt(orig.Id, 10), map[string]string{
			"X-User-Id": author.String(),
		}, body, "application/json")
		if status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", status, string(respBody))
		}
	})

	t.Run("400_content_too_long", func(t *testing.T) {
		long := strings.Repeat("a", 10001)
		body, _ := json.Marshal(map[string]any{"content": long})

		status, respBody := rawPatchThread(t, baseURL, strconv.FormatInt(orig.Id, 10), map[string]string{
			"X-User-Id": author.String(),
		}, body, "application/json")
		if status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", status, string(respBody))
		}
	})

	t.Run("400_tags_not_array", func(t *testing.T) {
		body := []byte(`{"tags":"nope"}`)

		status, respBody := rawPatchThread(t, baseURL, strconv.FormatInt(orig.Id, 10), map[string]string{
			"X-User-Id": author.String(),
		}, body, "application/json")
		if status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", status, string(respBody))
		}
	})

	t.Run("400_tags_too_many", func(t *testing.T) {
		tags := make([]string, 0, 11)
		for i := 0; i < 11; i++ {
			tags = append(tags, "t"+strconv.Itoa(i))
		}
		body, _ := json.Marshal(map[string]any{"tags": tags})

		status, respBody := rawPatchThread(t, baseURL, strconv.FormatInt(orig.Id, 10), map[string]string{
			"X-User-Id": author.String(),
		}, body, "application/json")
		if status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", status, string(respBody))
		}
	})

	t.Run("400_tag_item_empty", func(t *testing.T) {
		body := []byte(`{"tags":["ok",""]}`)

		status, respBody := rawPatchThread(t, baseURL, strconv.FormatInt(orig.Id, 10), map[string]string{
			"X-User-Id": author.String(),
		}, body, "application/json")
		if status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", status, string(respBody))
		}
	})

	t.Run("400_tag_item_too_long", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{
			"tags": []string{strings.Repeat("a", 33)},
		})

		status, respBody := rawPatchThread(t, baseURL, strconv.FormatInt(orig.Id, 10), map[string]string{
			"X-User-Id": author.String(),
		}, body, "application/json")
		if status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", status, string(respBody))
		}
	})

	t.Run("400_is_locked_wrong_type", func(t *testing.T) {
		body := []byte(`{"is_locked":"true"}`)

		status, respBody := rawPatchThread(t, baseURL, strconv.FormatInt(orig.Id, 10), map[string]string{
			"X-User-Id": author.String(),
		}, body, "application/json")
		if status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", status, string(respBody))
		}
	})

	t.Run("404_thread_not_found", func(t *testing.T) {
		body := []byte(`{"title":"x"}`)

		status, respBody := rawPatchThread(t, baseURL, "9999999", map[string]string{
			"X-User-Id": author.String(),
		}, body, "application/json")
		if status != http.StatusNotFound {
			t.Fatalf("expected 404, got %d body=%s", status, string(respBody))
		}
	})

	t.Run("403_patch_by_not_author", func(t *testing.T) {
		body := []byte(`{"title":"x"}`)

		status, respBody := rawPatchThread(t, baseURL, strconv.FormatInt(orig.Id, 10), map[string]string{
			"X-User-Id": otherUser.String(),
		}, body, "application/json")
		if status != http.StatusForbidden {
			t.Fatalf("expected 403, got %d body=%s", status, string(respBody))
		}
	})

	t.Run("403_thread_locked_when_patching_title", func(t *testing.T) {
		// lock thread
		lockBody := []byte(`{"is_locked":true}`)
		status, respBody := rawPatchThread(t, baseURL, strconv.FormatInt(orig.Id, 10), map[string]string{
			"X-User-Id": author.String(),
		}, lockBody, "application/json")
		if status != http.StatusOK {
			t.Fatalf("expected 200 while locking thread, got %d body=%s", status, string(respBody))
		}

		// now patch title should be forbidden
		body := []byte(`{"title":"cannot edit"}`)
		status, respBody = rawPatchThread(t, baseURL, strconv.FormatInt(orig.Id, 10), map[string]string{
			"X-User-Id": author.String(),
		}, body, "application/json")
		if status != http.StatusForbidden {
			t.Fatalf("expected 403, got %d body=%s", status, string(respBody))
		}

		// unlock back to not affect other tests
		unlockBody := []byte(`{"is_locked":false}`)
		status, respBody = rawPatchThread(t, baseURL, strconv.FormatInt(orig.Id, 10), map[string]string{
			"X-User-Id": author.String(),
		}, unlockBody, "application/json")
		if status != http.StatusOK {
			t.Fatalf("expected 200 while unlocking thread, got %d body=%s", status, string(respBody))
		}
	})
}

func rawPatchThread(
	t *testing.T,
	baseURL string,
	threadID string,
	headers map[string]string,
	body []byte,
	contentType string,
) (int, []byte) {
	t.Helper()

	u := strings.TrimRight(baseURL, "/") + "/api/v1/threads/" + threadID
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPatch, u, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("[raw_patch_thread] new request: %v", err)
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	return do(t, req)
}
