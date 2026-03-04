package handlers

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

type DocumentHandler struct {
	proxy  *httputil.ReverseProxy
	logger *logrus.Logger
}

func NewDocumentHandler(DocumentServiceURL string) *DocumentHandler {
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)

	// 解析目标服务URL
	targetURL, err := url.Parse(DocumentServiceURL)
	if err != nil {
		logger.WithError(err).Fatal("Failed to parse document service URL")
	}

	// 创建反向代理
	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	// 自定义Director（用于修改转发的请求）
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)

		// 重写路径：去掉 /api/v1 前缀
		// 例如：/api/v1/documents   -> /documents
		//      /api/v1/documents/   -> /documents
		//      /api/v1/documents/123 -> /documents/123
		if len(req.URL.Path) > 7 && req.URL.Path[:7] == "/api/v1" {
			req.URL.Path = req.URL.Path[7:] // 去掉 "/api/v1"
			if req.URL.Path == "" {
				req.URL.Path = "/"
			}
		}

		// 去掉尾部斜杠（主动预防后端触发 301 "trailing-slash" 重定向）
		// 例如：/documents/ -> /documents
		// 根路径 "/" 保持不变
		if req.URL.Path != "/" && strings.HasSuffix(req.URL.Path, "/") {
			req.URL.Path = strings.TrimRight(req.URL.Path, "/")
		}

		logger.WithFields(logrus.Fields{
			"method":      req.Method,
			"path":        req.URL.Path,
			"target_host": req.URL.Host,
		}).Info("Proxying request to document service")
	}

	// 兜底：拦截后端返回的重定向响应，将 Location 相对路径补全 /api/v1 前缀，
	// 防止浏览器跟随后绕过网关路由（如 /documents → /api/v1/documents）。
	proxy.ModifyResponse = func(resp *http.Response) error {
		resp.Header.Del("Access-Control-Allow-Origin")
		resp.Header.Del("Access-Control-Allow-Credentials")
		resp.Header.Del("Access-Control-Allow-Methods")
		resp.Header.Del("Access-Control-Allow-Headers")
		resp.Header.Del("Access-Control-Max-Age")
		switch resp.StatusCode {
		case http.StatusMovedPermanently, // 301
			http.StatusFound,             // 302
			http.StatusSeeOther,          // 303
			http.StatusTemporaryRedirect, // 307
			http.StatusPermanentRedirect: // 308
			location := resp.Header.Get("Location")
			// 仅处理相对路径（以 / 开头），且尚未含 /api/v1 前缀
			if strings.HasPrefix(location, "/") && !strings.HasPrefix(location, "/api/v1") {
				rewritten := "/api/v1" + location
				resp.Header.Set("Location", rewritten)
				logger.WithFields(logrus.Fields{
					"original":  location,
					"rewritten": rewritten,
				}).Debug("Rewriting redirect Location header")
			}
		}
		return nil
	}

	// 自定义错误处理
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		logger.WithError(err).Error("Proxy error")
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("Document service unavailable"))
	}

	return &DocumentHandler{
		proxy:  proxy,
		logger: logger,
	}
}

// ProxyRequest 通用的代理方法
// 这个方法会自动处理所有HTTP方法（GET、POST、PUT、DELETE等）
func (h *DocumentHandler) ProxyRequest(c *gin.Context) {
	// 将认证中间件解析出的用户信息注入请求 Header，供下游文档服务使用
	if userID, exists := c.Get("user_id"); exists {
		c.Request.Header.Set("X-User-ID", userID.(string))
	}
	if username, exists := c.Get("username"); exists {
		c.Request.Header.Set("X-User-Name", username.(string))
	}

	h.proxy.ServeHTTP(c.Writer, c.Request)
}
