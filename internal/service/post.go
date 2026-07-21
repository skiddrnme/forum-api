package service

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"
	forum "stepik.leoscode.http/internal/gen/api"
	"stepik.leoscode.http/internal/utils"
)

type PostService struct {
	posts            map[int64]forum.Post
	nextID           int64
	mu               sync.Mutex
	idempotencyStore *TypedIdempotencyStore[forum.Post]
}

func NewPostService() *PostService {
	return &PostService{
		posts:            make(map[int64]forum.Post),
		nextID:           1,
		idempotencyStore: NewTypedIdempotencyStore[forum.Post](24 * time.Hour),
	}
}

// Получение постов
func (p *PostService) GetPosts(threadID string, limit int, offset int) ([]forum.Post, error) {

	// Парсим thread_id
	threadIDInt, err := strconv.ParseInt(threadID, 10, 64)
	if err != nil {
		return []forum.Post{}, fmt.Errorf("invalid thread_id: %w", err)
	}

	// Фильтрация по thread_id
	var result []forum.Post
	for _, post := range p.posts {
		if post.ThreadId == threadIDInt {
			result = append(result, post)
		}
	}

	// Сортировка по дате (новые сверху)
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})

	// Пагинация
	paginated := utils.ApplyPaginateWithDefault(result, limit, offset)
	return paginated, nil
}

// GetPostsWithMeta получает посты с метаданными пагинации
func (p *PostService) GetPostsWithMeta(threadID string, limit int, offset int) (forum.PostListResponse, error) {
	// Получаем посты
	posts, err := p.GetPosts(threadID, limit, offset)
	if err != nil {
		return forum.PostListResponse{}, err
	}

	// Получаем общее количество
	total := p.countPosts(threadID)

	// Нормализуем параметры
	normalizedLimit, normalizedOffset := p.normalizePaginationParams(limit, offset)

	return forum.PostListResponse{
		Items: posts,
		Meta: forum.PaginationMeta{
			Limit:  int32(normalizedLimit),
			Offset: int32(normalizedOffset),
			Total:  int64(total),
		},
	}, nil
}

// countPosts считает общее количество постов в треде
func (p *PostService) countPosts(threadID string) int {

	threadIDInt, err := strconv.ParseInt(threadID, 10, 64)
	if err != nil {
		return 0
	}

	count := 0
	for _, post := range p.posts {
		if post.ThreadId == threadIDInt {
			count++
		}
	}
	return count
}

// normalizePaginationParams нормализует параметры пагинации
func (p *PostService) normalizePaginationParams(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func (p *PostService) CreatePost(threadID string, userID openapi_types.UUID, req forum.PostCreate, idempotencyKey string) (forum.Post, bool, bool, error){
	p.mu.Lock()
	defer p.mu.Unlock()
	
	// Если ключа нет - создаем как обычно
	if idempotencyKey == ""{
		post := p.createNewPost(threadID, userID, req)
		return post, false, false, nil
	}
	// Проверяем идемпотентность 
	if record, exists := p.idempotencyStore.GetFullRecord(idempotencyKey); exists{
		// Проверяем, что это тот же пользователь
		if record.UserID != userID.String(){
			return forum.Post{}, false, false, errors.New("user mismatch")
		}

		// Проверяем, совпадает ли тело запроса
		currentHash := HashRequestBody(req)
		if record.RequestBody != currentHash{
			// Тело отличается - конфликт!
			return forum.Post{}, false, true, errors.New("conflict: different request body")
		}

		// Все совпадает - возвращаем кэшированный результат
		return record.Result, true, false, nil
	}

	post := p.createNewPost(threadID, userID, req)

	hash := HashRequestBody(req)
	p.idempotencyStore.Set(idempotencyKey, userID.String(), post, hash)

	return post, false, false, nil
}

func (p *PostService) createNewPost(thread_id string, user_id openapi_types.UUID, req forum.PostCreate) forum.Post{
	thread_id_INT, err := strconv.ParseInt(thread_id, 10, 64)
	if err != nil{
		return forum.Post{}
	}
	post := forum.Post{
		Id: p.nextID,
		AuthorId: user_id,
		Content: req.Content,
		CreatedAt: time.Now(),
		ThreadId: thread_id_INT,
	}
	p.posts[post.Id] = post
	p.nextID++
	return post
}
