package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

func LoggingMiddleware() gin.HandlerFunc {
	logger := logrus.New()
	logger.SetFormatter(&logrus.JSONFormatter{})

	return gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		// 过滤健康检查路径，不记录日志
		if isHealthCheckPath(param.Path) {
			return ""
		}

		// 只记录真实的业务请求日志
		logger.WithFields(logrus.Fields{
			"status_code": param.StatusCode,
			"latency":     param.Latency,
			"client_ip":   param.ClientIP,
			"method":      param.Method,
			"path":        param.Path,
			"user_agent":  param.Request.UserAgent(),
			"error":       param.ErrorMessage,
			"timestamp":   param.TimeStamp.Format(time.RFC3339),
		}).Info("HTTP Request")

		return ""
	})
}

// isHealthCheckPath 判断是否为健康检查路径
func isHealthCheckPath(path string) bool {
	healthCheckPaths := []string{
		"/health",
		"/ready",
		"/healthz",
		"/readyz",
		"/livez",
	}

	for _, healthPath := range healthCheckPaths {
		if path == healthPath {
			return true
		}
	}

	return false
}

func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = generateRequestID()
		}

		c.Set("request_id", requestID)
		c.Header("X-Request-ID", requestID)
		c.Next()
	}
}

func generateRequestID() string {
	// 简单的请求ID生成，生产环境建议使用更复杂的算法
	return time.Now().Format("20060102150405") + "-" +
		string(rune(time.Now().UnixNano()%1000000))
}
