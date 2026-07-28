package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	forum "stepik.leoscode.http/internal/gen/api"
	"stepik.leoscode.http/internal/service"
)

type PostHandler struct {
	postService *service.PostService
}

func NewPostHandler(postService *service.PostService) *PostHandler {
	return &PostHandler{
		postService: postService,
	}
}

func (p *PostHandler) GetPosts(c *gin.Context) {
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

	thread_id := c.Param("thread_id")

	posts, err := p.postService.GetPostsWithMeta(thread_id, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "internal_server",
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, posts)
}

func (p *PostHandler) CreatePost(c *gin.Context) {
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

	idempotencyKey := c.GetHeader("X-Idempotency-Key")
	if idempotencyKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "validation_error",
			"message": "X-Idempotency-Key обязателен",
		})
		return
	}

	var req forum.PostCreate
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "bad_request",
			"message": err.Error(),
		})
		return
	}

	if req.Content == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "validation_error",
			"message": "Content обязательны",
		})
		return
	}

	thread_id := c.Query("thread_id")

	post, wasCached, conflict, err := p.postService.CreatePost(thread_id, parseUUID, req, idempotencyKey)
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

	statusCode := http.StatusCreated
	if wasCached {
		statusCode = http.StatusOK // 200 для повторных запросов
	}

	c.JSON(statusCode, post)
}
