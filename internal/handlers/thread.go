package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	forum "stepik.leoscode.http/internal/gen/api"
	"stepik.leoscode.http/internal/service"
)

type ThreadHandler struct {
	threadService *service.ThreadService
}

func NewThreadHandler(threadService *service.ThreadService) *ThreadHandler {
	return &ThreadHandler{
		threadService: threadService,
	}
}

func (t *ThreadHandler) GetThreads(c *gin.Context) {
	limitStr := c.DefaultQuery("limit", "20")
	offsetStr := c.DefaultQuery("offset", "0")

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 || limit > 100 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "validation_error",
			"message": "limit must be between 1 and 100",
		})
		return
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "validation_error",
			"message": "offset must be >= 0",
		})
		return
	}

	tag := c.Query("tag")
	authorID := c.Query("author_id")

	if authorID != "" {
		if _, err := uuid.Parse(authorID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    "validation_error",
				"message": "invalid author_id format",
			})
			return
		}
	}

	threads, err := t.threadService.GetThreadsWithMeta(limit, offset, tag, authorID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "internal_server",
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, threads)
}

func (t *ThreadHandler) GetThreadByID(c *gin.Context){
	// Получаем thread_id из path параметров
	id := c.Param("thread_id")

	thread, err := t.threadService.GetThreadByID(id)
	if err != nil{
		c.JSON(http.StatusNotFound, gin.H{
			"code": "not_found",
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Тред найден",
		"item": thread,
	})
}


func (t *ThreadHandler) Create(c *gin.Context) {
	// 1. Получаем userID
	userID := c.GetHeader("X-User-Id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    "unauthorized",
			"message": "User ID не найден",
		})
		return
	}
	parseUUID, err := uuid.Parse(userID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "bad_request",
			"message": "Неверный формат User ID",
		})
		return
	}

	// 2. Получаем Idempotency Key (обязателен по спецификации)
	idempotencyKey := c.GetHeader("X-Idempotency-Key")
	if idempotencyKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "validation_error",
			"message": "X-Idempotency-Key обязателен",
		})
		return
	}

	// 3. Парсим тело запроса
	var req forum.ThreadCreate
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "bad_request",
			"message": err.Error(),
		})
		return
	}

	// 4. Валидация обязательных полей
	if req.Title == "" || req.Content == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "validation_error",
			"message": "Title и Content обязательны",
		})
		return
	}

	// 5. Создаем тред с идемпотентностью

	thread, wasCached, conflict, err := t.threadService.Create(parseUUID, req, idempotencyKey)
	if err != nil {
		if conflict {
			// 409 Conflict - тело запроса отличается
			c.JSON(http.StatusConflict, gin.H{
				"code":    "bad_request",
				"message": "X-Idempotency-Key уже использован с другим телом запроса",
			})
			return
		}

		// 500 Internal Error
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "internal_error",
			"message": err.Error(),
		})
		return
	}

	// 6. Возвращаем ответ (200 или 201)

	statusCode := http.StatusCreated
	if wasCached {
		statusCode = http.StatusOK // 200 для повторных запросов
	}

	c.JSON(statusCode, thread)
}
