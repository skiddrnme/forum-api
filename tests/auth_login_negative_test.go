//go:build e2e

package tests

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	. "stepik.leoscode.http/internal/gen/api"
)

// Негативные сценарии для [POST]/api/v1/auth/login.
// Спека: username (3..32, ^[a-zA-Z0-9_]+$), password (8..64), requestBody required,
// content-type: application/x-www-form-urlencoded, ответы: 200/400/401/500.
func Test_Auth_Login_Negative(t *testing.T) {
	c := newTestClient(t)

	// Чистим БД, чтобы поведение "авторегистрация" не зависело от внешнего состояния.
	truncateDB(t, c)

	baseURL := getBaseURL()

	// --- act: создаём пользователя корректным логином ---
	var username = "user_neg_" + randomSuffix()
	const password = "password123"
	_ = login(t, c, username, password)

	t.Run("400_bad_request_validation", func(t *testing.T) {
		type tc struct {
			Name        string
			Form        url.Values
			ContentType string
			WantCodes   []ErrorResponseCode
		}

		long33 := strings.Repeat("a", 33)
		long65 := strings.Repeat("p", 65)

		cases := []tc{
			{
				Name:        "missing_body",
				Form:        nil, // nil body
				ContentType: "application/x-www-form-urlencoded",
				// Сервер может трактовать как bad_request (нет тела) или validation_error (нет обязательных полей).
				WantCodes: []ErrorResponseCode{BadRequest, ValidationError},
			},
			{
				Name:        "wrong_content_type_json",
				Form:        nil, // будет JSON body ниже
				ContentType: "application/json",
				WantCodes:   []ErrorResponseCode{BadRequest, ValidationError},
			},
			{
				Name:        "missing_username",
				Form:        url.Values{"password": []string{password}},
				ContentType: "application/x-www-form-urlencoded",
				WantCodes:   []ErrorResponseCode{ValidationError, BadRequest},
			},
			{
				Name:        "missing_password",
				Form:        url.Values{"username": []string{username}},
				ContentType: "application/x-www-form-urlencoded",
				WantCodes:   []ErrorResponseCode{ValidationError, BadRequest},
			},
			{
				Name:        "username_too_short",
				Form:        url.Values{"username": []string{"ab"}, "password": []string{password}},
				ContentType: "application/x-www-form-urlencoded",
				WantCodes:   []ErrorResponseCode{ValidationError},
			},
			{
				Name:        "username_too_long_33",
				Form:        url.Values{"username": []string{long33}, "password": []string{password}},
				ContentType: "application/x-www-form-urlencoded",
				WantCodes:   []ErrorResponseCode{ValidationError},
			},
			{
				Name:        "username_invalid_pattern_dash",
				Form:        url.Values{"username": []string{"ab-"}, "password": []string{password}},
				ContentType: "application/x-www-form-urlencoded",
				WantCodes:   []ErrorResponseCode{ValidationError},
			},
			{
				Name:        "username_invalid_pattern_space",
				Form:        url.Values{"username": []string{"abc def"}, "password": []string{password}},
				ContentType: "application/x-www-form-urlencoded",
				WantCodes:   []ErrorResponseCode{ValidationError},
			},
			{
				Name:        "password_too_short_7",
				Form:        url.Values{"username": []string{"abc"}, "password": []string{"1234567"}},
				ContentType: "application/x-www-form-urlencoded",
				WantCodes:   []ErrorResponseCode{ValidationError},
			},
			{
				Name:        "password_too_long_65",
				Form:        url.Values{"username": []string{"abc"}, "password": []string{long65}},
				ContentType: "application/x-www-form-urlencoded",
				WantCodes:   []ErrorResponseCode{ValidationError},
			},
		}

		for _, tt := range cases {
			t.Run(tt.Name, func(t *testing.T) {
				var (
					body io.Reader
					ct   = tt.ContentType
				)

				switch tt.Name {
				case "wrong_content_type_json":
					body = bytes.NewReader([]byte(`{"username":"abc","password":"password123"}`))
				case "missing_body":
					body = nil
				default:
					encoded := ""
					if tt.Form != nil {
						encoded = tt.Form.Encode()
					}
					body = strings.NewReader(encoded)
				}

				status, respBody := doRawLoginRequest(t, baseURL, body, ct)

				if status != http.StatusBadRequest {
					t.Fatalf("expected status=%d got=%d, body=%s", http.StatusBadRequest, status, string(respBody))
				}

				assertErrorCodeIn(t, respBody, tt.WantCodes...)
			})
		}
	})

	t.Run("401_invalid_credentials", func(t *testing.T) {
		const api = "[login]"

		resp, err := c.LoginWithFormdataBodyWithResponse(t.Context(), LoginFormdataRequestBody{
			Username: username,
			Password: "WRONG_password_123",
		})
		if err != nil {
			t.Fatalf("%s unexpected error: %v", api, err)
		}
		if resp.StatusCode() != http.StatusUnauthorized {
			t.Fatalf("%s expected status %d, got=%d body=%s", api, http.StatusUnauthorized, resp.StatusCode(), string(resp.Body))
		}
		if resp.JSON401 == nil {
			t.Fatalf("%s expected JSON401 body, got nil; raw=%s", api, string(resp.Body))
		}
		if resp.JSON401.Code != InvalidCredentials {
			t.Fatalf("%s expected error.code=%q, got=%q (message=%q)", api, InvalidCredentials, resp.JSON401.Code, resp.JSON401.Message)
		}
	})
}

func assertErrorCodeIn(t *testing.T, raw []byte, want ...ErrorResponseCode) {
	t.Helper()

	if len(raw) == 0 {
		t.Fatalf("expected non-empty error body")
	}

	var er ErrorResponse
	if err := json.Unmarshal(raw, &er); err != nil {
		t.Fatalf("expected json error body, unmarshal error=%v raw=%s", err, string(raw))
	}

	if !slices.Contains(want, er.Code) {
		t.Fatalf("unexpected error.code=%q; want one of=%v; message=%q; raw=%s", er.Code, want, er.Message, string(raw))
	}
}

func do(t *testing.T, req *http.Request) (int, []byte) {
	t.Helper()

	httpClient := newTracingHTTPClient(t, http.DefaultTransport)

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed read body: %s", err.Error())
	}
	return resp.StatusCode, b
}

func doRawLoginRequest(t *testing.T, baseURL string, body io.Reader, contentType string) (status int, respBody []byte) {
	t.Helper()

	u := strings.TrimRight(baseURL, "/") + "/api/v1/auth/login"
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, u, body)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	return do(t, req)
}
