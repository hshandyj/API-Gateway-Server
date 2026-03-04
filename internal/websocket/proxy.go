package websocket

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

type ProxyHandler struct {
	logger             *logrus.Logger
	upgrader           websocket.Upgrader
	documentServiceURL string
}

func NewProxyHandler(documentServiceURL string, allowedOrigins []string) *ProxyHandler {
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)

	return &ProxyHandler{
		logger: logger,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				// 在生产环境中应该实现更严格的origin检查
				origin := r.Header.Get("Origin")

				for _, allowed := range allowedOrigins {
					if origin == allowed {
						return true
					}
				}

				if strings.HasPrefix(origin, "http://localhost") {
					return true
				}

				logger.Warn("拒绝非法 Origin:", origin)
				return false
			},
		},
		documentServiceURL: documentServiceURL,
	}
}

func (ph *ProxyHandler) HandleWebSocket(c *gin.Context) {
	// 从上下文中获取用户信息（由认证中间件设置）
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未认证的用户"})
		return
	}
	username, _ := c.Get("username")

	// 从客户端请求中获取 doc_id
	docID := c.Query("doc_id")
	if docID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 doc_id 参数", "code": "MISSING_DOC_ID"})
		return
	}

	// 构建到文档服务的WebSocket连接URL
	documentURL, err := url.Parse(ph.documentServiceURL)
	if err != nil {
		ph.logger.WithError(err).Error("Invalid document service URL")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "服务配置错误"})
		return
	}

	// 将 doc_id 作为查询参数传给后端文档服务
	query := documentURL.Query()
	query.Set("doc_id", docID)
	documentURL.RawQuery = query.Encode()

	// 将用户身份信息通过 HTTP Header 传给后端文档服务
	// 后端 GatewayAuthMiddleware 从 X-User-ID / X-User-Name Header 中读取身份信息
	dialHeader := http.Header{}
	dialHeader.Set("X-User-ID", userID.(string))
	if username != nil {
		dialHeader.Set("X-User-Name", username.(string))
	}

	// ===== 关键：先连接后端服务，成功后再升级客户端连接 =====
	// 这样后端不可用时可以直接返回 HTTP 错误，而不是升级后再断开
	serviceConn, _, err := websocket.DefaultDialer.Dial(documentURL.String(), dialHeader)
	if err != nil {
		ph.logger.WithError(err).WithFields(logrus.Fields{
			"user_id":     userID,
			"doc_id":      docID,
			"service_url": documentURL.String(),
		}).Error("Failed to connect to document service")
		c.JSON(http.StatusBadGateway, gin.H{"error": "文档服务连接失败"})
		return
	}
	defer serviceConn.Close()

	// 升级客户端 HTTP 连接为 WebSocket 连接
	clientConn, err := ph.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		ph.logger.WithError(err).Error("Failed to upgrade connection to WebSocket")
		return
	}
	defer clientConn.Close()

	ph.logger.WithFields(logrus.Fields{
		"user_id":     userID,
		"doc_id":      docID,
		"service_url": documentURL.String(),
	}).Info("WebSocket proxy connection established")

	// 启动双向代理
	// 使用带缓冲的 channel（容量 2），两个协程各发送一次信号，避免 "send on closed channel" panic
	done := make(chan struct{}, 2)

	// 客户端到服务端的消息转发
	go func() {
		defer func() { done <- struct{}{} }()
		for {
			messageType, message, err := clientConn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					ph.logger.WithError(err).Error("Client connection error")
				}
				break
			}

			// 这里可以添加消息过滤、日志记录等逻辑
			ph.logMessage("client->service", userID.(string), messageType, message)

			if err := serviceConn.WriteMessage(messageType, message); err != nil {
				ph.logger.WithError(err).Error("Failed to forward message to service")
				break
			}
		}
	}()

	// 服务端到客户端的消息转发
	go func() {
		defer func() { done <- struct{}{} }()
		for {
			messageType, message, err := serviceConn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					ph.logger.WithError(err).Error("Service connection error")
				}
				break
			}

			// 这里可以添加消息过滤、转换等逻辑
			ph.logMessage("service->client", userID.(string), messageType, message)

			if err := clientConn.WriteMessage(messageType, message); err != nil {
				ph.logger.WithError(err).Error("Failed to forward message to client")
				break
			}
		}
	}()

	// 任意一端断开，主协程退出，defer 关闭两侧连接，另一协程也会自然退出
	// 第二次发送写入缓冲区，不会 panic
	<-done
	ph.logger.WithField("user_id", userID).Info("WebSocket proxy connection closed")
}

func (ph *ProxyHandler) logMessage(direction, userID string, messageType int, message []byte) {
	// 截断长消息以避免日志过大
	messagePreview := string(message)
	if len(messagePreview) > 200 {
		messagePreview = messagePreview[:200] + "..."
	}

	ph.logger.WithFields(logrus.Fields{
		"direction":    direction,
		"user_id":      userID,
		"message_type": messageType,
		"message_size": len(message),
		"preview":      messagePreview,
	}).Debug("WebSocket message proxied")
}
