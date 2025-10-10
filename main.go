package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"HPCP-P/API-Gateway-Server/internal/config"
	"HPCP-P/API-Gateway-Server/internal/grpc"
	"HPCP-P/API-Gateway-Server/internal/handlers"
	"HPCP-P/API-Gateway-Server/internal/kafka"
	"HPCP-P/API-Gateway-Server/internal/middleware"
	"HPCP-P/API-Gateway-Server/internal/websocket"
)

func main() {
	// 加载配置
	cfg := config.Load()

	// 设置日志
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)
	logger.SetFormatter(&logrus.JSONFormatter{})

	logger.Info("Starting API Gateway Server...")

	// 初始化gRPC客户端
	authClient, err := grpc.NewAuthClient(cfg.GRPC.AuthServiceAddr)
	if err != nil {
		logger.WithError(err).Fatal("Failed to initialize auth gRPC client")
	}
	defer authClient.Close()

	// 初始化Kafka生产者
	kafkaProducer, err := kafka.NewProducer(cfg.Kafka.Brokers, cfg.Kafka.Auth.Username, cfg.Kafka.Auth.Password)
	if err != nil {
		logger.WithError(err).Fatal("Failed to initialize Kafka producer")
	}
	defer kafkaProducer.Close()

	// 初始化WebSocket代理
	wsProxy := websocket.NewProxyHandler(cfg.Services.DocumentServiceAddr)

	// 初始化处理器
	healthHandler := handlers.NewHealthHandler(authClient)

	// 初始化中间件
	authMiddleware := middleware.NewAuthMiddleware(authClient)
	analyticsMiddleware := middleware.NewAnalyticsMiddleware(kafkaProducer, cfg.Kafka.Topics.UserAnalytics)

	// 设置Gin模式
	if os.Getenv("GIN_MODE") == "" {
		gin.SetMode(gin.ReleaseMode)
	}

	// 创建Gin路由器
	r := gin.New()

	// 全局中间件
	r.Use(middleware.RequestIDMiddleware())
	r.Use(middleware.LoggingMiddleware())
	r.Use(gin.Recovery())

	// 健康检查路由（不需要认证）
	r.GET("/health", healthHandler.HealthCheck)
	r.GET("/ready", healthHandler.ReadinessCheck)

	// API版本分组
	v1 := r.Group("/api/v1")
	{
		// 需要认证的路由组
		authenticated := v1.Group("/")
		authenticated.Use(authMiddleware.RequireAuth())
		authenticated.Use(analyticsMiddleware.TrackUserActivity())
		{
			// WebSocket代理路由
			authenticated.GET("/ws/document", wsProxy.HandleWebSocket)

			// 文档相关路由
			documents := authenticated.Group("/documents")
			documents.Use(authMiddleware.RequirePermission("documents", "read"))
			{
				documents.GET("/", func(c *gin.Context) {
					c.JSON(200, gin.H{"message": "获取文档列表"})
				})
				documents.GET("/:id", func(c *gin.Context) {
					c.JSON(200, gin.H{"message": "获取文档详情", "id": c.Param("id")})
				})
			}

			// 需要写权限的文档操作
			documentsWrite := authenticated.Group("/documents")
			documentsWrite.Use(authMiddleware.RequirePermission("documents", "write"))
			{
				documentsWrite.POST("/", func(c *gin.Context) {
					c.JSON(200, gin.H{"message": "创建文档"})
				})
				documentsWrite.PUT("/:id", func(c *gin.Context) {
					c.JSON(200, gin.H{"message": "更新文档", "id": c.Param("id")})
				})
				documentsWrite.DELETE("/:id", func(c *gin.Context) {
					c.JSON(200, gin.H{"message": "删除文档", "id": c.Param("id")})
				})
			}

			// 用户信息路由
			users := authenticated.Group("/users")
			{
				users.GET("/me", func(c *gin.Context) {
					userID := c.GetString("user_id")
					username := c.GetString("username")
					c.JSON(200, gin.H{
						"user_id":  userID,
						"username": username,
					})
				})
			}
		}
	}

	// 根路由
	r.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"service": "HPCP API Gateway Server",
			"version": "1.0.0",
			"status":  "running",
		})
	})

	// 优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		logger.WithField("port", cfg.Server.Port).Info("API Gateway Server started")
		if err := r.Run(":" + cfg.Server.Port); err != nil {
			logger.WithError(err).Fatal("Failed to start server")
		}
	}()

	<-quit
	logger.Info("Shutting down API Gateway Server...")
}
