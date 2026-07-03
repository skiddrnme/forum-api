package main

import (
	"github.com/gin-gonic/gin"
	"stepik.leoscode.http/internal/handlers"
)

func main() {
	r := gin.Default()
	
	r.GET("/internal/v1/health", handlers.Health)
	r.Run(":8080")
}
