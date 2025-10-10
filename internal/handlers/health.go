package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"HPCP-P/API-Gateway-Server/internal/grpc"
)

type HealthHandler struct {
	authClient *grpc.AuthClient
	logger     *logrus.Logger
}

func NewHealthHandler(authClient *grpc.AuthClient) *HealthHandler {
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)

	return &HealthHandler{
		authClient: authClient,
		logger:     logger,
	}
}

func (hh *HealthHandler) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "healthy",
		"timestamp": time.Now().Format(time.RFC3339),
		"service":   "API Gateway Server",
		"version":   "1.0.0",
	})
}

func (hh *HealthHandler) ReadinessCheck(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	checks := make(map[string]interface{})
	allHealthy := true

	// 检查认证服务连接
	if _, err := hh.authClient.ValidateToken(ctx, "health-check-token"); err != nil {
		checks["auth_service"] = map[string]interface{}{
			"status": "unhealthy",
			"error":  err.Error(),
		}
		allHealthy = false
		hh.logger.WithError(err).Error("Auth service health check failed")
	} else {
		checks["auth_service"] = map[string]interface{}{
			"status": "healthy",
		}
	}

	// 这里可以添加更多服务的健康检查
	// 例如：Kafka连接检查、数据库连接检查等

	status := "ready"
	statusCode := http.StatusOK
	if !allHealthy {
		status = "not_ready"
		statusCode = http.StatusServiceUnavailable
	}

	c.JSON(statusCode, gin.H{
		"status":    status,
		"timestamp": time.Now().Format(time.RFC3339),
		"checks":    checks,
	})
}
