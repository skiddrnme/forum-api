//go:build e2e

package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	. "stepik.leoscode.http/internal/gen/api"
)

func Test_Thread_Create_Negative(t *testing.T) {
	// Чистим БД, чтобы тесты были воспроизводимыми.
	t.Run("[setup] truncate", func(t *testing.T) {
		c := newTestClient(t)

		truncateDB(t, c)
	})

	// Логинимся (авторегистрация) и получаем X-User-Id.
	var (
		username = "user_" + randomSuffix()
		userID   uuid.UUID
	)
	t.Run("[setup] login", func(t *testing.T) {
		c := newTestClient(t)

		userID = login(t, c, username, "password123")
	})

	t.Run("[POST]/api/v1/threads negative", func(t *testing.T) {
		const api = "[threads.create negative]"

		// Валидное тело (будем от него плясать)
		okBody := ThreadCreate{
			Title:   "hello " + randomSuffix(),
			Content: "content " + randomSuffix(),
			Tags:    &[]string{"go", "tests"},
		}

		type tc struct {
			Name       string
			Params     *CreateThreadParams
			Body       *ThreadCreate
			WantStatus int
			Assert     func(t *testing.T, resp *CreateThreadResp)
		}

		// Удобные заготовки строк
		long := func(n int) string { return strings.Repeat("a", n) }

		cases := []tc{
			{
				Name: "empty_x_request_id",
				Params: &CreateThreadParams{
					XUserId:         UserIdHeader(userID),
					XIdempotencyKey: "",
				},
				Body:       &okBody,
				WantStatus: http.StatusBadRequest,
				Assert: func(t *testing.T, resp *CreateThreadResp) {
					if resp.JSON400 == nil {
						t.Fatalf("%s: expected JSON400, got nil (body=%s)", api, string(resp.Body))
					}
				},
			},
			{
				Name: "x_request_id_too_long_129",
				Params: &CreateThreadParams{
					XUserId:         UserIdHeader(userID),
					XIdempotencyKey: long(129),
				},
				Body:       &okBody,
				WantStatus: http.StatusBadRequest,
				Assert: func(t *testing.T, resp *CreateThreadResp) {
					if resp.JSON400 == nil {
						t.Fatalf("%s: expected JSON400, got nil (body=%s)", api, string(resp.Body))
					}
				},
			},
			{
				Name: "title_empty",
				Params: &CreateThreadParams{
					XUserId:         UserIdHeader(userID),
					XIdempotencyKey: "req-" + randomSuffix(),
				},
				Body: &ThreadCreate{
					Title:   "",
					Content: okBody.Content,
					Tags:    okBody.Tags,
				},
				WantStatus: http.StatusBadRequest,
				Assert: func(t *testing.T, resp *CreateThreadResp) {
					if resp.JSON400 == nil {
						t.Fatalf("%s: expected JSON400, got nil (body=%s)", api, string(resp.Body))
					}
				},
			},
			{
				Name: "title_too_long_256",
				Params: &CreateThreadParams{
					XUserId:         UserIdHeader(userID),
					XIdempotencyKey: "req-" + randomSuffix(),
				},
				Body: &ThreadCreate{
					Title:   long(256), // maxLength=255
					Content: okBody.Content,
					Tags:    okBody.Tags,
				},
				WantStatus: http.StatusBadRequest,
				Assert: func(t *testing.T, resp *CreateThreadResp) {
					if resp.JSON400 == nil {
						t.Fatalf("%s: expected JSON400, got nil (body=%s)", api, string(resp.Body))
					}
				},
			},
			{
				Name: "content_empty",
				Params: &CreateThreadParams{
					XUserId:         UserIdHeader(userID),
					XIdempotencyKey: "req-" + randomSuffix(),
				},
				Body: &ThreadCreate{
					Title:   okBody.Title,
					Content: "",
					Tags:    okBody.Tags,
				},
				WantStatus: http.StatusBadRequest,
				Assert: func(t *testing.T, resp *CreateThreadResp) {
					if resp.JSON400 == nil {
						t.Fatalf("%s: expected JSON400, got nil (body=%s)", api, string(resp.Body))
					}
				},
			},
			{
				Name: "content_too_long_10001",
				Params: &CreateThreadParams{
					XUserId:         UserIdHeader(userID),
					XIdempotencyKey: "req-" + randomSuffix(),
				},
				Body: &ThreadCreate{
					Title:   okBody.Title,
					Content: long(10001), // maxLength=10000
					Tags:    okBody.Tags,
				},
				WantStatus: http.StatusBadRequest,
				Assert: func(t *testing.T, resp *CreateThreadResp) {
					if resp.JSON400 == nil {
						t.Fatalf("%s: expected JSON400, got nil (body=%s)", api, string(resp.Body))
					}
				},
			},
		}

		for _, tt := range cases {
			t.Run(tt.Name, func(t *testing.T) {
				c := newTestClient(t)

				resp, err := c.CreateThreadWithResponse(t.Context(), tt.Params, *tt.Body)
				if err != nil {
					t.Fatalf("%s: CreateThread error: %v", api, err)
				}
				if resp.StatusCode() != tt.WantStatus {
					t.Fatalf("%s: status=%d want=%d body=%s", api, resp.StatusCode(), tt.WantStatus, string(resp.Body))
				}
				if tt.Assert != nil {
					tt.Assert(t, resp)
				}
			})
		}
	})

	t.Run("[POST]/api/v1/threads raw negative (headers/content-type/json)", func(t *testing.T) {
		const api = "[threads.create raw negative]"
		baseURL := getBaseURL()

		ok := map[string]any{
			"title":   "raw " + randomSuffix(),
			"content": "raw content " + randomSuffix(),
			"tags":    []string{"go"},
		}
		okJSON, _ := json.Marshal(ok)

		t.Run("missing_x_user_id_header", func(t *testing.T) {
			status, body := rawCreateThread(t, baseURL, map[string]string{
				"X-Request-Id": "req-" + randomSuffix(),
			}, okJSON, "application/json")

			if status != http.StatusUnauthorized && status != http.StatusBadRequest {
				t.Fatalf("%s: status=%d want=401-or-400 body=%s", api, status, string(body))
			}
		})

		t.Run("missing_x_request_id_header", func(t *testing.T) {
			status, body := rawCreateThread(t, baseURL, map[string]string{
				"X-User-Id": userID.String(),
			}, okJSON, "application/json")

			if status != http.StatusBadRequest {
				t.Fatalf("%s: status=%d want=400 body=%s", api, status, string(body))
			}
		})

		t.Run("invalid_x_user_id_format", func(t *testing.T) {
			status, body := rawCreateThread(t, baseURL, map[string]string{
				"X-User-Id":    "not-a-uuid",
				"X-Request-Id": "req-" + randomSuffix(),
			}, okJSON, "application/json")

			// В спеках это required uuid+pattern, сервер обычно отвечает 401 или 400.
			if status != http.StatusUnauthorized && status != http.StatusBadRequest {
				t.Fatalf("%s: status=%d want=401-or-400 body=%s", api, status, string(body))
			}
		})

		t.Run("wrong_content_type", func(t *testing.T) {
			status, body := rawCreateThread(t, baseURL, map[string]string{
				"X-User-Id":    userID.String(),
				"X-Request-Id": "req-" + randomSuffix(),
			}, okJSON, "text/plain")

			if status != http.StatusBadRequest {
				t.Fatalf("%s: status=%d want=400 body=%s", api, status, string(body))
			}
		})

		t.Run("invalid_json", func(t *testing.T) {
			status, body := rawCreateThread(t, baseURL, map[string]string{
				"X-User-Id":    userID.String(),
				"X-Request-Id": "req-" + randomSuffix(),
			}, []byte(`{"title":`), "application/json")

			if status != http.StatusBadRequest {
				t.Fatalf("%s: status=%d want=400 body=%s", api, status, string(body))
			}
		})

		t.Run("tags_wrong_type_not_array", func(t *testing.T) {
			bad := map[string]any{
				"title":   "raw " + randomSuffix(),
				"content": "raw content",
				"tags":    "go", // должно быть array
			}
			badJSON, _ := json.Marshal(bad)

			status, body := rawCreateThread(t, baseURL, map[string]string{
				"X-User-Id":    userID.String(),
				"X-Request-Id": "req-" + randomSuffix(),
			}, badJSON, "application/json")

			if status != http.StatusBadRequest {
				t.Fatalf("%s: status=%d want=400 body=%s", api, status, string(body))
			}
		})
	})

	t.Run("[POST]/api/v1/threads idempotency", func(t *testing.T) {
		const api = "[threads.create idempotency]"

		c := newTestClient(t)

		body1 := ThreadCreate{
			Title:   "idem " + randomSuffix(),
			Content: "idem content " + randomSuffix(),
			Tags:    &[]string{"idempotency"},
		}

		reqID := "idem-" + randomSuffix()
		params := &CreateThreadParams{
			XUserId:         UserIdHeader(userID),
			XIdempotencyKey: reqID,
		}

		// 1) Первый запрос: 201
		resp1, err := c.CreateThreadWithResponse(t.Context(), params, body1)
		if err != nil {
			t.Fatalf("%s: first CreateThread error: %v", api, err)
		}
		if resp1.StatusCode() != http.StatusCreated {
			t.Fatalf("%s: first status=%d want=201 body=%s", api, resp1.StatusCode(), string(resp1.Body))
		}
		if resp1.JSON201 == nil {
			t.Fatalf("%s: first JSON201 is nil", api)
		}
		created := resp1.JSON201

		// 2) Повтор с тем же X-Request-Id и тем же телом: 200 и тот же тред
		resp2, err := c.CreateThreadWithResponse(t.Context(), params, body1)
		if err != nil {
			t.Fatalf("%s: second CreateThread error: %v", api, err)
		}
		if resp2.StatusCode() != http.StatusOK {
			t.Fatalf("%s: second status=%d want=200 body=%s", api, resp2.StatusCode(), string(resp2.Body))
		}
		if resp2.JSON200 == nil {
			t.Fatalf("%s: second JSON200 is nil", api)
		}
		same := resp2.JSON200

		if created.Id != same.Id {
			t.Fatalf("%s: idempotency broken: id1=%d id2=%d", api, created.Id, same.Id)
		}
		if created.Title != same.Title || created.Content != same.Content {
			t.Fatalf("%s: idempotency broken: different payload returned", api)
		}

		// 3) Тот же X-Request-Id, но другое тело: 409
		body2 := ThreadCreate{
			Title:   body1.Title + " changed",
			Content: body1.Content,
			Tags:    body1.Tags,
		}
		resp3, err := c.CreateThreadWithResponse(t.Context(), params, body2)
		if err != nil {
			t.Fatalf("%s: third CreateThread error: %v", api, err)
		}
		if resp3.StatusCode() != http.StatusConflict {
			t.Fatalf("%s: third status=%d want=409 body=%s", api, resp3.StatusCode(), string(resp3.Body))
		}
		if resp3.JSON409 == nil {
			t.Fatalf("%s: expected JSON409, got nil (body=%s)", api, string(resp3.Body))
		}
	})
}

// rawCreateThread позволяет отправить запрос без ограничений типизированного клиента
// (например, чтобы проверить отсутствие/неверный формат заголовков или неправильный Content-Type).
func rawCreateThread(t *testing.T, baseURL string, headers map[string]string, body []byte, contentType string) (status int, respBody []byte) {
	t.Helper()

	u := strings.TrimRight(baseURL, "/") + "/api/v1/threads"
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("[rawCreateThread]: new request: %v", err)
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	return do(t, req)
}
