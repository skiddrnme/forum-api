package service

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"
	forum "stepik.leoscode.http/internal/gen/api"
)

type ThreadService struct {
	threads map[string]forum.Thread
	nextID int64
	idempotencyKeys map[string]IdempotencyRecord
}

type IdempotencyRecord struct{
	Thread forum.Thread
	CreatedAt time.Time
	RequestBody string // Хеш тела запроса для проверки конфликта
	UserID string
} 


func NewThreadService() *ThreadService{
	return &ThreadService{
		threads: make(map[string]forum.Thread),
		nextID: 1,
		idempotencyKeys: make(map[string]IdempotencyRecord),
	}
}

func (t *ThreadService) GetThreads()(map[string]forum.Thread, error){
	return t.threads, nil
}


// Create возвращает (thread, isCached, conflict, error)
func (t *ThreadService) Create(userID openapi_types.UUID, req forum.ThreadCreate, idempotencyKey string)(forum.Thread, bool, bool, error){

	if idempotencyKey == ""{
		// Если ключа нет - создаем как обычно (хотя по спецификации он обязателен)
		return t.createNewThread(userID, req), false, false, nil
	}

	if record, exists := t.idempotencyKeys[idempotencyKey]; exists{
		// Проверяем, что это тот же пользователь
		if record.UserID != userID.String(){
			return forum.Thread{}, false, false, errors.New("user mismatch")
		}

		// Проверяем, совпадает ли тело запроса
		currentHash := hashRequestBody(req)
		if record.RequestBody != currentHash{
			// Тело отличается - конфликт!
			return forum.Thread{}, false, true, errors.New("conflict: different request body")
		}

		// Все совпадает - возвращаем кэшированный результат
		return record.Thread, true, false, nil
	}
	// Новый запрос - создаем тред
	thread := t.createNewThread(userID, req)

	// Сохраняем в кэш
	t.idempotencyKeys[idempotencyKey] = IdempotencyRecord{
		Thread: thread,
		CreatedAt: time.Now(),
		RequestBody: hashRequestBody(req),
		UserID: userID.String(),
	}
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
	
	t.threads[fmt.Sprintf("%d", thread.Id)] = thread
	t.nextID++
	return thread
}

// создает хеш тела запроса для проверки конфликтов
func hashRequestBody(req forum.ThreadCreate) string{
	data := fmt.Sprintf("%s|%s|%v", req.Title, req.Content, req.Tags)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}