package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"costrict-router/internal/i18n"
)

const maxMetricsBodyBytes = 1024 * 1024

type chatRequestSummary struct {
	Model         string
	Stream        bool
	MessagesCount int
	ToolsCount    int
	MaxTokens     string
	Temperature   string
	TopP          string
}

type tokenUsage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	Known            bool
}

func (u tokenUsage) String() string {
	if !u.Known {
		return i18n.T("unknown", "未知")
	}
	return fmt.Sprintf(i18n.T("prompt=%d completion=%d total=%d", "输入=%d 输出=%d 总计=%d"), u.PromptTokens, u.CompletionTokens, u.TotalTokens)
}

type responseMetrics struct {
	Bytes          int64
	HeadersLatency time.Duration
	TTFB           time.Duration
	Duration       time.Duration
	Usage          tokenUsage
}

func (m responseMetrics) TokensPerSecond() string {
	if !m.Usage.Known || m.Usage.CompletionTokens <= 0 || m.TTFB <= 0 {
		return i18n.T("unknown", "未知")
	}
	generationDuration := m.Duration - m.TTFB
	if generationDuration <= 0 {
		return i18n.T("unknown", "未知")
	}
	return fmt.Sprintf("%.2f", float64(m.Usage.CompletionTokens)/generationDuration.Seconds())
}

type responseMetricsCollector struct {
	start       time.Time
	headersAt   time.Time
	firstByteAt time.Time
	bytes       int64
	isSSE       bool
	lineBuf     []byte
	bodyBuf     []byte
	usage       tokenUsage
}

func newResponseMetricsCollector(start, headersAt time.Time, format string) *responseMetricsCollector {
	return &responseMetricsCollector{
		start:     start,
		headersAt: headersAt,
		isSSE:     format == "sse",
	}
}

func (c *responseMetricsCollector) wrap(r io.Reader) io.Reader {
	return &metricsReader{inner: r, collector: c}
}

func (c *responseMetricsCollector) observe(chunk []byte) {
	if len(chunk) == 0 {
		return
	}
	if c.firstByteAt.IsZero() {
		c.firstByteAt = time.Now()
	}
	c.bytes += int64(len(chunk))
	if c.isSSE {
		c.observeSSE(chunk)
		return
	}
	if len(c.bodyBuf) < maxMetricsBodyBytes {
		remaining := maxMetricsBodyBytes - len(c.bodyBuf)
		if len(chunk) > remaining {
			chunk = chunk[:remaining]
		}
		c.bodyBuf = append(c.bodyBuf, chunk...)
	}
}

func (c *responseMetricsCollector) observeSSE(chunk []byte) {
	c.lineBuf = append(c.lineBuf, chunk...)
	for {
		idx := bytes.IndexByte(c.lineBuf, '\n')
		if idx < 0 {
			if len(c.lineBuf) > maxMetricsBodyBytes {
				c.lineBuf = c.lineBuf[len(c.lineBuf)-maxMetricsBodyBytes:]
			}
			return
		}
		line := strings.TrimSpace(string(c.lineBuf[:idx]))
		c.lineBuf = c.lineBuf[idx+1:]
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		if usage := usageFromJSON([]byte(payload)); usage.Known {
			c.usage = usage
		}
	}
}

func (c *responseMetricsCollector) finish() responseMetrics {
	if !c.isSSE {
		if usage := usageFromJSON(c.bodyBuf); usage.Known {
			c.usage = usage
		}
	}
	ttfb := time.Duration(0)
	if !c.firstByteAt.IsZero() {
		ttfb = c.firstByteAt.Sub(c.start)
	}
	return responseMetrics{
		Bytes:          c.bytes,
		HeadersLatency: c.headersAt.Sub(c.start),
		TTFB:           ttfb,
		Duration:       time.Since(c.start),
		Usage:          c.usage,
	}
}

type metricsReader struct {
	inner     io.Reader
	collector *responseMetricsCollector
}

func (r *metricsReader) Read(p []byte) (int, error) {
	n, err := r.inner.Read(p)
	if n > 0 {
		r.collector.observe(p[:n])
	}
	return n, err
}

func summarizeChatRequest(body []byte) chatRequestSummary {
	var raw struct {
		Model               string            `json:"model"`
		Stream              bool              `json:"stream"`
		Messages            []json.RawMessage `json:"messages"`
		Tools               []json.RawMessage `json:"tools"`
		MaxTokens           json.RawMessage   `json:"max_tokens"`
		MaxCompletionTokens json.RawMessage   `json:"max_completion_tokens"`
		Temperature         json.RawMessage   `json:"temperature"`
		TopP                json.RawMessage   `json:"top_p"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return chatRequestSummary{}
	}
	return chatRequestSummary{
		Model:         raw.Model,
		Stream:        raw.Stream,
		MessagesCount: len(raw.Messages),
		ToolsCount:    len(raw.Tools),
		MaxTokens:     rawJSONValue(firstRaw(raw.MaxTokens, raw.MaxCompletionTokens)),
		Temperature:   rawJSONValue(raw.Temperature),
		TopP:          rawJSONValue(raw.TopP),
	}
}

func firstRaw(values ...json.RawMessage) json.RawMessage {
	for _, value := range values {
		if len(value) > 0 {
			return value
		}
	}
	return nil
}

func rawJSONValue(value json.RawMessage) string {
	if len(value) == 0 || string(value) == "null" {
		return ""
	}
	var decoded any
	if err := json.Unmarshal(value, &decoded); err == nil {
		return fmt.Sprint(decoded)
	}
	return string(value)
}

func usageFromJSON(body []byte) tokenUsage {
	if len(body) == 0 {
		return tokenUsage{}
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil {
		return tokenUsage{}
	}
	if usageRaw, ok := root["usage"]; ok {
		return parseTokenUsage(usageRaw)
	}
	return tokenUsage{}
}

func parseTokenUsage(raw json.RawMessage) tokenUsage {
	var payload map[string]int
	if err := json.Unmarshal(raw, &payload); err != nil {
		return tokenUsage{}
	}
	prompt := firstInt(payload, "prompt_tokens", "input_tokens")
	completion := firstInt(payload, "completion_tokens", "output_tokens")
	total := firstInt(payload, "total_tokens")
	if total == 0 && (prompt > 0 || completion > 0) {
		total = prompt + completion
	}
	if prompt == 0 && completion == 0 && total == 0 {
		return tokenUsage{}
	}
	return tokenUsage{
		PromptTokens:     prompt,
		CompletionTokens: completion,
		TotalTokens:      total,
		Known:            true,
	}
}

func firstInt(payload map[string]int, keys ...string) int {
	for _, key := range keys {
		if value, ok := payload[key]; ok {
			return value
		}
	}
	return 0
}

func copyAndFlush(w io.Writer, r io.Reader) (int64, error) {
	buf := make([]byte, 32*1024)
	var written int64
	flusher, canFlush := w.(http.Flusher)
	for {
		nr, er := r.Read(buf)
		if nr > 0 {
			nw, ew := w.Write(buf[:nr])
			if nw > 0 {
				written += int64(nw)
			}
			if canFlush {
				flusher.Flush()
			}
			if ew != nil {
				return written, ew
			}
			if nr != nw {
				return written, io.ErrShortWrite
			}
		}
		if er != nil {
			if er == io.EOF {
				return written, nil
			}
			return written, er
		}
	}
}

func responseFormat(header http.Header) string {
	contentType := header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err == nil && mediaType == "text/event-stream" {
		return "sse"
	}
	return "json"
}

func valueOrUnknown(value string) string {
	if value == "" {
		return i18n.T("unknown", "未知")
	}
	return value
}

func valueOrNone(value string) string {
	if value == "" {
		return i18n.T("none", "无")
	}
	return value
}

func formatDuration(d time.Duration) string {
	if d <= 0 {
		return i18n.T("unknown", "未知")
	}
	return d.String()
}
