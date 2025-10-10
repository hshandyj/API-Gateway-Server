package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"HPCP-P/API-Gateway-Server/internal/kafka"
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

func (am *AnalyticsMiddleware) TrackUserActivity() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		// 只有在用户已认证的情况下才发送分析事件
		userID, exists := c.Get("user_id")
		if !exists {
			return
		}

		// 创建用户行为分析事件
		event := &kafka.UserAnalyticsEvent{
			UserID:    userID.(string),
			Action:    c.Request.Method,
			Resource:  c.Request.URL.Path,
			Timestamp: start,
			Metadata: map[string]interface{}{
				"status_code":   c.Writer.Status(),
				"response_time": time.Since(start).Milliseconds(),
				"request_size":  c.Request.ContentLength,
				"response_size": c.Writer.Size(),
				"query_params":  c.Request.URL.RawQuery,
			},
			IP:        c.ClientIP(),
			UserAgent: c.Request.UserAgent(),
		}

		// 异步发送到Kafka，避免阻塞请求响应
		go func() {
			if err := am.producer.SendUserAnalyticsEvent(am.topic, event); err != nil {
				am.logger.WithError(err).WithFields(logrus.Fields{
					"user_id": userID,
					"path":    c.Request.URL.Path,
					"method":  c.Request.Method,
				}).Error("Failed to send analytics event")
			}
		}()
	}
}
