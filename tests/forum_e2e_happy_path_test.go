//go:build e2e

package tests

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"slices"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	. "stepik.leoscode.http/internal/gen/api"
)

var emptyUUID uuid.UUID

// baseURL для API берём из окружения, чтобы можно было легко
// запускать против разных окружений.
func getBaseURL() string {
	if v := os.Getenv("FORUM_BASE_URL"); v != "" {
		return v
	}
	// по умолчанию — локальный сервер
	return "http://localhost:8080"
}

func Test_Forum_e2e_HappyPath(t *testing.T) {
	// 1. Health-check
	t.Run("[GET]/internal/v1/health", func(t *testing.T) {
		const api = "[health_check]"

		t.Run("without_header", func(t *testing.T) {
			c := newTestClient(t)

			resp, err := c.HealthCheckWithResponse(t.Context(), &HealthCheckParams{})
			if err != nil {
				t.Fatalf("%s unexpected error: %v", api, err)
			}

			if resp.StatusCode() != http.StatusOK {
				t.Fatalf("%s unexpected status: %d, body=%s", api, resp.StatusCode(), string(resp.Body))
			}

			if resp.JSON200 == nil || resp.JSON200.Status != "ok" {
				t.Fatalf("%s unexpected body: %+v", api, resp.JSON200)
			}

			t.Logf("%s: status=%s", api, resp.JSON200.Status)

			if _, ok := resp.HTTPResponse.Header["X-Request-Id"]; ok {
				t.Fatalf("%s expected no X-Request-Id header key", api)
			}
		})

		t.Run("with_header", func(t *testing.T) {
			c := newTestClient(t)

			reqID := "e2e-health-" + randomSuffix()

			resp, err := c.HealthCheckWithResponse(t.Context(), &HealthCheckParams{
				XRequestId: ptr(RequestIdHeader(reqID)),
			})
			if err != nil {
				t.Fatalf("%s unexpected error: %v", api, err)
			}

			if resp.StatusCode() != http.StatusOK {
				t.Fatalf("%s unexpected status: %d, body=%s", api, resp.StatusCode(), string(resp.Body))
			}

			if resp.JSON200 == nil || resp.JSON200.Status != "ok" {
				t.Fatalf("%s unexpected body: %+v", api, resp.JSON200)
			}

			t.Logf("%s: status=%s", api, resp.JSON200.Status)

			got := resp.HTTPResponse.Header.Get("X-Request-Id")
			if got != reqID {
				t.Fatalf("%s expected X-Request-Id=%q, got=%q", api, reqID, got)
			}

			t.Logf("%s: X-Request-Id=%s", api, got)
		})
	})

	// 1.1 Truncate
	t.Run("[POST]/internal/v1/truncate", func(t *testing.T) {
		c := newTestClient(t)

		truncateDB(t, c)
	})

	// 2. Login (авторегистрация) двух пользователей
	var (
		usernameA = "userA_" + randomSuffix()
		usernameB = "userB_" + randomSuffix()

		userA, userB uuid.UUID
	)

	t.Run("[POST]/api/v1/auth/login", func(t *testing.T) {
		const api = "[login]"

		const pswd = "password123"
		users := []struct {
			Username string
			Password string
			Ref      *uuid.UUID
		}{
			{
				Username: usernameA,
				Password: pswd,
				Ref:      &userA,
			},
			{
				Username: usernameB,
				Password: pswd,
				Ref:      &userB,
			},
		}

		t.Run("create_users", func(t *testing.T) {
			for _, user := range users {
				t.Run(user.Username, func(t *testing.T) {
					c := newTestClient(t)

					*user.Ref = login(t, c, user.Username, user.Password)
				})
			}
		})

		t.Run("login_users", func(t *testing.T) {
			for _, user := range users {
				t.Run(user.Username, func(t *testing.T) {
					c := newTestClient(t)

					if id := login(t, c, user.Username, user.Password); id != *user.Ref {
						t.Fatalf("%s Login '%s': expected user_id=%s, got=%s", api, user.Username, *user.Ref, id)
					}
				})
			}
		})
	})

	if userA == emptyUUID || userB == emptyUUID {
		t.Fatalf("users not initialized")
	}

	// 3. Создаём несколько тредов от разных пользователей
	var (
		threadA1 *Thread
		threadA2 *Thread
		threadB1 *Thread
		threadB2 *Thread
	)

	t.Run("[POST]/api/v1/threads", func(t *testing.T) {
		const api = "[threads]"

		// Важно: Ref — **Thread, чтобы мы могли сохранить указатель наружу из табличного теста.
		threads := []struct {
			Name    string
			UserID  uuid.UUID
			ReqID   string // префикс X-Request-Id
			Title   string
			Content string
			Tags    *[]string
			Ref     **Thread // куда сохранить результат (resp.JSON201)
		}{
			{
				Name:    "userA_thread1",
				UserID:  userA,
				ReqID:   "e2e-create-threadA1",
				Title:   "How to start with Go? " + randomSuffix(),
				Content: "I want to learn Go. Any tips?",
				Tags:    &[]string{"go", "novice"},
				Ref:     &threadA1,
			},
			{
				Name:    "userA_thread2",
				UserID:  userA,
				ReqID:   "e2e-create-threadA2",
				Title:   "Advanced Go tips " + randomSuffix(),
				Content: "Let's discuss performance tuning in Go.",
				Tags:    ptr([]string{"go", "advanced"}),
				Ref:     &threadA2,
			},
			{
				Name:    "userB_thread1",
				UserID:  userB,
				ReqID:   "e2e-create-threadB1",
				Title:   "Python vs Go " + randomSuffix(),
				Content: "Which language should I choose?",
				Tags:    ptr([]string{"python"}),
				Ref:     &threadB1,
			},
			{
				Name:    "userB_thread2",
				UserID:  userB,
				ReqID:   "e2e-create-threadB2",
				Title:   "Rust vs Go " + randomSuffix(),
				Content: "Is Rust faster than Go?",
				Tags:    nil, // tags опциональны
				Ref:     &threadB2,
			},
		}

		for _, tt := range threads {
			t.Run(tt.Name, func(t *testing.T) {
				c := newTestClient(t)

				reqID := tt.ReqID + "-" + randomSuffix()
				params := &CreateThreadParams{
					XUserId:         UserIdHeader(tt.UserID),
					XIdempotencyKey: IdempotencyKeyHeader(reqID),
				}
				body := ThreadCreate{
					Title:   tt.Title,
					Content: tt.Content,
					Tags:    tt.Tags,
				}

				created := createThread(t, c, params, body)

				// Ассерты на содержимое Thread
				assertThread(t, created, tt.UserID, tt.Title, tt.Content, tt.Tags, false)

				// Сохраняем ссылку наружу
				*tt.Ref = created

				t.Logf("%s Created %s: id=%d title=%s", api, tt.Name, created.Id, created.Title)
			})
		}
	})

	if threadA1 == nil || threadA2 == nil || threadB1 == nil || threadB2 == nil {
		t.Fatalf("threads not initialized")
	}

	// 4. Получение треда по ID
	t.Run("[GET]/api/v1/threads/{thread_id}", func(t *testing.T) {
		const api = "[threads_get]"

		tests := []struct {
			Name     string
			CallerID uuid.UUID // кто делает запрос (может быть автор или другой)
			Thread   *Thread   // ссылка на переменную threadA1/threadA2/... (двойной указатель)
		}{
			{Name: "get_threadA1_by_userA", CallerID: userA, Thread: threadA1},
			{Name: "get_threadA2_by_userA", CallerID: userA, Thread: threadA2},
			{Name: "get_threadB1_by_userB", CallerID: userB, Thread: threadB1},
			{Name: "get_threadB2_by_userB", CallerID: userB, Thread: threadB2},
			{Name: "get_threadB1_by_userA", CallerID: userA, Thread: threadB1},
		}

		for _, tt := range tests {
			t.Run(tt.Name, func(t *testing.T) {
				c := newTestClient(t)

				want := tt.Thread

				got := getThread(t, c, tt.CallerID, want.Id)

				// Ассерты на содержимое Thread (те же ожидания, что при создании)
				assertThread(
					t,
					got,
					want.AuthorId, // ожидаемый автор (UUID из Thread)
					want.Title,    // ожидаемый title
					want.Content,  // ожидаемый content
					want.Tags,     // ожидаемые tags (*[]string или nil)
					want.IsLocked, // ожидаемый locked
				)

				t.Logf("%s Get thread %s: id=%d title=%s", api, tt.Name, got.Id, got.Title)
			})
		}
	})

	// 5. Список тредов + фильтры + пагинация (только happy-path)
	t.Run("[GET]/api/v1/threads", func(t *testing.T) {
		const api = "[list_threads]"

		expectedAll := map[int64]struct{}{
			threadA1.Id: {},
			threadA2.Id: {},
			threadB1.Id: {},
			threadB2.Id: {},
		}

		cases := []struct {
			Name      string
			Params    *ListThreadsParams
			WantTotal int64
			WantIDs   map[int64]struct{} // nil => не проверяем конкретный набор id
		}{
			{
				Name: "all_default_limit_100",
				Params: &ListThreadsParams{
					Limit:  ptr(int32(100)),
					Offset: ptr(int32(0)),
				},
				WantTotal: 4,
				WantIDs:   expectedAll,
			},
			{
				Name: "filter_author_userA",
				Params: &ListThreadsParams{
					AuthorId: ptr(uuid.UUID(userA)),
					Limit:    ptr(int32(100)),
					Offset:   ptr(int32(0)),
				},
				WantTotal: 2,
				WantIDs: map[int64]struct{}{
					threadA1.Id: {},
					threadA2.Id: {},
				},
			},
			{
				Name: "filter_author_userB",
				Params: &ListThreadsParams{
					AuthorId: ptr(uuid.UUID(userB)),
					Limit:    ptr(int32(100)),
					Offset:   ptr(int32(0)),
				},
				WantTotal: 2,
				WantIDs: map[int64]struct{}{
					threadB1.Id: {},
					threadB2.Id: {},
				},
			},
			{
				Name: "filter_tag_go",
				Params: &ListThreadsParams{
					Tag:    ptr("go"),
					Limit:  ptr(int32(100)),
					Offset: ptr(int32(0)),
				},
				WantTotal: 2,
				WantIDs: map[int64]struct{}{
					threadA1.Id: {},
					threadA2.Id: {},
				},
			},
			{
				Name: "filter_tag_python",
				Params: &ListThreadsParams{
					Tag:    ptr("python"),
					Limit:  ptr(int32(100)),
					Offset: ptr(int32(0)),
				},
				WantTotal: 1,
				WantIDs: map[int64]struct{}{
					threadB1.Id: {},
				},
			},
		}

		for _, tc := range cases {
			t.Run(tc.Name, func(t *testing.T) {
				c := newTestClient(t)

				list := listThreads(t, c, tc.Params)

				// Meta
				if list.Meta.Total != tc.WantTotal {
					t.Fatalf("%s total: expected=%d got=%d", api, tc.WantTotal, list.Meta.Total)
				}

				items := list.Items // nil => пустой
				if int64(len(items)) > tc.WantTotal {
					t.Fatalf("%s items len=%d > total=%d", api, len(items), tc.WantTotal)
				}

				// Проверяем, что id из ответа входят в ожидаемый набор (если задан)
				if tc.WantIDs != nil {
					gotIDs := map[int64]struct{}{}
					for i := range items {
						gotIDs[items[i].Id] = struct{}{}
					}
					if len(gotIDs) != len(tc.WantIDs) {
						t.Fatalf("%s ids len mismatch: expected=%d got=%d (ids=%v)", api, len(tc.WantIDs), len(gotIDs), gotIDs)
					}
					for id := range tc.WantIDs {
						if _, ok := gotIDs[id]; !ok {
							t.Fatalf("%s expected id=%d in response, got ids=%v", api, id, gotIDs)
						}
					}
				}
			})
		}

		// Пагинация (порядок не фиксируем, но проверяем стабильность и отсутствие пересечения)
		t.Run("pagination_full_scan", func(t *testing.T) {
			c := newTestClient(t)

			const limit = int32(1)
			var totalThreads = int64(len(expectedAll))

			// Узнаём total
			first := listThreads(t, c, &ListThreadsParams{
				Limit:  ptr(limit),
				Offset: ptr(int32(0)),
			})
			total := first.Meta.Total
			if total != totalThreads {
				t.Fatalf("%s unexpected total: got=%d want=%d meta=%+v", api, total, totalThreads, first.Meta)
			}

			seen := make(map[int64]struct{}, total)
			for offset := int32(0); offset < int32(total); offset++ {
				t.Run(fmt.Sprintf("limit1_offset%d", offset), func(t *testing.T) {
					list := listThreads(t, c, &ListThreadsParams{
						Limit:  ptr(limit),
						Offset: ptr(offset),
					})

					assertListThreadsMeta(t, list.Meta, total, limit, offset)
					if len(list.Items) != 1 {
						t.Fatalf("%s expected 1 item on offset=%d, got %d", api, offset, len(list.Items))
					}

					id := list.Items[0].Id

					if _, ok := expectedAll[id]; !ok {
						t.Fatalf("%s offset=%d id=%d not in expected set", api, offset, id)
					}
					if _, dup := seen[id]; dup {
						t.Fatalf("%s duplicate id=%d on offset=%d", api, id, offset)
					}
					seen[id] = struct{}{}
				})
			}

			if int64(len(seen)) != total {
				t.Fatalf("%s expected %d unique ids, got %d", api, total, len(seen))
			}

			t.Run("offset_out_of_range", func(t *testing.T) {
				// Мы должны отдать просто пустой список с успешным кодом и метой
				offset := int32(total)

				list := listThreads(t, c, &ListThreadsParams{
					Limit:  ptr(limit),
					Offset: ptr(offset),
				})

				assertListThreadsMeta(t, list.Meta, total, limit, offset)

				if len(list.Items) != 0 {
					t.Fatalf("%s expected empty items for offset=%d, got %d", api, offset, len(list.Items))
				}
			})
		})
	})

	// 6. PATCH треда
	t.Run("[PATCH]/api/v1/threads/{thread_id}", func(t *testing.T) {
		const api = "[threads.patch]"

		type tc struct {
			Name      string
			UserID    uuid.UUID
			ThreadRef **Thread // какую переменную обновляем (threadA1 и т.д.)
			BuildBody func(t *testing.T) ThreadPatch
			// ожидания на изменённые поля
			WantTitle    string
			WantContent  string
			WantTags     *[]string
			WantIsLocked bool
		}

		// локальные конструкторы
		makeTitlePatch := func(t *testing.T, s string) ThreadPatch {
			t.Helper()

			var p ThreadPatch
			if err := p.FromThreadPatch0(ThreadPatch0{Title: s}); err != nil {
				t.Fatalf("%s makeTitlePatch: %s", api, err)
			}
			return p
		}
		makeContentPatch := func(t *testing.T, s string) ThreadPatch {
			t.Helper()

			var p ThreadPatch
			if err := p.FromThreadPatch1(ThreadPatch1{Content: s}); err != nil {
				t.Fatalf("%s makeContentPatch: %s", api, err)
			}
			return p
		}
		makeTagsPatch := func(t *testing.T, tags []string) ThreadPatch {
			t.Helper()

			var p ThreadPatch
			if err := p.FromThreadPatch2(ThreadPatch2{Tags: tags}); err != nil {
				t.Fatalf("%s makeTagsPatch: %s", api, err)
			}
			return p
		}
		makeIsLockedPatch := func(t *testing.T, isLocked bool) ThreadPatch {
			t.Helper()

			var p ThreadPatch
			if err := p.FromThreadPatch3(ThreadPatch3{IsLocked: isLocked}); err != nil {
				t.Fatalf("%s makeTagsPatch: %s", api, err)
			}
			return p
		}

		makeContentAndTagsPatch := func(t *testing.T, s string, tags []string) ThreadPatch {
			t.Helper()

			var p ThreadPatch
			if err := p.FromThreadPatch1(ThreadPatch1{Content: s}); err != nil {
				t.Fatalf("%s makeContentPatch: %s", api, err)
			}

			if err := p.MergeThreadPatch2(ThreadPatch2{Tags: tags}); err != nil {
				t.Fatalf("%s makeTagsPatch: %s", api, err)
			}
			return p
		}

		var (
			// берём текущие значения из уже созданных тредов
			origA1 = *threadA1
			origB1 = *threadB1
			origB2 = *threadB2

			newTitleA1   = "UPDATED TITLE " + randomSuffix()
			newContentA1 = "UPDATED CONTENT " + randomSuffix()
			newContentB1 = "UPDATED CONTENT " + randomSuffix()
			newTagsB1    = []string{"go", "python"}
			newTagsB2    = []string{"go", "e2e", "patched"}
		)

		cases := []tc{
			{
				Name:      "patch_title_threadA1",
				UserID:    userA,
				ThreadRef: &threadA1,
				BuildBody: func(t *testing.T) ThreadPatch { return makeTitlePatch(t, newTitleA1) },
				// обновили
				WantTitle: newTitleA1,
				// остальные поля должны остаться как были
				WantContent:  origA1.Content,
				WantTags:     origA1.Tags,
				WantIsLocked: origA1.IsLocked,
			},
			{
				Name:      "patch_content_threadA1",
				UserID:    userA,
				ThreadRef: &threadA1,
				BuildBody: func(t *testing.T) ThreadPatch { return makeContentPatch(t, newContentA1) },
				// уже обновили ранее
				WantTitle: newTitleA1,
				// обновили
				WantContent: newContentA1,
				// остальные поля должны остаться как были
				WantTags:     origA1.Tags,
				WantIsLocked: origA1.IsLocked,
			},
			{
				Name:      "patch_tags_threadB2",
				UserID:    userB,
				ThreadRef: &threadB2,
				BuildBody: func(t *testing.T) ThreadPatch { return makeTagsPatch(t, newTagsB2) },
				// обновили
				WantTags: ptr(newTagsB2),
				// остальные поля должны остаться как были
				WantTitle:    origB2.Title,
				WantContent:  origB2.Content,
				WantIsLocked: origB2.IsLocked,
			},
			{
				Name:      "patch_content_and_tags_threadB1",
				UserID:    userB,
				ThreadRef: &threadB1,
				BuildBody: func(t *testing.T) ThreadPatch { return makeContentAndTagsPatch(t, newContentB1, newTagsB1) },
				// обновили
				WantContent: newContentB1,
				WantTags:    ptr(newTagsB1),
				// остальные поля должны остаться как были
				WantTitle:    origB1.Title,
				WantIsLocked: origB1.IsLocked,
			},
			{
				Name:      "patch_is_locked_threadB1",
				UserID:    userB,
				ThreadRef: &threadB1,
				BuildBody: func(t *testing.T) ThreadPatch { return makeIsLockedPatch(t, true) },
				// уже обновили ранее
				WantContent: newContentB1,
				WantTags:    ptr(newTagsB1),
				// обновили
				WantIsLocked: true,
				// остальные поля должны остаться как были
				WantTitle: origB1.Title,
			},
		}

		for _, tt := range cases {
			t.Run(tt.Name, func(t *testing.T) {
				c := newTestClient(t)

				before := *(*tt.ThreadRef)

				updated := patchThread(t, c, before.Id, tt.UserID, tt.BuildBody(t))

				assertThread(t, updated, tt.UserID, tt.WantTitle, tt.WantContent, tt.WantTags, tt.WantIsLocked)

				// обновляем ссылку на тред (чтобы следующие тесты работали с актуальными данными)
				*tt.ThreadRef = updated
			})
		}
	})

	// 7. PUT (полная замена треда)
	t.Run("[PUT]/api/v1/threads/{thread_id}", func(t *testing.T) {
		const api = "[replace_thread]"

		type tc struct {
			Name       string
			UserID     uuid.UUID
			ThreadRef  **Thread // какую переменную обновляем (threadA1 и т.д.)
			NewTitle   string
			NewContent string
			NewTags    *[]string
		}

		var (
			newTagsA2 = &[]string{"go", "tips", "replaced"}
			// пример replace в nil tags (полная замена с tags=nil)
			newTagsNil *[]string = nil
		)

		cases := []tc{
			{
				Name:       "replace_threadA2_by_author_userA",
				UserID:     userA,
				ThreadRef:  &threadA2,
				NewTitle:   "REPLACED A2 TITLE " + randomSuffix(),
				NewContent: "REPLACED A2 CONTENT " + randomSuffix(),
				NewTags:    newTagsA2,
			},
			{
				Name:       "replace_threadB2_by_author_userB",
				UserID:     userB,
				ThreadRef:  &threadB2,
				NewTitle:   "REPLACED B2 TITLE " + randomSuffix(),
				NewContent: "REPLACED B2 CONTENT " + randomSuffix(),
				NewTags:    newTagsNil,
			},
		}

		for _, tt := range cases {
			t.Run(tt.Name, func(t *testing.T) {
				c := newTestClient(t)

				before := *(*tt.ThreadRef)

				updated := replaceThread(t, c, before.Id, tt.UserID, ThreadCreate{
					Title:   tt.NewTitle,
					Content: tt.NewContent,
					Tags:    tt.NewTags,
				})

				// 1) ID должен остаться тем же
				if updated.Id != before.Id {
					t.Fatalf("%s id changed: before=%d after=%d", api, before.Id, updated.Id)
				}

				// 2) created_at обычно НЕ должен меняться при replace
				if !updated.CreatedAt.Equal(before.CreatedAt) {
					t.Fatalf("%s created_at changed: before=%s after=%s", api, before.CreatedAt, updated.CreatedAt)
				}

				// 3) Ассертим полностью содержимое (title/content/tags + автор + locked)
				assertThread(t, updated, tt.UserID, tt.NewTitle, tt.NewContent, tt.NewTags, false)

				// 4) Обновляем ссылку на тред, чтобы дальше тесты работали с новым состоянием
				*tt.ThreadRef = updated
			})
		}
	})

	// 8. Посты в треде: создание (happy path)
	var (
		postA1_1 *Post
		postA1_2 *Post
		postA2_1 *Post
		postB2_1 *Post
	)

	t.Run("[POST]/api/v1/threads/{thread_id}/posts", func(t *testing.T) {
		const api = "[posts.create]"

		cases := []struct {
			Name      string
			UserID    uuid.UUID
			ThreadRef **Thread // берем id из уже созданного threadA1 / threadB2 / ...
			Content   string
			Ref       **Post // куда сохранить созданный пост
		}{
			{
				Name:      "threadA1_post1_userA",
				UserID:    userA,
				ThreadRef: &threadA1,
				Content:   "First post from userA " + randomSuffix(),
				Ref:       &postA1_1,
			},
			{
				Name:      "threadA1_post2_userB",
				UserID:    userB,
				ThreadRef: &threadA1,
				Content:   "Reply from userB " + randomSuffix(),
				Ref:       &postA1_2,
			},
			{
				Name:      "threadA2_post1_userB",
				UserID:    userB,
				ThreadRef: &threadA2,
				Content:   "Cross-user post " + randomSuffix(),
				Ref:       &postA2_1,
			},
			{
				Name:      "threadB2_post1_userB",
				UserID:    userB,
				ThreadRef: &threadB2,
				Content:   "Hello in my own thread " + randomSuffix(),
				Ref:       &postB2_1,
			},
		}

		for _, tt := range cases {
			t.Run(tt.Name, func(t *testing.T) {
				c := newTestClient(t)

				thread := *tt.ThreadRef
				if thread == nil {
					t.Fatalf("%s %s: threadRef is nil", api, tt.Name)
				}

				created := createPost(t, c, tt.UserID, thread.Id, tt.Content)
				assertPost(t, created, tt.UserID, thread.Id, tt.Content)

				*tt.Ref = created
				t.Logf("%s created %s: post_id=%d thread_id=%d", api, tt.Name, created.Id, created.ThreadId)
			})
		}
	})

	if postA1_1 == nil || postA1_2 == nil || postA2_1 == nil || postB2_1 == nil {
		t.Fatalf("posts not initialized")
	}

	// 9. LIST постов в треде: пагинация (happy path)
	t.Run("[GET]/api/v1/threads/{thread_id}/posts", func(t *testing.T) {
		const api = "[posts.list]"

		type tc struct {
			Name     string
			Thread   *Thread
			Expected []*Post // какие посты должны быть в этом треде (по id)
		}

		cases := []tc{
			{
				Name:     "threadA1_posts",
				Thread:   threadA1,
				Expected: []*Post{postA1_1, postA1_2},
			},
			{
				Name:     "threadA2_posts",
				Thread:   threadA2,
				Expected: []*Post{postA2_1},
			},
			{
				Name:     "threadB2_posts",
				Thread:   threadB2,
				Expected: []*Post{postB2_1},
			},
		}

		for _, tt := range cases {
			t.Run(tt.Name, func(t *testing.T) {
				// Ожидаемый набор id
				expectedIDs := make(map[int64]struct{}, len(tt.Expected))
				for _, p := range tt.Expected {
					if p == nil {
						t.Fatalf("%s %s: expected post is nil", api, tt.Name)
					}
					expectedIDs[p.Id] = struct{}{}
				}
				wantTotal := len(expectedIDs)

				// Итерация по всем страницам limit=1
				seen := make(map[int64]struct{}, wantTotal)
				for offset := range wantTotal {
					c := newTestClient(t)
					list := listPosts(t, c, tt.Thread.Id, &ListPostsParams{
						Limit:  ptr(int32(1)),
						Offset: ptr(int32(offset)),
					})

					if int(list.Meta.Total) != wantTotal {
						t.Fatalf("%s %s meta.total: expected=%d got=%d", api, tt.Name, wantTotal, list.Meta.Total)
					}

					if len(list.Items) != 1 {
						t.Fatalf("%s %s: expected 1 item at offset=%d, got=%d", api, tt.Name, offset, len(list.Items))
					}

					id := list.Items[0].Id

					if _, ok := expectedIDs[id]; !ok {
						t.Fatalf("%s %s: item id=%d at offset=%d not in expected set", api, tt.Name, id, offset)
					}
					if _, dup := seen[id]; dup {
						t.Fatalf("%s %s: duplicate id=%d encountered at offset=%d", api, tt.Name, id, offset)
					}
					seen[id] = struct{}{}
				}

				if len(seen) != wantTotal {
					t.Fatalf("%s %s: expected to see %d unique ids, got=%d", api, tt.Name, wantTotal, len(seen))
				}

				// Выход за пределы: offset == total => пусто, но meta.total сохраняется
				t.Run("offset_out_of_range", func(t *testing.T) {
					c := newTestClient(t)

					list := listPosts(t, c, tt.Thread.Id, &ListPostsParams{
						Limit:  ptr(int32(1)),
						Offset: ptr(int32(wantTotal)), // за пределами
					})

					if int(list.Meta.Total) != wantTotal {
						t.Fatalf("%s %s meta.total(out_of_range): expected=%d got=%d", api, tt.Name, wantTotal, list.Meta.Total)
					}
					if len(list.Items) != 0 {
						t.Fatalf("%s %s: expected empty list for out_of_range, got=%d", api, tt.Name, len(list.Items))
					}
				})
			})
		}
	})

	// 10. Вложения: upload -> get metadata -> download

	// 10.1 Upload attachment (multipart/form-data)
	var (
		uploadedAttachment *Attachment
		uploadedBytes      []byte
		uploadedFilename   = "smoke.png"
		uploadedMime       = "image/png"
	)

	t.Run("[POST]/api/v1/threads/{thread_id}/attachments", func(t *testing.T) {
		const api = "[attachments.upload]"

		c := newTestClient(t)

		// 1x1 PNG (валидный минимальный PNG), чтобы тест был самодостаточным
		uploadedBytes = []byte{
			0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
			0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
			0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
			0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4, 0x89,
			0x00, 0x00, 0x00, 0x0A, 0x49, 0x44, 0x41, 0x54,
			0x78, 0x9C, 0x63, 0x00, 0x01, 0x00, 0x00, 0x05,
			0x00, 0x01, 0x0D, 0x0A, 0x2D, 0xB4,
			0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44,
			0xAE, 0x42, 0x60, 0x82,
		}

		caption := "smoke caption " + randomSuffix()
		ct, body := buildMultipartUpload(t, uploadedFilename, uploadedMime, uploadedBytes, &caption)

		resp, err := c.UploadAttachmentWithBodyWithResponse(
			t.Context(),
			ThreadIdPath(threadA1.Id),
			&UploadAttachmentParams{XUserId: UserIdHeader(userA)},
			ct,
			body,
		)
		if err != nil {
			t.Fatalf("%s unexpected error: %v", api, err)
		}
		if resp.StatusCode() != http.StatusCreated {
			t.Fatalf("%s unexpected status: %d body=%s", api, resp.StatusCode(), string(resp.Body))
		}
		if resp.JSON201 == nil {
			t.Fatalf("%s JSON201 is nil", api)
		}

		uploadedAttachment = resp.JSON201

		// Минимальные ассерты на метаданные
		if uploadedAttachment.Id.String() == "" || uploadedAttachment.Id.String() == "00000000-0000-0000-0000-000000000000" {
			t.Fatalf("%s id is empty: %+v", api, uploadedAttachment)
		}
		if uploadedAttachment.ThreadId != threadA1.Id {
			t.Fatalf("%s thread_id: expected=%d got=%d", api, threadA1.Id, uploadedAttachment.ThreadId)
		}
		if uploadedAttachment.Filename != uploadedFilename {
			t.Fatalf("%s filename: expected=%q got=%q", api, uploadedFilename, uploadedAttachment.Filename)
		}
		if string(uploadedAttachment.MimeType) != uploadedMime {
			t.Fatalf("%s mime_type: expected=%q got=%q", api, uploadedMime, uploadedAttachment.MimeType)
		}
		if uploadedAttachment.Size != int64(len(uploadedBytes)) {
			t.Fatalf("%s size: expected=%d got=%d", api, len(uploadedBytes), uploadedAttachment.Size)
		}
		if uploadedAttachment.CreatedAt.IsZero() {
			t.Fatalf("%s created_at is zero", api)
		}
	})

	if uploadedAttachment == nil {
		t.Fatalf("attachment not initialized")
	}

	// 10.2 Get attachment metadata
	t.Run("[GET]/api/v1/attachments/{attachment_id}", func(t *testing.T) {
		const api = "[attachments.get]"

		c := newTestClient(t)

		resp, err := c.GetAttachmentWithResponse(t.Context(), uploadedAttachment.Id)
		if err != nil {
			t.Fatalf("%s unexpected error: %v", api, err)
		}
		if resp.StatusCode() != http.StatusOK {
			t.Fatalf("%s unexpected status: %d body=%s", api, resp.StatusCode(), string(resp.Body))
		}
		if resp.JSON200 == nil {
			t.Fatalf("%s JSON200 is nil", api)
		}

		got := resp.JSON200
		if got.Id != uploadedAttachment.Id {
			t.Fatalf("%s id: expected=%s got=%s", api, uploadedAttachment.Id, got.Id)
		}
		if got.ThreadId != uploadedAttachment.ThreadId {
			t.Fatalf("%s thread_id: expected=%d got=%d", api, uploadedAttachment.ThreadId, got.ThreadId)
		}
		if got.Filename != uploadedAttachment.Filename {
			t.Fatalf("%s filename: expected=%q got=%q", api, uploadedAttachment.Filename, got.Filename)
		}
		if got.MimeType != uploadedAttachment.MimeType {
			t.Fatalf("%s mime_type: expected=%q got=%q", api, uploadedAttachment.MimeType, got.MimeType)
		}
		if got.Size != uploadedAttachment.Size {
			t.Fatalf("%s size: expected=%d got=%d", api, uploadedAttachment.Size, got.Size)
		}
		if got.CreatedAt.IsZero() {
			t.Fatalf("%s created_at is zero", api)
		}
	})

	// 10.3 Download attachment file
	t.Run("[GET]/api/v1/attachments/{attachment_id}/file", func(t *testing.T) {
		const api = "[attachments.download]"

		c := newTestClient(t)

		resp, err := c.DownloadAttachmentFileWithResponse(t.Context(), uploadedAttachment.Id)
		if err != nil {
			t.Fatalf("%s unexpected error: %v", api, err)
		}
		if resp.StatusCode() != http.StatusOK {
			t.Fatalf("%s unexpected status: %d body_len=%d", api, resp.StatusCode(), len(resp.Body))
		}

		// Минимальные проверки на содержимое файла.
		if len(resp.Body) == 0 {
			t.Fatalf("%s expected non-empty body", api)
		}
		// PNG signature
		pngSig := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
		if len(resp.Body) < len(pngSig) || string(resp.Body[:len(pngSig)]) != string(pngSig) {
			t.Errorf("%s expected PNG signature, got first bytes=%v", api, resp.Body[:min(len(resp.Body), 8)])
		}
		// В идеале байты совпадают 1-в-1; если сервис что-то перекодирует — это может отличаться.
		if len(resp.Body) == len(uploadedBytes) && string(resp.Body) != string(uploadedBytes) {
			t.Errorf("%s size matches but bytes differ", api)
		}
	})

	// 11. Поиск
	t.Run("[GET]/api/v1/search", func(t *testing.T) {
		t.Skip("Можете пропустить тесты на полнотекстовый поиск если ваша реализация не умеет это.")
	})

	// 12. Удаление треда
	t.Run("[DELETE]/api/v1/threads/{thread_id}", func(t *testing.T) {
		const api = "[threads.delete]"

		c := newTestClient(t)

		// --- act: удаляем ранее созданный тред ---
		deleteResp, err := c.DeleteThreadWithResponse(
			t.Context(),
			ThreadIdPath(threadA1.Id),
			&DeleteThreadParams{
				XUserId: UserIdHeader(userA),
			},
		)
		if err != nil {
			t.Fatalf("%s delete thread error: %v", api, err)
		}
		if deleteResp.StatusCode() != http.StatusNoContent {
			t.Fatalf("%s unexpected delete status: %d", api, deleteResp.StatusCode())
		}

		// --- assert: тред больше недоступен ---
		getResp, err := c.GetThreadWithResponse(
			t.Context(),
			threadA1.Id,
			&GetThreadParams{
				XUserId: ptr(OptionalUserIdHeader(userA)),
			},
		)
		if err != nil {
			t.Fatalf("%s get deleted thread error: %v", api, err)
		}
		if getResp.StatusCode() != http.StatusNotFound {
			t.Fatalf("%s expected 404 after delete, got %d", api, getResp.StatusCode())
		}
	})
}

// --- вспомогательные тесты ---

// newTestClient создаёт клиента с разумным таймаутом.
func newTestClient(t *testing.T) ClientWithResponsesInterface {
	t.Helper()

	baseURL := getBaseURL()
	t.Logf("[setup] Using base URL: %s", baseURL)

	httpClient := newTracingHTTPClient(t, http.DefaultTransport)

	client, err := NewClientWithResponses(baseURL, WithHTTPClient(httpClient))
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	return client
}

func truncateDB(t *testing.T, c ClientWithResponsesInterface) {
	t.Helper()

	const api = "[internal_truncate]"

	resp, err := c.InternalTruncateWithResponse(t.Context())
	if err != nil {
		t.Fatalf("%s unexpected error: %v", api, err)
	}

	if resp.StatusCode() != http.StatusNoContent {
		t.Fatalf("%s unexpected status=%d", api, resp.StatusCode())
	}

	t.Logf("%s successed", api)
}

func login(
	t *testing.T,
	c ClientWithResponsesInterface,
	username,
	password string,
) uuid.UUID {
	t.Helper()

	const api = "[login]"

	resp, err := c.LoginWithFormdataBodyWithResponse(t.Context(), LoginFormdataRequestBody{
		Username: username,
		Password: password,
	})
	if err != nil {
		t.Fatalf("%s Login '%s' error: %v", api, username, err)
	}

	if resp.StatusCode() != http.StatusOK {
		t.Fatalf("%s Login '%s' status=%d body=%s",
			api, username, resp.StatusCode(), string(resp.Body))
	}
	if resp.JSON200 == nil || resp.JSON200.UserId == emptyUUID {
		t.Fatalf("%s Login '%s': empty JSON200 or user_id", api, username)
	}

	t.Logf("%s Login '%s' OK: user_id=%s", api, username, resp.JSON200.UserId)
	return resp.JSON200.UserId
}

func assertThread(
	t *testing.T,
	got *Thread,
	wantAuthor uuid.UUID,
	wantTitle,
	wantContent string,
	wantTags *[]string, // nil => ожидаем, что tags отсутствует (или пустой слайс)
	wantLocked bool,
) {
	t.Helper()

	const api = "[thread]"

	if got == nil {
		t.Fatalf("%s thread is nil", api)
	}

	// Id
	if got.Id <= 0 {
		t.Fatalf("%s thread.id: expected > 0, got=%d", api, got.Id)
	}

	// AuthorId
	if got.AuthorId != wantAuthor {
		t.Fatalf("%s thread.author_id: expected=%s got=%s", api, wantAuthor, got.AuthorId)
	}

	// Title
	if got.Title != wantTitle {
		t.Fatalf("%s thread.title: expected=%q got=%q", api, wantTitle, got.Title)
	}

	// Content
	if got.Content != wantContent {
		t.Fatalf("%s thread.content: expected=%q got=%q", api, wantContent, got.Content)
	}

	// Tags (order-insensitive)
	if wantTags == nil {
		// допускаем nil или пустой массив в ответе
		if got.Tags != nil && len(*got.Tags) != 0 {
			t.Fatalf("%s thread.tags: expected nil/empty, got=%v", api, *got.Tags)
		}
	} else {
		if got.Tags == nil {
			t.Fatalf("%s thread.tags: expected %v, got=nil", api, *wantTags)
		}

		// copy
		gotTags := append([]string(nil), (*got.Tags)...)
		want := append([]string(nil), (*wantTags)...)

		sort.Strings(gotTags)
		sort.Strings(want)

		if !slices.Equal(gotTags, want) {
			t.Fatalf("%s thread.tags: expected=%v got=%v", api, want, gotTags)
		}
	}

	// Locked
	if got.IsLocked != wantLocked {
		t.Fatalf("%s thread.is_locked: expected=%v got=%v", api, wantLocked, got.IsLocked)
	}

	// Timestamps
	if got.CreatedAt.IsZero() {
		t.Fatalf("%s thread.created_at: expected non-zero", api)
	}

	if got.UpdatedAt != nil {
		if got.UpdatedAt.Before(got.CreatedAt) {
			t.Fatalf("%s thread.updated_at < created_at: created=%s updated=%s", api, got.CreatedAt, got.UpdatedAt)
		}
	}
}

func getThread(
	t *testing.T,
	c ClientWithResponsesInterface,
	userID uuid.UUID,
	threadID int64,
) *Thread {
	t.Helper()

	const api = "[get_thread]"

	resp, err := c.GetThreadWithResponse(t.Context(), ThreadIdPath(threadID), &GetThreadParams{
		XUserId: ptr(OptionalUserIdHeader(userID)),
	})
	if err != nil {
		t.Fatalf("%s unexpected error: %v", api, err)
	}

	if resp.StatusCode() != http.StatusOK {
		t.Fatalf("%s unexpected status: %d body=%s", api, resp.StatusCode(), string(resp.Body))
	}

	if resp.JSON200 == nil {
		t.Fatalf("%s JSON200 is nil", api)
	}

	return resp.JSON200
}

func listThreads(
	t *testing.T,
	c ClientWithResponsesInterface,
	params *ListThreadsParams,
) *ThreadListResponse {
	t.Helper()

	const api = "[list_threads]"

	resp, err := c.ListThreadsWithResponse(t.Context(), params)
	if err != nil {
		t.Fatalf("%s unexpected error: %v", api, err)
	}
	if resp.StatusCode() != http.StatusOK {
		t.Fatalf("%s unexpected status: %d body=%s", api, resp.StatusCode(), string(resp.Body))
	}
	if resp.JSON200 == nil {
		t.Fatalf("%s JSON200 is nil", api)
	}
	return resp.JSON200
}

func assertListThreadsMeta(
	t *testing.T,
	got PaginationMeta,
	wantTotal int64,
	wantLimit,
	wantOffset int32,
) {
	t.Helper()

	const api = "[list_threads][pagination]"

	if got.Total != wantTotal {
		t.Fatalf("%s meta.total: expected=%d got=%d (meta=%+v)", api, wantTotal, got.Total, got)
	}

	if got.Limit != wantLimit {
		t.Fatalf("%s meta.limit: expected=%d got=%d (meta=%+v)", api, wantLimit, got.Limit, got)
	}

	if got.Offset != wantOffset {
		t.Fatalf("%s meta.offset: expected=%d got=%d (meta=%+v)", api, wantOffset, got.Offset, got)
	}
}

func patchThread(
	t *testing.T,
	c ClientWithResponsesInterface,
	threadID int64,
	userID uuid.UUID,
	body ThreadPatch,
) *Thread {
	t.Helper()

	const api = "[patch_thread]"

	resp, err := c.PatchThreadWithResponse(t.Context(), threadID, &PatchThreadParams{
		XUserId: UserIdHeader(userID),
	}, body)
	if err != nil {
		t.Fatalf("%s unexpected error: %v", api, err)
	}
	if resp.StatusCode() != http.StatusOK {
		t.Fatalf("%s unexpected status: %d body=%s", api, resp.StatusCode(), string(resp.Body))
	}
	if resp.JSON200 == nil {
		t.Fatalf("%s JSON200 is nil", api)
	}
	return resp.JSON200
}

func replaceThread(
	t *testing.T,
	c ClientWithResponsesInterface,
	threadID int64,
	userID uuid.UUID,
	body ThreadCreate,
) *Thread {
	t.Helper()

	const api = "[replace_thread]"

	resp, err := c.ReplaceThreadWithResponse(
		t.Context(),
		ThreadIdPath(threadID),
		&ReplaceThreadParams{
			XUserId: UserIdHeader(userID),
		},
		body,
	)
	if err != nil {
		t.Fatalf("%s unexpected error: %v", api, err)
	}

	if resp.StatusCode() != http.StatusOK {
		t.Fatalf("%s unexpected status=%d body=%s", api, resp.StatusCode(), string(resp.Body))
	}

	if resp.JSON200 == nil {
		t.Fatalf("%s JSON200 is nil", api)
	}

	return resp.JSON200
}

func createPost(
	t *testing.T,
	c ClientWithResponsesInterface,
	userID uuid.UUID,
	threadID int64,
	content string,
) *Post {
	t.Helper()

	const api = "[create_post]"

	resp, err := c.CreatePostWithResponse(t.Context(), ThreadIdPath(threadID),
		&CreatePostParams{XUserId: UserIdHeader(userID), XIdempotencyKey: randomSuffix()},
		PostCreate{Content: content},
	)
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

func assertPost(
	t *testing.T,
	got *Post,
	wantUserID uuid.UUID,
	wantThreadID int64,
	wantContent string,
) {
	t.Helper()
	const api = "[assert_post]"

	if got == nil {
		t.Fatalf("%s post is nil", api)
	}

	if got.Id <= 0 {
		t.Fatalf("%s post.id: expected >0, got=%d", api, got.Id)
	}
	if got.ThreadId != wantThreadID {
		t.Fatalf("%s post.thread_id: expected=%d got=%d", api, wantThreadID, got.ThreadId)
	}

	if got.AuthorId != wantUserID {
		t.Fatalf("%s post.author_id: expected=%s got=%s", api, wantUserID, got.AuthorId)
	}

	if got.Content != wantContent {
		t.Fatalf("%s post.content: expected=%q got=%q", api, wantContent, got.Content)
	}

	if got.CreatedAt.IsZero() {
		t.Fatalf("%s post.created_at: expected non-zero", api)
	}

	// updated_at у Post nullable (*time.Time) — тут просто допускаем nil.
	if got.UpdatedAt != nil && got.UpdatedAt.IsZero() {
		t.Fatalf("%s post.updated_at: non-nil but zero", api)
	}
}

func listPosts(
	t *testing.T,
	c ClientWithResponsesInterface,
	threadID int64,
	params *ListPostsParams,
) *PostListResponse {
	t.Helper()

	const api = "[list_posts]"

	resp, err := c.ListPostsWithResponse(t.Context(), ThreadIdPath(threadID), params)
	if err != nil {
		t.Fatalf("%s unexpected error: %v", api, err)
	}
	if resp.StatusCode() != http.StatusOK {
		t.Fatalf("%s unexpected status: %d body=%s", api, resp.StatusCode(), string(resp.Body))
	}
	if resp.JSON200 == nil {
		t.Fatalf("%s JSON200 is nil", api)
	}
	return resp.JSON200
}

// --- маленькие утилиты ---

// генерим суффикс для уникальных username / title
func randomSuffix() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// traceTransport логирует HTTP-запросы/ответы и печатает их только если тест упал.
// Это сильно ускоряет отладку студентам.
type traceTransport struct {
	t    *testing.T
	next http.RoundTripper

	mu  sync.Mutex
	buf []traceEntry
	cap int
}

type traceEntry struct {
	Method string
	URL    string

	ReqHeaders http.Header
	ReqBody    string

	Status      int
	RespHeaders http.Header
	RespBody    string

	Dur time.Duration
}

func newTracingHTTPClient(t *testing.T, base http.RoundTripper) *http.Client {
	if base == nil {
		base = http.DefaultTransport
	}
	tr := &traceTransport{t: t, next: base, cap: 64}
	// Печатаем трейсы только если тест упал.
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		tr.dump()
	})

	return &http.Client{
		Transport: tr,
		Timeout:   10 * time.Second,
	}
}

func (tr *traceTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()

	// Снимаем тело запроса (и восстанавливаем его назад)
	var reqBody []byte
	if req.Body != nil {
		reqBody, _ = io.ReadAll(req.Body)
		req.Body.Close()
		req.Body = io.NopCloser(bytes.NewReader(reqBody))
	}

	resp, err := tr.next.RoundTrip(req)
	dur := time.Since(start)
	if err != nil {
		tr.append(traceEntry{
			Method:      req.Method,
			URL:         req.URL.String(),
			ReqHeaders:  req.Header.Clone(),
			ReqBody:     truncate(string(reqBody), 4096),
			Status:      0,
			RespHeaders: nil,
			RespBody:    err.Error(),
			Dur:         dur,
		})
		return nil, err
	}

	// Снимаем тело ответа (и восстанавливаем его назад)
	var respBody []byte
	if resp.Body != nil {
		respBody, _ = io.ReadAll(resp.Body)
		resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(respBody))
	}

	tr.append(traceEntry{
		Method:      req.Method,
		URL:         req.URL.String(),
		ReqHeaders:  req.Header.Clone(),
		ReqBody:     truncate(string(reqBody), 4096),
		Status:      resp.StatusCode,
		RespHeaders: resp.Header.Clone(),
		RespBody:    truncate(string(respBody), 4096),
		Dur:         dur,
	})

	return resp, nil
}

func (tr *traceTransport) append(e traceEntry) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	if tr.cap <= 0 {
		tr.cap = 64
	}
	if len(tr.buf) >= tr.cap {
		copy(tr.buf, tr.buf[1:])
		tr.buf[len(tr.buf)-1] = e
		return
	}
	tr.buf = append(tr.buf, e)
}

func (tr *traceTransport) dump() {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	if len(tr.buf) == 0 {
		tr.t.Logf("[trace] no http calls captured")
		return
	}
	tr.t.Logf("===== HTTP TRACE (last %d) =====", len(tr.buf))
	for i, e := range tr.buf {
		tr.t.Logf("[%02d] %s %s (%s)", i, e.Method, e.URL, e.Dur)
		tr.t.Logf("     req_headers=%v", e.ReqHeaders)
		if e.ReqBody != "" {
			tr.t.Logf("     req_body=%s", e.ReqBody)
		}
		tr.t.Logf("     status=%d resp_headers=%v", e.Status, e.RespHeaders)
		if e.RespBody != "" {
			tr.t.Logf("     resp_body=%s", e.RespBody)
		}
	}
	tr.t.Logf("===== END TRACE =====")
}

func truncate(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n] + "…(truncated)"
}

func ptr[T any](v T) *T { return &v }

// func deref[T any](v *T) T {
// 	var def T
// 	if v == nil {
// 		return def
// 	}
// 	return *v
// }

func buildMultipartUpload(t *testing.T, filename, mimeType string, b []byte, caption *string) (contentType string, body *bytes.Buffer) {
	t.Helper()

	body = &bytes.Buffer{}
	w := multipart.NewWriter(body)

	// caption — обычное текстовое поле
	if caption != nil {
		_ = w.WriteField("caption", *caption)
	}

	// file — бинарное поле + MIME
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, "file", filename))
	h.Set("Content-Type", mimeType)

	part, err := w.CreatePart(h)
	if err != nil {
		t.Fatalf("multipart create part: %v", err)
	}
	if _, err := part.Write(b); err != nil {
		t.Fatalf("multipart write file: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("multipart close: %v", err)
	}

	return w.FormDataContentType(), body
}

func createThread(
	t *testing.T,
	c ClientWithResponsesInterface,
	params *CreateThreadParams,
	body CreateThreadJSONRequestBody,
) *Thread {
	const api = "[thread_create]"

	resp, err := c.CreateThreadWithResponse(t.Context(), params, body)
	if err != nil {
		t.Fatalf("%s CreateThread %s error: %v", api, t.Name(), err)
	}
	if resp.StatusCode() != http.StatusCreated {
		t.Fatalf("%s CreateThread %s: status=%d body=%s", api, t.Name(), resp.StatusCode(), string(resp.Body))
	}
	if resp.JSON201 == nil {
		t.Fatalf("%s CreateThread %s: JSON201 is nil", api, t.Name())
	}
	return resp.JSON201
}
