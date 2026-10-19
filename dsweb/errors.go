package dsweb

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ErrNoAPIKey 未配置 API Key 时返回。
var ErrNoAPIKey = errors.New("dsweb: API key not configured " +
	"(set DEEPSEEK_API_KEY, use --api-key, or run `ds-search auth set`)")

// ParamError 本地参数校验失败(不发请求)。
type ParamError struct {
	Field  string
	Reason string
}

func (e *ParamError) Error() string {
	return fmt.Sprintf("dsweb: invalid parameter %q: %s", e.Field, e.Reason)
}

// TransportError 网络层错误(DNS、连接、TLS、超时等),属于可重试错误。
type TransportError struct {
	Err error
}

func (e *TransportError) Error() string {
	return fmt.Sprintf("dsweb: transport error: %v", e.Err)
}

func (e *TransportError) Unwrap() error { return e.Err }

// APIError 服务端返回的错误(OpenAI 兼容格式)。
type APIError struct {
	// Status HTTP 状态码。
	Status int
	// Message 错误说明。
	Message string
	// Type 错误大类,如 invalid_request_error / invalid_api_key / insufficient_balance。
	// (不经 JSON 反序列化填充,故无 tag;envelope 才是解码目标。)
	Type string
	// Code 具体错误码(DeepSeek 中与 type 相同)。
	Code string

	retryAfter time.Duration
}

func (e *APIError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = "(no message)"
	}
	typ := e.Type
	if typ == "" {
		typ = e.Code
	}
	s := fmt.Sprintf("dsweb: HTTP %d: %s", e.Status, msg)
	if typ != "" {
		s += " [" + typ + "]"
	}
	switch e.Status {
	case 402:
		s += " — account balance insufficient, top up at https://platform.deepseek.com"
	case 429:
		s += " — QPS limit exceeded (Search/Fetch each allow 5 QPS per account), wait and retry"
	}
	return s
}

// Status 返回 HTTP 状态码,实现带状态码的错误接口。
func (e *APIError) StatusCode() int { return e.Status }

// Retryable 判断是否值得重试:408/429/5xx 按官方文档可重试,其余 4xx 不重试。
func (e *APIError) Retryable() bool {
	switch {
	case e.Status == http.StatusRequestTimeout, // 408
		e.Status == http.StatusTooManyRequests, // 429
		e.Status >= 500:
		return true
	default:
		return false
	}
}

// apiErrorEnvelope OpenAI 兼容错误响应体。
type apiErrorEnvelope struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

func newAPIError(resp *http.Response) *APIError {
	e := &APIError{Status: resp.StatusCode}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err == nil && len(body) > 0 {
		var env apiErrorEnvelope
		if jsonErr := json.Unmarshal(body, &env); jsonErr == nil {
			e.Message = strings.TrimSpace(env.Error.Message)
			e.Type = env.Error.Type
			e.Code = env.Error.Code
		}
		if e.Message == "" {
			e.Message = strings.TrimSpace(string(body))
		}
	}
	if ra := resp.Header.Get("Retry-After"); ra != "" {
		if secs, err := atoiSafe(ra); err == nil {
			e.retryAfter = time.Duration(secs) * time.Second
		} else if t, err := http.ParseTime(ra); err == nil {
			if d := time.Until(t); d > 0 {
				e.retryAfter = d
			}
		}
	}
	return e
}

// atoiSafe 解析 Retry-After 中的整数秒;非正数或超大值视为无效(防 duration 溢出)。
func atoiSafe(s string) (int, error) {
	secs, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || secs <= 0 || secs > 1e7 {
		return 0, fmt.Errorf("invalid seconds")
	}
	return secs, nil
}
