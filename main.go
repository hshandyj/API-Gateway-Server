package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"APIGatewayServer/internal/config"
	"APIGatewayServer/internal/grpc"
	"APIGatewayServer/internal/handlers"
	"APIGatewayServer/internal/kafka"
	"APIGatewayServer/internal/middleware"
	"APIGatewayServer/internal/websocket"
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

	// 初始化WebSocket代理（使用 ws:// 地址）
	wsProxy := websocket.NewProxyHandler(cfg.Services.DocumentServiceWSAddr, cfg.WebClientConfig.AllowedOrigins)

	// 初始化处理器
	healthHandler := handlers.NewHealthHandler(authClient)
	documentHandler := handlers.NewDocumentHandler(cfg.Services.DocumentServiceAddr)
	userinfoHandler := handlers.NewUserinfoHandler(cfg.Services.UserinfoServiceAddr)
	authHandler := handlers.NewAuthHandler(cfg.Services.AuthServiceHTTPAddr)
	// 初始化中间件
	authMiddleware := middleware.NewAuthMiddleware(authClient)
	analyticsMiddleware := middleware.NewAnalyticsMiddleware(kafkaProducer, cfg.Kafka.Topics.UserAnalytics)

	// 初始化限流中间件（从配置文件加载参数）
	rateLimitCfg := middleware.RateLimitConfig{
		GlobalQPS:  cfg.RateLimit.GlobalQPS,
		PerIPQPS:   cfg.RateLimit.PerIPQPS,
		PerUserQPS: cfg.RateLimit.PerUserQPS,
		BurstSize:  cfg.RateLimit.BurstSize,
	}

	// 初始化熔断器管理器（为每个下游服务创建独立实例，实现故障隔离）
	cbCfg := middleware.CircuitBreakerConfig{
		FailureThreshold:    cfg.CircuitBreaker.FailureThreshold,
		SuccessThreshold:    cfg.CircuitBreaker.SuccessThreshold,
		RecoveryTimeout:     time.Duration(cfg.CircuitBreaker.RecoveryTimeoutSec) * time.Second,
		HalfOpenMaxRequests: cfg.CircuitBreaker.HalfOpenMaxRequests,
	}
	cbManager := middleware.NewCircuitBreakerManager(
		[]string{"iam-auth-service", "document-service", "data-analysis-service"},
		cbCfg,
	)
	logger.WithFields(logrus.Fields{
		"rate_limit":      rateLimitCfg,
		"circuit_breaker": cbCfg,
	}).Info("Rate limiting and circuit breaker initialized")

	// 设置Gin模式
	if os.Getenv("GIN_MODE") == "" {
		gin.SetMode(gin.ReleaseMode)
	}

	// 创建Gin路由器
	r := gin.New()

	// 全局中间件
	r.Use(middleware.RequestIDMiddleware())
	r.Use(middleware.LoggingMiddleware())
	r.Use(middleware.CORSMiddleware(cfg.WebClientConfig.AllowedOrigins)) // 使用配置的允许来源
	r.Use(middleware.RateLimitMiddleware(rateLimitCfg))                  // 全局 + Per-IP 限流（CORS 之后、Auth 之前）
	r.Use(gin.Recovery())

	// 健康检查路由（不需要认证）
	r.GET("/health", healthHandler.HealthCheck)
	r.GET("/ready", healthHandler.ReadinessCheck)

	// API版本分组
	v1 := r.Group("/api/v1")
	{
		// 不需要认证的路由组（IAM 认证服务熔断器保护）
		publicAuth := v1.Group("/auth")
		publicAuth.Use(middleware.CircuitBreakerMiddleware(cbManager.Get("iam-auth-service")))
		publicAuth.Use(analyticsMiddleware.TrackLoginEvent()) // 追踪登录事件
		publicAuth.Any("/*path", authHandler.ProxyRequest)

		// 需要认证的路由组
		authenticated := v1.Group("/")
		authenticated.Use(authMiddleware.RequireAuth())
		authenticated.Use(middleware.UserRateLimitMiddleware(rateLimitCfg)) // Per-User 限流（Auth 之后）
		authenticated.Use(analyticsMiddleware.TrackUserActivity())
		{
			// WebSocket代理路由（文档服务熔断器保护）
			authenticated.GET("/ws/document", wsProxy.HandleWebSocket)

			// 文档相关路由（文档服务熔断器保护）
			documents := authenticated.Group("/documents")
			documents.Use(middleware.CircuitBreakerMiddleware(cbManager.Get("document-service")))
			documents.Any("/*path", documentHandler.ProxyRequest)

			// 数据分析服务路由（数据分析服务熔断器保护）
			userinfo := authenticated.Group("/userinfo")
			userinfo.Use(middleware.CircuitBreakerMiddleware(cbManager.Get("data-analysis-service")))
			userinfo.Any("/*path", userinfoHandler.ProxyRequest)
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
