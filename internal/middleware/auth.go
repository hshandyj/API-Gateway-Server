package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"APIGatewayServer/internal/grpc"
)

type AuthMiddleware struct {
	authClient *grpc.AuthClient
	logger     *logrus.Logger
}

func NewAuthMiddleware(authClient *grpc.AuthClient) *AuthMiddleware {
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)

	return &AuthMiddleware{
		authClient: authClient,
		logger:     logger,
	}
}

func (am *AuthMiddleware) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从请求头中获取token
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "缺少认证token",
				"code":  "MISSING_TOKEN",
			})
			c.Abort()
			return
		}

		// 解析Bearer token
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == authHeader {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "无效的token格式",
				"code":  "INVALID_TOKEN_FORMAT",
			})
			c.Abort()
			return
		}

		// 调用认证服务验证token
		resp, err := am.authClient.ValidateToken(c.Request.Context(), token)
		if err != nil {
			am.logger.WithError(err).Error("Failed to validate token")
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "认证服务不可用",
				"code":  "AUTH_SERVICE_ERROR",
			})
			c.Abort()
			return
		}

		if !resp.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": resp.ErrorMessage,
				"code":  "INVALID_TOKEN",
			})
			c.Abort()
			return
		}

		// 将用户信息存储到上下文中
		c.Set("user_id", resp.UserId)
		c.Set("username", resp.Username)
		c.Set("token", token)

		am.logger.WithFields(logrus.Fields{
			"user_id":  resp.UserId,
			"username": resp.Username,
			"path":     c.Request.URL.Path,
			"method":   c.Request.Method,
		}).Info("User authenticated successfully")

		c.Next()
	}
}

func (am *AuthMiddleware) RequirePermission(resource, action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "用户未认证",
				"code":  "USER_NOT_AUTHENTICATED",
			})
			c.Abort()
			return
		}

		// 检查用户权限
		resp, err := am.authClient.CheckPermission(c.Request.Context(),
			userID.(string), resource, action)
		if err != nil {
			am.logger.WithError(err).Error("Failed to check permission")
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "权限检查服务不可用",
				"code":  "PERMISSION_SERVICE_ERROR",
			})
			c.Abort()
			return
		}

		if !resp.Allowed {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "权限不足",
				"code":  "INSUFFICIENT_PERMISSION",
			})
			c.Abort()
			return
		}

		am.logger.WithFields(logrus.Fields{
			"user_id":  userID,
			"resource": resource,
			"action":   action,
		}).Info("Permission check passed")

		c.Next()
	}
}
