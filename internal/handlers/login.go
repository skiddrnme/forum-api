package handlers

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"stepik.leoscode.http/internal/service"
)

type AuthHandler struct{
	authService *service.AuthService
}

func NewAuthHandler(authService *service.AuthService) *AuthHandler{
	return &AuthHandler{
		authService: authService,
	}
}

func (h *AuthHandler) Login(c *gin.Context){
	username := c.PostForm("username")
	password := c.PostForm("password")

	
	if username == "" && password == ""{
		c.JSON(http.StatusBadRequest, gin.H{"ошибка": "пустые поля"})
		return 
	}

	id, err := h.authService.Login(username, password)
	if err != nil{
		fmt.Println("Ошибка")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"user_id": id,
	})
	

}