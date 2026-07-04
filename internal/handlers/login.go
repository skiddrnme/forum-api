package handlers

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"stepik.leoscode.http/internal/service"
)


func Login(c *gin.Context){
	username := c.PostForm("username")
	password := c.PostForm("password")

	authService := service.NewAuthService()
	if username == "" && password == ""{
		c.JSON(http.StatusBadRequest, gin.H{"ошибка": "пустые поля"})
	}

	id, err := authService.Login(username, password)
	if err != nil{
		fmt.Println("Ошибка")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"token": id,
	})
	

}