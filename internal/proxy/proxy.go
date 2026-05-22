package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"time"

	"costrict-router/internal/config"
	"costrict-router/internal/i18n"
	"costrict-router/internal/ids"
	"costrict-router/internal/logx"
)

const Version = "0.1.0"

type TokenProvider interface {
	Config() config.Config
	EnsureFreshToken(context.Context) error
}

type Handler struct {
	Tokens TokenProvider
	Client *http.Client
	Logger *logx.Logger
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/healthz":
		h.handleHealth(w)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/chat/completions":
		if !h.authorizeLocalAPIKey(w, r) {
			return
		}
		h.forward(w, r, "/chat-rag/api/v1/chat/completions")
	case r.Method == http.MethodGet && r.URL.Path == "/v1/models":
		if !h.authorizeLocalAPIKey(w, r) {
			return
		}
		h.forward(w, r, "/ai-gateway/api/v1/models")
	default:
		writeOpenAIError(w, http.StatusNotFound, "not_found", i18n.T("local route not found", "未找到本地路由"))
	}
}

func (h *Handler) handleHealth(w http.ResponseWriter) {
	cfg := h.Tokens.Config()
	payload := map[string]any{
		"ok":                       cfg.LoggedIn(),
		"base_url":                 cfg.BaseURL,
		"listen_addr":              cfg.ListenAddr,
		"machine_code":             config.Redact(cfg.MachineCode),
		"user_id":                  cfg.UserID,
		"access_token":             config.Redact(cfg.AccessToken),
		"refresh_token":            config.Redact(cfg.RefreshToken),
		"access_expires":           cfg.AccessTokenExpiresAt,
		"local_api_key_configured": cfg.LocalAPIKeyHash != "",
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func (h *Handler) authorizeLocalAPIKey(w http.ResponseWriter, r *http.Request) bool {
	cfg := h.Tokens.Config()
	if cfg.LocalAPIKeyHash == "" {
		writeOpenAIError(w, http.StatusInternalServerError, "configuration_error", i18n.T("local API key is not configured; restart costrict-router to generate one", "本地 API Key 未配置，请重启 costrict-router 生成"))
		return false
	}
	apiKey, ok := bearerToken(r.Header.Get("Authorization"))
	if !ok || apiKey == "" {
		writeOpenAIError(w, http.StatusUnauthorized, "authentication_error", i18n.T("missing local API key", "缺少本地 API Key"))
		return false
	}
	if !cfg.VerifyLocalAPIKey(apiKey) {
		writeOpenAIError(w, http.StatusUnauthorized, "authentication_error", i18n.T("invalid local API key", "本地 API Key 无效"))
		return false
	}
	return true
}

func (h *Handler) forward(w http.ResponseWriter, r *http.Request, upstreamPath string) {
	// 转发前确保 token 可用，再把 OpenAI 兼容路径映射到真实 CoStrict 上游接口。
	start := time.Now()
	if err := h.Tokens.EnsureFreshToken(r.Context()); err != nil {
		writeOpenAIError(w, http.StatusUnauthorized, "authentication_error", err.Error())
		return
	}

	cfg := h.Tokens.Config()
	if !cfg.LoggedIn() {
		writeOpenAIError(w, http.StatusUnauthorized, "authentication_error", i18n.T("not logged in; run costrict-router login first", "未登录，请先执行 costrict-router login"))
		return
	}

	upstreamURL, err := joinURL(cfg.BaseURL, upstreamPath)
	if err != nil {
		writeOpenAIError(w, http.StatusServiceUnavailable, "configuration_error", err.Error())
		return
	}
	if r.URL.RawQuery != "" && upstreamPath != "/ai-gateway/api/v1/models" {
		upstreamURL += "?" + r.URL.RawQuery
	}

	var body io.Reader = r.Body
	var bodyBytes []byte
	if h.Logger != nil && h.Logger.DebugEnabled() && r.Body != nil {
		// debug 模式才读取请求体用于日志，默认运行不记录用户转发内容。
		bodyBytes, _ = io.ReadAll(r.Body)
		body = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL, body)
	if err != nil {
		writeOpenAIError(w, http.StatusServiceUnavailable, "proxy_error", err.Error())
		return
	}
	copySelectedHeaders(req.Header, r.Header)
	applyCostrictHeaders(req.Header, cfg, r)

	requestID := req.Header.Get("X-Request-ID")
	if h.Logger != nil && h.Logger.DebugEnabled() {
		h.Logger.Debugf("forward request id=%s method=%s path=%s upstream=%s headers=%v body=%q",
			requestID, r.Method, r.URL.Path, upstreamURL, logx.RedactHeader(req.Header), logx.TruncateBody(bodyBytes, 32*1024))
	}

	resp, err := h.httpClient().Do(req)
	if err != nil {
		if h.Logger != nil {
			h.Logger.Warnf(i18n.T("upstream request failed method=%s path=%s request_id=%s err=%v", "上游请求失败 method=%s path=%s request_id=%s err=%v"), r.Method, r.URL.Path, requestID, err)
		}
		writeOpenAIError(w, http.StatusBadGateway, "upstream_error", err.Error())
		return
	}
	defer resp.Body.Close()

	copyResponseHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, copyErr := io.Copy(w, resp.Body)
	if h.Logger != nil {
		if h.Logger.DebugEnabled() {
			h.Logger.Debugf("forward response id=%s method=%s path=%s status=%d duration=%s",
				requestID, r.Method, r.URL.Path, resp.StatusCode, time.Since(start))
		} else if resp.StatusCode >= 400 {
			h.Logger.Warnf(i18n.T("upstream returned error method=%s path=%s status=%d request_id=%s duration=%s", "上游返回错误 method=%s path=%s status=%d request_id=%s duration=%s"),
				r.Method, r.URL.Path, resp.StatusCode, requestID, time.Since(start))
		}
	}
	if copyErr != nil && h.Logger != nil {
		h.Logger.Warnf(i18n.T("failed to copy upstream response request_id=%s err=%v", "复制上游响应失败 request_id=%s err=%v"), requestID, copyErr)
	}
}

func applyCostrictHeaders(h http.Header, cfg config.Config, incoming *http.Request) {
	// 补齐 CoStrict 上游依赖的认证、用户、请求追踪和客户端上下文头。
	requestID := ids.UUID()
	taskID := ids.UUID()
	h.Set("Authorization", "Bearer "+cfg.AccessToken)
	h.Set("Accept-Language", firstHeader(incoming, "Accept-Language", "zh-CN"))
	h.Set("HTTP-Referer", firstHeader(incoming, "HTTP-Referer", "https://github.com/RooVetGit/Roo-Cline"))
	h.Set("X-Title", firstHeader(incoming, "X-Title", "Roo Code"))
	h.Set("User-Agent", fmt.Sprintf("costrict-router/%s (%s/%s)", Version, runtime.GOOS, runtime.GOARCH))
	h.Set("X-Costrict-Version", cfg.PluginVersion)
	h.Set("x-quota-identity", firstHeader(incoming, "x-quota-identity", "system"))
	h.Set("X-Request-ID", requestID)
	h.Set("zgsm-request-id", requestID)
	h.Set("zgsm-task-id", taskID)
	h.Set("x-user-id", cfg.UserID)
	h.Set("zgsm-client-id", cfg.MachineCode)
	h.Set("zgsm-provider", "costrict")
	h.Set("x-caller", "chat")
	for _, key := range []string{"zgsm-project-path", "zgsm-prompt-tags", "agent-type"} {
		if value := incoming.Header.Get(key); value != "" {
			h.Set(key, value)
		}
	}
}

func copySelectedHeaders(dst, src http.Header) {
	for _, key := range []string{"Accept", "Content-Type", "Cache-Control"} {
		if value := src.Get(key); value != "" {
			dst.Set(key, value)
		}
	}
}

func copyResponseHeaders(dst, src http.Header) {
	for key, values := range src {
		if isHopByHop(key) {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func isHopByHop(key string) bool {
	switch strings.ToLower(key) {
	case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade":
		return true
	default:
		return false
	}
}

func joinURL(base, path string) (string, error) {
	u, err := url.Parse(strings.TrimRight(base, "/"))
	if err != nil {
		return "", err
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("base_url 无效: %s", base)
	}
	u.Path = path
	u.RawQuery = ""
	return u.String(), nil
}

func firstHeader(r *http.Request, key, fallback string) string {
	if value := r.Header.Get(key); value != "" {
		return value
	}
	return fallback
}

func bearerToken(value string) (string, bool) {
	scheme, token, ok := strings.Cut(strings.TrimSpace(value), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	token = strings.TrimSpace(token)
	return token, token != ""
}

func (h *Handler) httpClient() *http.Client {
	if h.Client != nil {
		return h.Client
	}
	return http.DefaultClient
}

func writeOpenAIError(w http.ResponseWriter, status int, typ, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"type":    typ,
			"message": message,
		},
	})
}
