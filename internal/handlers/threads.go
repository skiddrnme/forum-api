package handlers

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"stepik.leoscode.http/internal/service"
)

type ThreadsHandler struct{
	threadsService *service.Thread
}

func NewThreadHandler(threadsService *service.Thread) *ThreadsHandler{
	return &ThreadsHandler{
		threadsService: threadsService,
	}
}

func (t *ThreadsHandler) GetThreads(c *gin.Context){
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	if limit > 100{
		limit = 100
	}

	threads, err := t.threadsService.GetThreads()
	if err != nil{
		fmt.Println("Ошибка при загрузке тредов", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"limit": limit,
		"offset": offset,
		"data": threads,
	})
}