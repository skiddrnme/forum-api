//go:build e2e

package tests

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	. "stepik.leoscode.http/internal/gen/api"
)

func Test_Threads_List_Negative(t *testing.T) {
	t.Run("[setup] truncate", func(t *testing.T) {
		c := newTestClient(t)
		truncateDB(t, c)
	})

	var author uuid.UUID
	t.Run("[setup] login", func(t *testing.T) {
		c := newTestClient(t)
		author = login(t, c, "user_"+randomSuffix(), "password123")
	})

	t.Run("[setup] create threads", func(t *testing.T) {
		c := newTestClient(t)

		for i := 0; i < 3; i++ {
			params := &CreateThreadParams{
				XUserId:         UserIdHeader(author),
				XIdempotencyKey: IdempotencyKeyHeader("thread-" + randomSuffix()),
			}
			body := ThreadCreate{
				Title:   fmt.Sprintf("thread %d %s", i, randomSuffix()),
				Content: "content " + randomSuffix(),
				Tags:    &[]string{"go", "list"},
			}
			createThread(t, c, params, body)
		}
	})

	baseURL := getBaseURL()

	t.Run("bad query params => 400", func(t *testing.T) {
		cases := []struct {
			name string
			url  string
		}{
			{"limit=0", threadsURL(baseURL, "limit=0")},
			{"limit too big", threadsURL(baseURL, "limit=1000")},
			{"limit not a number", threadsURL(baseURL, "limit=abc")},
			{"offset negative", threadsURL(baseURL, "offset=-1")},
			{"offset not a number", threadsURL(baseURL, "offset=abc")},
			{"author_id not uuid", threadsURL(baseURL, "author_id=not-a-uuid")},
			{"tag empty", threadsURL(baseURL, "tag=")},
			{"tag too long", threadsURL(baseURL, "tag="+strings.Repeat("a", 40))},
			{"tag invalid chars", threadsURL(baseURL, "tag=go-lang")},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				getAndExpect(t, tc.url, nil, http.StatusBadRequest)
			})
		}
	})

	t.Run("ok without params", func(t *testing.T) {
		resp := getAndExpect(t, threadsURL(baseURL, ""), nil, http.StatusOK)

		var out ThreadListResponse
		decodeJSON(t, resp, &out)

		if len(out.Items) == 0 {
			t.Fatalf("expected non-empty thread list")
		}
	})

	t.Run("ok empty result for unknown tag", func(t *testing.T) {
		resp := getAndExpect(t, threadsURL(baseURL, "tag=unknown"), nil, http.StatusOK)

		var out ThreadListResponse
		decodeJSON(t, resp, &out)

		if len(out.Items) != 0 {
			t.Fatalf("expected empty list, got %d", len(out.Items))
		}
	})
}

func threadsURL(baseURL, query string) string {
	if query == "" {
		return baseURL + "/api/v1/threads"
	}
	return baseURL + "/api/v1/threads?" + query
}

func getAndExpect(t *testing.T, url string, headers map[string]string, wantStatus int) *http.Response {
	t.Helper()

	resp := rawGET(t, url, headers)

	if resp.StatusCode != wantStatus {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status %d, got %d, body=%s", wantStatus, resp.StatusCode, string(body))
	}

	return resp
}

func rawGET(t *testing.T, url string, headers map[string]string) *http.Response {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	httpClient := newTracingHTTPClient(t, http.DefaultTransport)

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}

	t.Cleanup(func() { resp.Body.Close() })

	return resp
}

func decodeJSON(t *testing.T, resp *http.Response, v any) {
	t.Helper()

	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decode body: %v", err)
	}
}
