package handlers

import (
	"github.com/gin-gonic/gin"
	"stepik.leoscode.http/internal/service"
)

type PostHandler struct {
	postService *service.PostService
}

func (p *PostHandler) NewPostHandler(postService *service.PostService) *PostHandler{
	return &PostHandler{
		postService: postService,
	}
}

func (p *PostHandler) GetPosts(c *gin.Context){
	
}