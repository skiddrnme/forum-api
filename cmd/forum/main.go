package main

import (
	"github.com/gin-gonic/gin"
	"stepik.leoscode.http/internal/handlers"
	"stepik.leoscode.http/internal/service"
)

func main() {
	r := gin.Default()
	
	authService := service.NewAuthService()
	authHandler := handlers.NewAuthHandler(authService)

	threadsService := service.NewThreadService()


	r.GET("/internal/v1/health", handlers.Health)
	r.GET("/api/v1/threads", handlers.GetThreads)

	r.POST("/api/v1/auth/login", authHandler.Login)

	r.Run(":8080")
}
