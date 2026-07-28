// internal/service/idempotency.go
package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// TypedIdempotencyRecord типизированная запись идемпотентности
type TypedIdempotencyRecord[T any] struct {
	Result      T
	CreatedAt   time.Time
	UserID      string
	RequestBody string
	TTL         time.Duration
}

// TypedIdempotencyStore типизированное хранилище
type TypedIdempotencyStore[T any] struct {
	records map[string]TypedIdempotencyRecord[T]
	mu      sync.RWMutex
	ttl     time.Duration
}

// NewTypedIdempotencyStore создает новое хранилище
func NewTypedIdempotencyStore[T any](ttl time.Duration) *TypedIdempotencyStore[T] {
	store := &TypedIdempotencyStore[T]{
		records: make(map[string]TypedIdempotencyRecord[T]),
		ttl:     ttl,
	}

	// Запускаем фоновую очистку
	go store.cleanupWorker()

	return store
}

// Set сохраняет результат
func (s *TypedIdempotencyStore[T]) Set(key string, userID string, result T, requestBody string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.records[key] = TypedIdempotencyRecord[T]{
		Result:      result,
		CreatedAt:   time.Now(),
		UserID:      userID,
		RequestBody: requestBody,
		TTL:         s.ttl,
	}
}

// Get получает результат
func (s *TypedIdempotencyStore[T]) Get(key string) (T, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var zero T
	record, exists := s.records[key]
	if !exists {
		return zero, false
	}

	// Проверяем TTL
	if time.Since(record.CreatedAt) > record.TTL {
		return zero, false
	}

	return record.Result, true
}

// GetFullRecord получает полную запись (для проверки конфликтов)
func (s *TypedIdempotencyStore[T]) GetFullRecord(key string) (TypedIdempotencyRecord[T], bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	record, exists := s.records[key]
	if !exists {
		return TypedIdempotencyRecord[T]{}, false
	}

	// Проверяем TTL
	if time.Since(record.CreatedAt) > record.TTL {
		return TypedIdempotencyRecord[T]{}, false
	}

	return record, true
}

// Delete удаляет ключ
func (s *TypedIdempotencyStore[T]) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.records, key)
}

// cleanupWorker периодически чистит устаревшие записи
func (s *TypedIdempotencyStore[T]) cleanupWorker() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		s.cleanup()
	}
}

// cleanup удаляет все устаревшие записи
func (s *TypedIdempotencyStore[T]) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for key, record := range s.records {
		if now.Sub(record.CreatedAt) > record.TTL {
			delete(s.records, key)
		}
	}
}

// hashRequestBody создает хеш для проверки конфликтов
func HashRequestBody(data interface{}) string {
	hash := sha256.Sum256([]byte(fmt.Sprintf("%v", data)))
	return hex.EncodeToString(hash[:])
}
