package service

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"sync"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"
	forum "stepik.leoscode.http/internal/gen/api"
	"stepik.leoscode.http/internal/utils"
)

// Сервис для работы с тредами
type ThreadService struct {
	threads         map[int64]forum.Thread
	nextID          int64
	mu             sync.RWMutex
	idempotencyStore *TypedIdempotencyStore[forum.Thread] // Типизированное хранилище для Thread
}

func NewThreadService() *ThreadService {
	return &ThreadService{
		threads:         make(map[int64]forum.Thread),
		nextID:          1,
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

func (t *ThreadService) UpdateAllThread(user_id openapi_types.UUID, thread_id string, req forum.ThreadCreate) (forum.Thread, error) {
	threadIdInt, _ := strconv.ParseInt(thread_id, 10, 64)

	if entry, ok := t.threads[threadIdInt]; ok {
		if entry.AuthorId != user_id {
			return forum.Thread{}, errors.New("Несоответствие пользователей")
		}
		now := time.Now()
		entry.Title = req.Title
		entry.Content = req.Content
		entry.Tags = req.Tags
		entry.UpdatedAt = &now

		t.threads[threadIdInt] = entry

		return entry, nil
	}
	return forum.Thread{}, errors.New("thread not found")
}

func (t *ThreadService) UpdateThreadPatch(user_id openapi_types.UUID, thread_id string, req forum.ThreadPatch) (forum.Thread, error) {
	threadIDInt, err := strconv.ParseInt(thread_id, 10, 64)
	if err != nil {
		return forum.Thread{}, fmt.Errorf("неверный thread_id", err)
	}

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
		if patch0.Title != "" {
			entry.Title = patch0.Title
			hasChanges = true
		}
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
	if !ok{
		return fmt.Errorf("thread with id %d not found", threadIDInt)
	}

	if thread.AuthorId != user_id{
		return errors.New("forbidden: only the author can delete the thread")
	}

	if thread.IsLocked {
		return errors.New("thread_locked: cannot delete a locked thread")
	}

	delete(t.threads, threadIDInt)

	return nil
}
