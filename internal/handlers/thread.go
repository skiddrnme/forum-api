package handlers

import (
	"net/http"
	"strconv"
	"strings"

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

func (t *ThreadHandler) GetThreadByID(c *gin.Context) {
	// Получаем thread_id из path параметров
	id := c.Param("thread_id")

	thread, err := t.threadService.FindThreadByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    "not_found",
			"message": err.Error(),
		})
		return
	}

	// Дописать еще одну ошибку (error internal server)

	c.JSON(http.StatusOK, thread)
}

func (t *ThreadHandler) UpdateAll(c *gin.Context) {

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

	threadID := c.Param("thread_id")

	var req forum.ThreadCreate

	// Парсим тело запроса
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "bad_request",
			"message": err.Error(),
		})
		return
	}

	// 4. Вызываем сервис
	thread, err := t.threadService.UpdateAllThread(parseUUID, threadID, req)
	if err != nil {
		// ✅ ВАЖНО: обрабатываем разные типы ошибок!
		errMsg := err.Error()

		// Проверяем ошибки валидации → 400 Bad Request
		if strings.Contains(errMsg, "validation_error") ||
			strings.Contains(errMsg, "title must be between") ||
			strings.Contains(errMsg, "content must be between") ||
			strings.Contains(errMsg, "too many tags") ||
			strings.Contains(errMsg, "tag at position") ||
			strings.Contains(errMsg, "cannot be empty") ||
			strings.Contains(errMsg, "cannot be only spaces") {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    "validation_error",
				"message": errMsg,
			})
			return
		}

		// Проверяем ошибку "not found" → 404
		if strings.Contains(errMsg, "thread not found") {
			c.JSON(http.StatusNotFound, gin.H{
				"code":    "not_found",
				"message": errMsg,
			})
			return
		}

		// Проверяем ошибку "user mismatch" → 403
		if strings.Contains(errMsg, "user mismatch") || strings.Contains(errMsg, "Несоответствие пользователей") {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    "forbidden",
				"message": errMsg,
			})
			return
		}

		// Проверяем ошибку "locked" → 403
		if strings.Contains(errMsg, "locked") || strings.Contains(errMsg, "заблокирован") {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    "thread_locked",
				"message": errMsg,
			})
			return
		}

		// Неизвестная ошибка → 500
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "internal_error",
			"message": "internal server error",
		})
		return
	}

	// 5. Успешный ответ
	c.JSON(http.StatusOK, gin.H{
		"message": "Тред заменен",
		"thread":  thread,
	})

}

func (t *ThreadHandler) UpdatePatch(c *gin.Context) {
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
	threadID := c.Param("thread_id")
	if threadID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "validation error",
			"message": "thread_id is required",
		})
		return
	}

	var req forum.ThreadPatch
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "bad_request",
			"message": "invalid JSON body",
		})
		return
	}

	thread, err := t.threadService.UpdateThreadPatch(parseUUID, threadID, req)
	if err != nil {
		switch {
		case strings.Contains(err.Error(), "thread not found"):
			c.JSON(http.StatusNotFound, gin.H{
				"code":    "not_found",
				"message": err.Error(),
			})
		case strings.Contains(err.Error(), "forbidden") || strings.Contains(err.Error(), "user mismatch"):
			c.JSON(http.StatusForbidden, gin.H{
				"code":    "forbidden",
				"message": err.Error(),
			})
		case strings.Contains(err.Error(), "locked"):
			c.JSON(http.StatusForbidden, gin.H{
				"code":    "thread_locked",
				"message": err.Error(),
			})
		case strings.Contains(err.Error(), "invalid thread_id") || strings.Contains(err.Error(), "no valid fields"):
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    "validation_error",
				"message": err.Error(),
			})
		default:
			// Неизвестная ошибка - 500
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    "internal_error",
				"message": "an unexpected error occurred",
			})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Тред обновлен",
		"thread":  thread,
	})

}

func (t *ThreadHandler) CreateThread(c *gin.Context) {
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

	thread, wasCached, conflict, err := t.threadService.CreateThread(parseUUID, req, idempotencyKey)
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

func (t *ThreadHandler) DeleteThread(c *gin.Context) {
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

	threadID := c.Param("thread_id")

	if err := t.threadService.DeleteThread(parseUUID, threadID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "bad_request",
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "Тред удален",
	})
}
