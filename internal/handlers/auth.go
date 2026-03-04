package handlers

import (
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

type AuthHandler struct {
	proxy  *httputil.ReverseProxy
	logger *logrus.Logger
}

func NewAuthHandler(AuthServiceURL string) *AuthHandler {
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)

	// 解析目标服务URL
	targetURL, err := url.Parse(AuthServiceURL)
	if err != nil {
		logger.WithError(err).Fatal("Failed to parse auth service URL")
	}

	// 创建反向代理
	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	// 自定义Director（用于修改转发的请求）
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)

		// 重写路径：去掉 /api/v1 前缀
		// 例如：/api/v1/login -> /login
		//      /api/v1/register -> /register
		if len(req.URL.Path) > 7 && req.URL.Path[:7] == "/api/v1" {
			req.URL.Path = req.URL.Path[7:] // 去掉 "/api/v1"
			if req.URL.Path == "" {
				req.URL.Path = "/"
			}
		}

		// 可以在这里添加自定义Headers
		// req.Header.Set("X-Forwarded-By", "API-Gateway")
		logger.WithFields(logrus.Fields{
			"method":      req.Method,
			"path":        req.URL.Path,
			"target_host": req.Host,
		}).Info("Proxying request to auth service")
	}

	// 自定义错误处理（可选）
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		logger.WithError(err).Error("Proxy error")
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("Auth service unavailable"))
	}

	return &AuthHandler{
		proxy:  proxy,
		logger: logger,
	}
}

// ProxyRequest 通用的代理方法
// 这个方法会自动处理所有HTTP方法（GET、POST、PUT、DELETE等）
func (h *AuthHandler) ProxyRequest(c *gin.Context) {
	// 直接使用proxy的ServeHTTP方法
	// 它会自动：
	// 1. 读取请求（包括method、headers、body）
	// 2. 转发到目标服务
	// 3. 返回响应
	h.proxy.ServeHTTP(c.Writer, c.Request)
}
