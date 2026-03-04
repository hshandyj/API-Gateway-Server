package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// CORSMiddleware 创建 CORS 中间件，支持配置允许的来源
// 注意：不能同时使用 Access-Control-Allow-Origin: * 和 Access-Control-Allow-Credentials: true
// 因此需要根据请求的 Origin 动态设置允许的来源
func CORSMiddleware(allowedOrigins []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")

		// 检查请求的 Origin 是否在允许列表中
		allowedOrigin := ""
		if origin != "" {
			for _, allowed := range allowedOrigins {
				if origin == allowed {
					allowedOrigin = origin
					break
				}
			}
			// 开发环境：允许 localhost 的所有端口
			if allowedOrigin == "" && strings.HasPrefix(origin, "http://localhost:") {
				allowedOrigin = origin
			}
		}

		// 如果没有匹配的 Origin，不设置 CORS 头（浏览器会阻止跨域请求）
		if allowedOrigin == "" {
			c.Next()
			return
		}

		// 设置 CORS 响应头（只设置一次，避免重复）
		c.Header("Access-Control-Allow-Origin", allowedOrigin)
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS,PATCH")
		c.Header("Access-Control-Allow-Headers", "Content-Type,Authorization,X-Request-ID")
		c.Header("Access-Control-Max-Age", "3600")

		// 处理 OPTIONS 预检请求
		if c.Request.Method == "OPTIONS" {
			c.Status(200)
			c.Abort()
			return
		}

		c.Next()
	}
}
