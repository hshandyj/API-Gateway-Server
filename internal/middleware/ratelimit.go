package middleware

import (
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
)

// ──────────────────────────────────────────
// 限流配置
// ──────────────────────────────────────────

// RateLimitConfig 限流配置参数
type RateLimitConfig struct {
	GlobalQPS  float64 // 全局 QPS 上限（所有请求合计）
	PerIPQPS   float64 // 单 IP 每秒请求上限
	PerUserQPS float64 // 单用户每秒请求上限（认证后）
	BurstSize  int     // 令牌桶突发容量（允许短时间内超出 QPS 的请求数）
}

// DefaultRateLimitConfig 返回合理的默认限流配置
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		GlobalQPS:  1000,
		PerIPQPS:   100,
		PerUserQPS: 50,
		BurstSize:  20,
	}
}

// ──────────────────────────────────────────
// 限流器管理（Per-IP / Per-User 独立桶）
// ──────────────────────────────────────────

// limiterEntry 单个限流器条目，附带最后访问时间用于过期清理
type limiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// limiterStore 管理一组键控限流器（按 IP 或 user_id）
type limiterStore struct {
	mu       sync.RWMutex
	limiters map[string]*limiterEntry
	rate     rate.Limit
	burst    int
}

// newLimiterStore 创建限流器存储
func newLimiterStore(r float64, burst int) *limiterStore {
	s := &limiterStore{
		limiters: make(map[string]*limiterEntry),
		rate:     rate.Limit(r),
		burst:    burst,
	}
	// 后台定期清理过期的限流器（5分钟未活跃则移除，节省内存）
	go s.cleanupLoop()
	return s
}

// getLimiter 获取或创建指定 key 的限流器
func (s *limiterStore) getLimiter(key string) *rate.Limiter {
	s.mu.RLock()
	entry, exists := s.limiters[key]
	s.mu.RUnlock()

	if exists {
		s.mu.Lock()
		entry.lastSeen = time.Now()
		s.mu.Unlock()
		return entry.limiter
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 双重检查，避免并发创建
	if entry, exists := s.limiters[key]; exists {
		entry.lastSeen = time.Now()
		return entry.limiter
	}

	limiter := rate.NewLimiter(s.rate, s.burst)
	s.limiters[key] = &limiterEntry{
		limiter:  limiter,
		lastSeen: time.Now(),
	}
	return limiter
}

// cleanupLoop 定期清理超过 5 分钟未活跃的限流器
func (s *limiterStore) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.Lock()
		threshold := time.Now().Add(-5 * time.Minute)
		for key, entry := range s.limiters {
			if entry.lastSeen.Before(threshold) {
				delete(s.limiters, key)
			}
		}
		s.mu.Unlock()
	}
}

// ──────────────────────────────────────────
// RateLimitMiddleware
// ──────────────────────────────────────────

// RateLimitMiddleware 创建三维限流中间件（全局 + Per-IP + Per-User）
//
// 执行顺序：
//  1. 全局限流 — 超出则直接 429
//  2. Per-IP 限流 — 超出则 429
//  3. Per-User 限流 — 认证后生效，超出则 429（未认证请求跳过此层）
func RateLimitMiddleware(cfg RateLimitConfig) gin.HandlerFunc {
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)

	// 全局令牌桶
	globalLimiter := rate.NewLimiter(rate.Limit(cfg.GlobalQPS), cfg.BurstSize*10)
	// Per-IP 令牌桶存储
	ipStore := newLimiterStore(cfg.PerIPQPS, cfg.BurstSize)

	return func(c *gin.Context) {
		now := time.Now()

		// ── 第一层：全局限流 ──
		if !globalLimiter.Allow() {
			writeRateLimitResponse(c, cfg.GlobalQPS, 0, now)
			logger.WithFields(logrus.Fields{
				"client_ip": c.ClientIP(),
				"path":      c.Request.URL.Path,
				"dimension": "global",
			}).Warn("Rate limit exceeded")
			return
		}

		// ── 第二层：Per-IP 限流 ──
		clientIP := c.ClientIP()
		ipLimiter := ipStore.getLimiter(clientIP)
		if !ipLimiter.Allow() {
			remaining := int(ipLimiter.Tokens())
			writeRateLimitResponse(c, cfg.PerIPQPS, remaining, now)
			logger.WithFields(logrus.Fields{
				"client_ip": clientIP,
				"path":      c.Request.URL.Path,
				"dimension": "per_ip",
			}).Warn("Rate limit exceeded")
			return
		}

		// ── 第三层：Per-User 限流（仅认证后生效）──
		// 注意：此中间件注册在 Auth 之前，所以这里检查 user_id 是否已设置
		// 认证路由中 Auth 中间件会在 c.Next() 前设置 user_id，
		// 但由于本中间件先于 Auth 执行，user_id 在此刻尚未设置。
		// 因此 Per-User 限流通过 afterAuth 钩子在 c.Next() 返回后不做阻断，
		// 而是在 authenticated 路由组中单独添加 UserRateLimitMiddleware。
		// 这里只做全局和 Per-IP 限流。

		// 设置响应头（即使未触发限流也返回，便于客户端感知剩余配额）
		c.Header("X-RateLimit-Limit-Global", fmt.Sprintf("%.0f", cfg.GlobalQPS))
		c.Header("X-RateLimit-Limit-PerIP", fmt.Sprintf("%.0f", cfg.PerIPQPS))

		c.Next()
	}
}

// UserRateLimitMiddleware 创建 Per-User 限流中间件
// 应挂载在 Auth 中间件之后的认证路由组中
func UserRateLimitMiddleware(cfg RateLimitConfig) gin.HandlerFunc {
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)

	userStore := newLimiterStore(cfg.PerUserQPS, cfg.BurstSize)

	return func(c *gin.Context) {
		userID, exists := c.Get("user_id")
		if !exists {
			c.Next()
			return
		}

		userLimiter := userStore.getLimiter(userID.(string))
		if !userLimiter.Allow() {
			now := time.Now()
			remaining := int(userLimiter.Tokens())
			writeRateLimitResponse(c, cfg.PerUserQPS, remaining, now)
			logger.WithFields(logrus.Fields{
				"user_id":   userID,
				"path":      c.Request.URL.Path,
				"dimension": "per_user",
			}).Warn("Rate limit exceeded")
			return
		}

		// 设置用户级限流信息响应头
		remaining := int(userLimiter.Tokens())
		c.Header("X-RateLimit-Limit-PerUser", fmt.Sprintf("%.0f", cfg.PerUserQPS))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))

		c.Next()
	}
}

// ──────────────────────────────────────────
// 限流响应
// ──────────────────────────────────────────

// writeRateLimitResponse 返回标准 429 响应及限流头
func writeRateLimitResponse(c *gin.Context, limit float64, remaining int, now time.Time) {
	// 计算重置时间（下一个完整秒）
	resetTime := now.Add(time.Second).Unix()
	retryAfter := 1 // 建议客户端至少等待 1 秒后重试

	c.Header("X-RateLimit-Limit", fmt.Sprintf("%.0f", limit))
	c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))
	c.Header("X-RateLimit-Reset", strconv.FormatInt(resetTime, 10))
	c.Header("Retry-After", strconv.Itoa(retryAfter))

	c.JSON(http.StatusTooManyRequests, gin.H{
		"error": "请求过于频繁，请稍后重试",
		"code":  "RATE_LIMIT_EXCEEDED",
	})
	c.Abort()
}
