package middleware

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// ──────────────────────────────────────────
// 熔断器状态
// ──────────────────────────────────────────

// CircuitState 熔断器三态
type CircuitState int

const (
	StateClosed   CircuitState = iota // 正常：请求正常转发
	StateOpen                         // 熔断：直接拒绝请求，返回 503
	StateHalfOpen                     // 半开：放行有限请求试探恢复
)

// String 返回状态名称
func (s CircuitState) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// ──────────────────────────────────────────
// 熔断器配置
// ──────────────────────────────────────────

// CircuitBreakerConfig 熔断器配置参数
type CircuitBreakerConfig struct {
	FailureThreshold    int           // 连续失败次数阈值，超过后切换到 Open
	SuccessThreshold    int           // Half-Open 状态下连续成功次数阈值，达到后恢复 Closed
	RecoveryTimeout     time.Duration // Open 状态持续时间，超时后切换到 Half-Open
	HalfOpenMaxRequests int           // Half-Open 状态下允许放行的最大请求数
}

// DefaultCircuitBreakerConfig 返回合理的默认熔断配置
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		FailureThreshold:    5,
		SuccessThreshold:    3,
		RecoveryTimeout:     30 * time.Second,
		HalfOpenMaxRequests: 3,
	}
}

// ──────────────────────────────────────────
// CircuitBreaker 单个服务的熔断器实例
// ──────────────────────────────────────────

// CircuitBreaker 三态熔断器
type CircuitBreaker struct {
	mu     sync.RWMutex
	name   string // 服务名称（用于日志和响应）
	config CircuitBreakerConfig
	logger *logrus.Logger

	state            CircuitState // 当前状态
	failureCount     int          // 连续失败计数
	successCount     int          // Half-Open 下连续成功计数
	halfOpenRequests int          // Half-Open 下已放行的请求数
	lastFailureTime  time.Time    // 最后一次失败时间（用于计算恢复超时）
}

// NewCircuitBreaker 创建指定服务的熔断器
func NewCircuitBreaker(name string, cfg CircuitBreakerConfig) *CircuitBreaker {
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)

	return &CircuitBreaker{
		name:   name,
		config: cfg,
		logger: logger,
		state:  StateClosed,
	}
}

// Allow 判断当前请求是否允许通过
// 返回 true 表示放行，false 表示被熔断
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		return true

	case StateOpen:
		// 检查恢复超时是否已过
		if time.Since(cb.lastFailureTime) > cb.config.RecoveryTimeout {
			// 超时，切换到 Half-Open 试探
			cb.toHalfOpen()
			return true
		}
		return false

	case StateHalfOpen:
		// Half-Open 下限制放行请求数
		if cb.halfOpenRequests < cb.config.HalfOpenMaxRequests {
			cb.halfOpenRequests++
			return true
		}
		return false

	default:
		return true
	}
}

// RecordSuccess 记录一次成功的请求
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		// 重置连续失败计数
		cb.failureCount = 0

	case StateHalfOpen:
		cb.successCount++
		if cb.successCount >= cb.config.SuccessThreshold {
			// 连续成功达标，恢复 Closed 状态
			cb.toClosed()
		}
	}
}

// RecordFailure 记录一次失败的请求
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		cb.failureCount++
		if cb.failureCount >= cb.config.FailureThreshold {
			// 连续失败达标，切换到 Open
			cb.toOpen()
		}

	case StateHalfOpen:
		// Half-Open 下任何一次失败立即重新打开
		cb.toOpen()
	}
}

// State 获取当前状态（只读）
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// ── 状态转换（内部方法，调用时必须已持有锁）──

func (cb *CircuitBreaker) toOpen() {
	cb.state = StateOpen
	cb.lastFailureTime = time.Now()
	cb.logger.WithFields(logrus.Fields{
		"service":        cb.name,
		"failure_count":  cb.failureCount,
		"recovery_after": cb.config.RecoveryTimeout,
	}).Warn("Circuit breaker OPENED — service requests will be rejected")
}

func (cb *CircuitBreaker) toHalfOpen() {
	cb.state = StateHalfOpen
	cb.halfOpenRequests = 1 // 当前这个请求算第一个
	cb.successCount = 0
	cb.logger.WithFields(logrus.Fields{
		"service":      cb.name,
		"max_requests": cb.config.HalfOpenMaxRequests,
	}).Info("Circuit breaker HALF-OPEN — probing service recovery")
}

func (cb *CircuitBreaker) toClosed() {
	prevState := cb.state
	cb.state = StateClosed
	cb.failureCount = 0
	cb.successCount = 0
	cb.halfOpenRequests = 0
	if prevState != StateClosed {
		cb.logger.WithField("service", cb.name).Info("Circuit breaker CLOSED — service recovered")
	}
}

// ──────────────────────────────────────────
// CircuitBreakerManager 管理多个服务的熔断器
// ──────────────────────────────────────────

// CircuitBreakerManager 为每个下游服务维护独立的熔断器（故障隔离）
type CircuitBreakerManager struct {
	breakers map[string]*CircuitBreaker
}

// NewCircuitBreakerManager 创建熔断器管理器，为每个服务名创建独立实例
func NewCircuitBreakerManager(serviceNames []string, cfg CircuitBreakerConfig) *CircuitBreakerManager {
	m := &CircuitBreakerManager{
		breakers: make(map[string]*CircuitBreaker, len(serviceNames)),
	}
	for _, name := range serviceNames {
		m.breakers[name] = NewCircuitBreaker(name, cfg)
	}
	return m
}

// Get 获取指定服务的熔断器
func (m *CircuitBreakerManager) Get(serviceName string) *CircuitBreaker {
	return m.breakers[serviceName]
}

// ──────────────────────────────────────────
// Gin 中间件工厂
// ──────────────────────────────────────────

// CircuitBreakerMiddleware 为指定服务创建熔断器 Gin 中间件
//
// 工作流程：
//  1. 请求到达时调用 Allow() 判断是否放行
//  2. 放行 → 执行后续 Handler → 根据响应状态码判定成功/失败
//  3. 拒绝 → 直接返回 503 Service Unavailable
//
// 判定规则：HTTP 状态码 >= 500 视为后端服务失败，触发失败计数
func CircuitBreakerMiddleware(cb *CircuitBreaker) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 检查熔断器是否允许请求通过
		if !cb.Allow() {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error":   fmt.Sprintf("服务 [%s] 暂时不可用，请稍后重试", cb.name),
				"code":    "SERVICE_CIRCUIT_OPEN",
				"service": cb.name,
			})
			c.Abort()
			return
		}

		// 放行请求，执行后续 Handler
		c.Next()

		// 根据响应状态码判定成功/失败
		statusCode := c.Writer.Status()
		if statusCode >= http.StatusInternalServerError {
			// 5xx 视为后端服务故障
			cb.RecordFailure()
		} else {
			cb.RecordSuccess()
		}
	}
}
