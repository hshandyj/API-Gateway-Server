package websocket

import (
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

type ProxyHandler struct {
	logger             *logrus.Logger
	upgrader           websocket.Upgrader
	documentServiceURL string
}

func NewProxyHandler(documentServiceURL string) *ProxyHandler {
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)

	return &ProxyHandler{
		logger: logger,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				// 在生产环境中应该实现更严格的origin检查
				return true
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

	// 升级HTTP连接为WebSocket连接
	clientConn, err := ph.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		ph.logger.WithError(err).Error("Failed to upgrade connection to WebSocket")
		return
	}
	defer clientConn.Close()

	// 构建到文档服务的WebSocket连接URL
	documentURL, err := url.Parse(ph.documentServiceURL)
	if err != nil {
		ph.logger.WithError(err).Error("Invalid document service URL")
		return
	}

	// 添加用户ID到查询参数
	query := documentURL.Query()
	query.Set("user_id", userID.(string))
	documentURL.RawQuery = query.Encode()

	// 连接到文档服务
	serviceConn, _, err := websocket.DefaultDialer.Dial(documentURL.String(), nil)
	if err != nil {
		ph.logger.WithError(err).Error("Failed to connect to document service")
		clientConn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "服务连接失败"))
		return
	}
	defer serviceConn.Close()

	ph.logger.WithFields(logrus.Fields{
		"user_id":     userID,
		"service_url": documentURL.String(),
	}).Info("WebSocket proxy connection established")

	// 启动双向代理
	done := make(chan struct{})

	// 客户端到服务端的消息转发
	go func() {
		defer close(done)
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
		done <- struct{}{}
	}()

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
