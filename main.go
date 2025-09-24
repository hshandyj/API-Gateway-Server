package main

import (
	"fmt"

	"github.com/gin-gonic/gin"
)

func main() {
	fmt.Println("Hello, World!")

	// 创建Gin路由器
	r := gin.Default()

	// 添加一个简单的路由
	r.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "Hello from API Gateway Server!",
		})
	})

	// 启动服务器
	r.Run(":8080")
}
