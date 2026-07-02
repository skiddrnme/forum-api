//go:build e2e

package tests

import (
	"bytes"
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"

	. "stepik.leoscode.http/internal/gen/api"
)

// Негативные тесты для [DELETE] /api/v1/threads/{thread_id}
//
// В swagger: ответы 204, 401, 404, 500. Валидация thread_id (minimum=1) и формата X-User-Id
// даёт 400 на "битые" входные данные — это нормальная серверная практика и соответствует остальным хендлерам.
func Test_Threads_Delete_Negative(t *testing.T) {
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

	// Создаём тред (нужен для проверок 204 и 404 "не автор")
	var orig *Thread
	t.Run("[setup] create thread", func(t *testing.T) {
		c := newTestClient(t)

		reqID := "thread" + "-" + randomSuffix()
		params := &CreateThreadParams{
			XUserId:         UserIdHeader(author),
			XIdempotencyKey: IdempotencyKeyHeader(reqID),
		}
		body := ThreadCreate{
			Title:   "thread for delete negative " + randomSuffix(),
			Content: "content for delete negative " + randomSuffix(),
			Tags:    &[]string{"go", "negative"},
		}

		orig = createThreadNegHelper(t, c, params, body)
	})

	baseURL := getBaseURL()

	t.Run("401_missing_x_user_id", func(t *testing.T) {
		res := doRawDelete(t, baseURL+"/api/v1/threads/1", nil)
		if res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", res.StatusCode)
		}
	})

	t.Run("401_invalid_x_user_id_format", func(t *testing.T) {
		h := http.Header{}
		h.Set("X-User-Id", "not-a-uuid")
		res := doRawDelete(t, baseURL+"/api/v1/threads/1", h)
		if res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", res.StatusCode)
		}
	})

	t.Run("400_thread_id_not_a_number", func(t *testing.T) {
		h := http.Header{}
		h.Set("X-User-Id", author.String())
		res := doRawDelete(t, baseURL+"/api/v1/threads/not-a-number", h)
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", res.StatusCode)
		}
	})

	t.Run("400_thread_id_zero", func(t *testing.T) {
		h := http.Header{}
		h.Set("X-User-Id", author.String())
		res := doRawDelete(t, baseURL+"/api/v1/threads/0", h)
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", res.StatusCode)
		}
	})

	t.Run("404_not_found", func(t *testing.T) {
		c := newTestClient(t)

		resp, err := c.DeleteThreadWithResponse(t.Context(), ThreadIdPath(999999999), &DeleteThreadParams{
			XUserId: UserIdHeader(author),
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode() != http.StatusNotFound {
			t.Fatalf("expected 404, got %d body=%s", resp.StatusCode(), string(resp.Body))
		}
		if resp.JSON404 == nil {
			t.Fatalf("expected JSON404, got nil (body=%s)", string(resp.Body))
		}
	})

	t.Run("404_delete_not_author", func(t *testing.T) {
		c := newTestClient(t)

		resp, err := c.DeleteThreadWithResponse(t.Context(), ThreadIdPath(orig.Id), &DeleteThreadParams{
			XUserId: UserIdHeader(otherUser),
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode() != http.StatusNotFound {
			t.Fatalf("expected 404, got %d body=%s", resp.StatusCode(), string(resp.Body))
		}
	})

	t.Run("204_delete_ok_then_get_is_404", func(t *testing.T) {
		c := newTestClient(t)

		// delete ok
		resp, err := c.DeleteThreadWithResponse(t.Context(), ThreadIdPath(orig.Id), &DeleteThreadParams{
			XUserId: UserIdHeader(author),
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode() != http.StatusNoContent {
			t.Fatalf("expected 204, got %d body=%s", resp.StatusCode(), string(resp.Body))
		}

		// confirm deleted
		getResp, err := c.GetThreadWithResponse(context.Background(), ThreadIdPath(orig.Id), &GetThreadParams{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if getResp.StatusCode() != http.StatusNotFound {
			t.Fatalf("expected 404 after delete, got %d body=%s", getResp.StatusCode(), string(getResp.Body))
		}
	})
}

// createThreadNegHelper — локальный helper для создания треда (в happy-path файле его нет).
func createThreadNegHelper(
	t *testing.T,
	c ClientWithResponsesInterface,
	params *CreateThreadParams,
	body ThreadCreate,
) *Thread {
	t.Helper()
	const api = "[threads.create]"

	resp, err := c.CreateThreadWithResponse(t.Context(), params, body)
	if err != nil {
		t.Fatalf("%s unexpected error: %v", api, err)
	}
	if resp.StatusCode() != http.StatusCreated {
		t.Fatalf("%s unexpected status: %d body=%s", api, resp.StatusCode(), string(resp.Body))
	}
	if resp.JSON201 == nil {
		t.Fatalf("%s JSON201 is nil", api)
	}
	return resp.JSON201
}

func doRawDelete(t *testing.T, url string, h http.Header) *http.Response {
	t.Helper()

	req, err := http.NewRequest(http.MethodDelete, url, bytes.NewReader(nil))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if h != nil {
		req.Header = h
	}

	cl := &http.Client{}
	res, err := cl.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	return res
}
