package middleware

import (
	"bytes"
	"encoding/json"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"APIGatewayServer/internal/kafka"
)

type AnalyticsMiddleware struct {
	producer *kafka.Producer
	topic    string
	logger   *logrus.Logger
}

func NewAnalyticsMiddleware(producer *kafka.Producer, topic string) *AnalyticsMiddleware {
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)

	return &AnalyticsMiddleware{
		producer: producer,
		topic:    topic,
		logger:   logger,
	}
}

// TrackLoginEvent 专门追踪登录事件的中间件
// 登录时用户还未认证，需要从响应中提取用户信息
func (am *AnalyticsMiddleware) TrackLoginEvent() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 只追踪登录路由
		if c.Request.URL.Path != "/api/v1/auth/login" {
			c.Next()
			return
		}

		start := time.Now()

		// 使用 ResponseWriter wrapper 捕获响应体
		blw := &bodyLogWriter{body: bytes.NewBufferString(""), ResponseWriter: c.Writer}
		c.Writer = blw

		c.Next()

		// 只追踪成功登录（状态码 200）
		statusCode := c.Writer.Status()
		if statusCode != 200 {
			return
		}

		// 尝试从响应中提取用户信息
		var userID, username string
		responseBody := blw.body.String()
		if responseBody != "" {
			var loginResponse map[string]interface{}
			if err := json.Unmarshal([]byte(responseBody), &loginResponse); err == nil {
				// 尝试提取 user_id 和 username（支持多种响应格式）
				if uid, ok := loginResponse["user_id"].(string); ok {
					userID = uid
				} else if uid, ok := loginResponse["userId"].(string); ok {
					userID = uid
				} else if data, ok := loginResponse["data"].(map[string]interface{}); ok {
					if uid, ok := data["user_id"].(string); ok {
						userID = uid
					} else if uid, ok := data["userId"].(string); ok {
						userID = uid
					}
					if uname, ok := data["username"].(string); ok {
						username = uname
					}
				}
				if uname, ok := loginResponse["username"].(string); ok {
					username = uname
				}
			}
		}

		// 创建登录事件
		event := &kafka.UserAnalyticsEvent{
			EventType: "user.login",
			UserID:    userID, // 如果无法提取则为空
			Username:  username,
			Metadata: map[string]interface{}{
				"ip":          c.ClientIP(),
				"user_agent":  c.Request.UserAgent(),
				"duration_ms": time.Since(start).Milliseconds(),
				"status_code": statusCode,
				"method":      c.Request.Method,
				"path":        c.Request.URL.Path,
			},
			Timestamp: start.UTC().Format(time.RFC3339),
		}

		// 异步发送到Kafka，避免阻塞请求响应
		go func() {
			if err := am.producer.SendUserAnalyticsEvent(am.topic, event); err != nil {
				am.logger.WithError(err).WithFields(logrus.Fields{
					"user_id":    userID,
					"event_type": "user.login",
					"path":       c.Request.URL.Path,
				}).Error("Failed to send login analytics event")
			}
		}()
	}
}

// bodyLogWriter 用于捕获响应体的 ResponseWriter wrapper
type bodyLogWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w *bodyLogWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

func (w *bodyLogWriter) WriteString(s string) (int, error) {
	w.body.WriteString(s)
	return w.ResponseWriter.WriteString(s)
}

func (am *AnalyticsMiddleware) TrackUserActivity() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		// 只有在用户已认证的情况下才发送分析事件
		userID, exists := c.Get("user_id")
		if !exists {
			return
		}

		// 推断 event_type（仅追踪有意义的业务事件，忽略 4xx/5xx 失败请求）
		statusCode := c.Writer.Status()
		eventType := inferEventType(c.Request.Method, c.Request.URL.Path, statusCode)
		if eventType == "" {
			return
		}

		// 获取用户名
		username, _ := c.Get("username")
		usernameStr, _ := username.(string)

		// 从路径中提取 document_id
		documentID := extractDocumentID(c.Request.URL.Path)

		// 创建符合规范的用户行为分析事件
		event := &kafka.UserAnalyticsEvent{
			EventType:  eventType,
			UserID:     userID.(string),
			Username:   usernameStr,
			DocumentID: documentID,
			Metadata: map[string]interface{}{
				"ip":          c.ClientIP(),
				"user_agent":  c.Request.UserAgent(),
				"duration_ms": time.Since(start).Milliseconds(),
				"status_code": statusCode,
				"method":      c.Request.Method,
				"path":        c.Request.URL.Path,
			},
			Timestamp: start.UTC().Format(time.RFC3339),
		}

		// 异步发送到Kafka，避免阻塞请求响应
		go func() {
			if err := am.producer.SendUserAnalyticsEvent(am.topic, event); err != nil {
				am.logger.WithError(err).WithFields(logrus.Fields{
					"user_id":    userID,
					"event_type": eventType,
					"path":       c.Request.URL.Path,
				}).Error("Failed to send analytics event")
			}
		}()
	}
}

// inferEventType 根据 HTTP 方法、请求路径和响应状态码推断 Kafka 事件类型。
// 仅对成功响应（2xx）生成事件；路径不匹配时返回空字符串。
func inferEventType(method, path string, statusCode int) string {
	// 只追踪成功请求
	if statusCode < 200 || statusCode >= 300 {
		return ""
	}

	// 文档相关路由：/api/v1/documents/*
	if strings.HasPrefix(path, "/api/v1/documents") {
		// 截取 /api/v1/documents/ 之后的部分
		suffix := strings.TrimPrefix(path, "/api/v1/documents/")

		switch method {
		case "POST":
			// POST /api/v1/documents/ → 创建文档
			if suffix == "" || suffix == "?" {
				return "document.create"
			}
		case "GET":
			// GET /api/v1/documents/{id} → 查看文档（不含子路径）
			if suffix != "" && !strings.Contains(suffix, "/") {
				return "document.view"
			}
		case "PUT":
			// PUT /api/v1/documents/{id} → 编辑文档（不含子路径）
			if suffix != "" && !strings.Contains(suffix, "/") {
				return "document.edit"
			}
		case "DELETE":
			// DELETE /api/v1/documents/{id} → 删除文档（不含子路径）
			if suffix != "" && !strings.Contains(suffix, "/") {
				return "document.delete"
			}
		}
	}

	return ""
}

// extractDocumentID 从请求路径中提取文档 ID。
// 例如：/api/v1/documents/doc-123 → "doc-123"
//
//	/api/v1/documents/        → ""
func extractDocumentID(path string) string {
	const prefix = "/api/v1/documents/"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	suffix := strings.TrimPrefix(path, prefix)
	// 取第一段（不含子路径，如 /permissions）
	if idx := strings.Index(suffix, "/"); idx != -1 {
		suffix = suffix[:idx]
	}
	return suffix
}
