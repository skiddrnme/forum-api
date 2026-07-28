package service

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"
	forum "stepik.leoscode.http/internal/gen/api"
	"stepik.leoscode.http/internal/utils"
)

// Сервис для работы с тредами
type ThreadService struct {
	threads          map[int64]forum.Thread
	nextID           int64
	mu               sync.RWMutex
	idempotencyStore *TypedIdempotencyStore[forum.Thread] // Типизированное хранилище для Thread
}

func NewThreadService() *ThreadService {
	return &ThreadService{
		threads:          make(map[int64]forum.Thread),
		nextID:           1,
		idempotencyStore: NewTypedIdempotencyStore[forum.Thread](24 * time.Hour),
	}
}

func (t *ThreadService) GetThreads(limit int, offset int, tag string, authorID string) ([]forum.Thread, error) {
	var result []forum.Thread
	for _, thread := range t.threads {
		if !t.matchesFilters(thread, tag, authorID) {
			continue
		}
		result = append(result, thread)
	}

	// Сортировка по дате (новые сверху)
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})

	// Пагинация
	paginated := utils.ApplyPaginateWithDefault(result, limit, offset)

	return paginated, nil
}

func (t *ThreadService) GetThreadsWithMeta(limit int, offset int, tag string, authorID string) (forum.ThreadListResponse, error) {
	threads, err := t.GetThreads(limit, offset, tag, authorID)
	if err != nil {
		return forum.ThreadListResponse{}, err
	}

	// Получаем общее количество (без пагинации)
	total := t.countThreads(tag, authorID)

	return forum.ThreadListResponse{
		Items: threads,
		Meta: forum.PaginationMeta{
			Limit:  int32(limit),
			Offset: int32(offset),
			Total:  int64(total),
		},
	}, nil
}

func (t *ThreadService) countThreads(tag string, authorID string) int {
	count := 0
	for _, thread := range t.threads {
		if t.matchesFilters(thread, tag, authorID) {
			count++
		}
	}
	return count
}

// matchesFilters проверяет соответствие фильтрам
func (t *ThreadService) matchesFilters(thread forum.Thread, tag string, authorID string) bool {
	// Проверка автора
	if authorID != "" && thread.AuthorId.String() != authorID {
		return false
	}

	// Проверка тега
	if tag != "" && !hasTag(thread, tag) {
		return false
	}

	return true
}

func hasTag(thread forum.Thread, tag string) bool {
	if tag == "" {
		return true
	}
	if thread.Tags != nil {
		if slices.Contains(*thread.Tags, tag) {
			return true
		}
	}
	return false
}

// CreateThread создает новый тред с поддержкой идемпотентности
func (t *ThreadService) CreateThread(
	userID openapi_types.UUID,
	req forum.ThreadCreate,
	idempotencyKey string,
) (forum.Thread, bool, bool, error) {

	t.mu.Lock()
	defer t.mu.Unlock()

	// Если ключа нет - создаем как обычно
	if idempotencyKey == "" {
		thread := t.createNewThread(userID, req)
		return thread, false, false, nil
	}

	// Проверяем идемпотентность
	if record, exists := t.idempotencyStore.GetFullRecord(idempotencyKey); exists {
		// Проверяем, что это тот же пользователь
		if record.UserID != userID.String() {
			return forum.Thread{}, false, false, errors.New("user mismatch")
		}

		// Проверяем, совпадает ли тело запроса
		currentHash := HashRequestBody(req)
		if record.RequestBody != currentHash {
			// Тело отличается - конфликт!
			return forum.Thread{}, false, true, errors.New("conflict: different request body")
		}

		// Все совпадает - возвращаем кэшированный результат
		return record.Result, true, false, nil
	}

	// Новый запрос - создаем тред
	thread := t.createNewThread(userID, req)

	// Сохраняем в идемпотентное хранилище
	hash := HashRequestBody(req)
	t.idempotencyStore.Set(idempotencyKey, userID.String(), thread, hash)

	return thread, false, false, nil
}

func (t *ThreadService) createNewThread(userID openapi_types.UUID, req forum.ThreadCreate) forum.Thread {
	thread := forum.Thread{
		Id:        t.nextID,
		AuthorId:  userID,
		Title:     req.Title,
		Content:   req.Content,
		Tags:      req.Tags,
		CreatedAt: time.Now(),
		IsLocked:  false,
	}

	t.threads[thread.Id] = thread
	t.nextID++
	return thread
}

func (t *ThreadService) FindThreadByID(thread_id string) (forum.Thread, error) {
	threadIdInt, _ := strconv.ParseInt(thread_id, 10, 64)
	if thread, ok := t.threads[threadIdInt]; ok {
		return thread, nil
	}
	return forum.Thread{}, errors.New("thread not found")
}

func (t *ThreadService) UpdateAllThread(userID openapi_types.UUID, thread_id string, req forum.ThreadCreate) (forum.Thread, error) {
	threadIDInt, err := strconv.ParseInt(thread_id, 10, 64)

	if err != nil {
		return forum.Thread{}, fmt.Errorf("invalid thread_id format: %w", err)
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	entry, exists := t.threads[threadIDInt]
	if !exists {
		return forum.Thread{}, errors.New("thread not found")
	}
	if entry.AuthorId != userID {
		return forum.Thread{}, errors.New("user mismatch: cannot modify another user's thread")
	}

	// 4. Проверяем блокировку
	if entry.IsLocked {
		return forum.Thread{}, errors.New("thread is locked and cannot be modified")
	}

	if len(req.Title) == 0 {
		return forum.Thread{}, errors.New("validation_error: title cannot be empty")
	}
	if len(req.Title) > 255 {
		return forum.Thread{}, fmt.Errorf("validation_error: title must be between 1 and 255 characters, got %d", len(req.Title))
	}
	if strings.TrimSpace(req.Title) == "" {
		return forum.Thread{}, errors.New("validation_error: title cannot be only spaces")
	}

	if len(req.Content) == 0 {
		return forum.Thread{}, errors.New("validation_error: content cannot be empty")
	}
	if len(req.Content) > 10000 {
		return forum.Thread{}, fmt.Errorf("validation_error: content must be between 1 and 10000 characters, got %d", len(req.Content))
	}
	if strings.TrimSpace(req.Content) == "" {
		return forum.Thread{}, errors.New("validation_error: content cannot be only spaces")
	}

	// Tags (если есть)
	if req.Tags != nil {
		if len(*req.Tags) > 10 {
			return forum.Thread{}, fmt.Errorf("validation_error: too many tags, maximum 10 allowed, got %d", len(*req.Tags))
		}
		for i, tag := range *req.Tags {
			if len(tag) == 0 {
				return forum.Thread{}, fmt.Errorf("validation_error: tag at position %d cannot be empty", i)
			}
			if len(tag) > 32 {
				return forum.Thread{}, fmt.Errorf("validation_error: tag at position %d must be between 1 and 32 characters, got %d", i, len(tag))
			}
			if strings.TrimSpace(tag) == "" {
				return forum.Thread{}, fmt.Errorf("validation_error: tag at position %d cannot be only spaces", i)
			}
		}
	}

	// 6. Обновляем все поля
	now := time.Now()
	entry.Title = req.Title
	entry.Content = req.Content
	entry.Tags = req.Tags
	entry.UpdatedAt = &now

	t.threads[threadIDInt] = entry

	return entry, nil
}

func (t *ThreadService) UpdateThreadPatch(user_id openapi_types.UUID, thread_id string, req forum.ThreadPatch) (forum.Thread, error) {
	threadIDInt, err := strconv.ParseInt(thread_id, 10, 64)
	if err != nil {
		return forum.Thread{}, fmt.Errorf("неверный thread_id", err)
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	// Проверяем существование треда
	entry, exists := t.threads[threadIDInt]
	if !exists {
		return forum.Thread{}, errors.New("thread not found")
	}

	// Проверяем права доступа
	if entry.AuthorId != user_id {
		return forum.Thread{}, errors.New("user mismatch: cannot modify another user's thread")
	}

	// Проверяем, не заблокирован ли тред
	if entry.IsLocked {
		return forum.Thread{}, errors.New("thread is locked and cannot be modified")
	}

	// Проверяем, что в запросе есть хотя бы одно поле для обновления
	hasChanges := false

	// Пробуем распарсить каждый тип патча
	if patch0, err := req.AsThreadPatch0(); err == nil {
		// ВАЖНО: валидируем ДАЖЕ если title пустой (потому что тест может проверить это)
		// Но если title - это пустая строка, мы должны вернуть ошибку
		if len(patch0.Title) == 0 {
			return forum.Thread{}, errors.New("validation_error: title cannot be empty")
		}
		if len(patch0.Title) > 255 {
			return forum.Thread{}, fmt.Errorf("validation_error: title must be between 1 and 255 characters, got %d", len(patch0.Title))
		}
		if strings.TrimSpace(patch0.Title) == "" {
			return forum.Thread{}, errors.New("validation_error: title cannot be only spaces")
		}

		entry.Title = patch0.Title
		hasChanges = true
	}

	if patch1, err := req.AsThreadPatch1(); err == nil {
		if patch1.Content != "" {
			entry.Content = patch1.Content
			hasChanges = true
		}
	}

	if patch2, err := req.AsThreadPatch2(); err == nil {
		// ВАЖНО: tags может быть пустым массивом (очистка тегов)
		if len(patch2.Tags) >= 0 { // разрешаем пустой массив
			entry.Tags = &patch2.Tags
			hasChanges = true
		}
	}

	if patch3, err := req.AsThreadPatch3(); err == nil {
		// ВАЖНО: может быть false, поэтому проверяем что поле установлено
		entry.IsLocked = patch3.IsLocked
		hasChanges = true
	}

	// Если ничего не изменилось - возвращаем ошибку
	if !hasChanges {
		return forum.Thread{}, errors.New("no valid fields to update")
	}

	now := time.Now()
	entry.UpdatedAt = &now

	t.threads[threadIDInt] = entry

	return entry, nil

}

func (t *ThreadService) DeleteThread(user_id openapi_types.UUID, thread_id string) error {
	threadIDInt, err := strconv.ParseInt(thread_id, 10, 64)
	if err != nil {
		return fmt.Errorf("неверный thread_id: %w", err)
	}

	thread, ok := t.threads[threadIDInt]
	if !ok {
		return fmt.Errorf("thread with id %d not found", threadIDInt)
	}

	if thread.AuthorId != user_id {
		return errors.New("forbidden: only the author can delete the thread")
	}

	if thread.IsLocked {
		return errors.New("thread_locked: cannot delete a locked thread")
	}

	delete(t.threads, threadIDInt)

	return nil
}
