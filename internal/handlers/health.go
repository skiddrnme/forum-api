package handlers

import (
	"net/http"
	"github.com/gin-gonic/gin"
)

func Health(c *gin.Context) {
	reqID := c.GetHeader("X-Request-Id")
	if reqID != "" {
		c.Header("X-Request-Id", reqID)
	}
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
	})
	
}
