package handlers

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"stepik.leoscode.http/internal/service"
)

type AttachmentHandler struct {
	attachmentService *service.AttachmentService
}

func NewAttachmentHandler(attachmentService *service.AttachmentService) *AttachmentHandler {
	return &AttachmentHandler{
		attachmentService: attachmentService,
	}
}

func (a *AttachmentHandler) CreateAttachment(c *gin.Context) {
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

	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file uploaded"})
		return
	}

	// caption := c.PostForm("caption")

	file, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer file.Close()

	attachment, err := a.attachmentService.UploadAttachment(threadID, parseUUID, file, fileHeader)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("Attachment %s uploaded succesfully", attachment)})
}

func (a *AttachmentHandler) GetAttachmentMeta(c *gin.Context) {
	attachmentId := c.Param("attachment_id")
	parseUUID, err := uuid.Parse(attachmentId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "bad_request",
			"message": "Неверный формат Attachment ID",
		})
		return
	}

	attachment, err := a.attachmentService.GetAttachmentMeta(parseUUID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    "not_found",
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, attachment)
}

func (a *AttachmentHandler) SaveAttachment(c *gin.Context) {
	attachmentId := c.Param("attachment_id")
	parseUUID, err := uuid.Parse(attachmentId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "bad_request",
			"message": "Неверный формат Attachment ID",
		})
		return
	}

	attachment, err := a.attachmentService.GetAttachmentMeta(parseUUID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    "not_found",
			"message": "attachment not found",
		})
		return
	}

	filePath := filepath.Join("./uploads", attachment.Filename)

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    "not_found",
			"message": "file not found on disk",
		})
		return
	}

	c.File(filePath)
}
