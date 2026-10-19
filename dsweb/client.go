// Package dsweb 是 DeepSeek 网页搜索 / 抓取接口(Search/Fetch)的 Go 客户端库。
//
// 接口文档: https://api-docs.deepseek.com/zh-cn/guides/search/
//
// 用法:
//
//	client := dsweb.NewClient(dsweb.WithAPIKey(os.Getenv("DEEPSEEK_API_KEY")))
//	resp, err := client.Search(ctx, &dsweb.SearchRequest{Query: "DeepSeek V4 发布", Count: 5})
//	if err != nil { ... }
//	for _, r := range resp.Results {
//		fmt.Println(r.Title, r.URL, r.Summary)
//	}
package dsweb

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// BaseURL DeepSeek API 基地址。
	BaseURL = "https://api.deepseek.com"
	// SearchEndpoint Search 接口路径。
	SearchEndpoint = "/v1/tools/search"
	// FetchEndpoint Fetch 接口路径。
	FetchEndpoint = "/v1/tools/fetch"

	// QueryMaxLen query 最大字符数(rune 计)。
	QueryMaxLen = 128
	// CountMax count 上限。
	CountMax = 50
	// URLMaxLen fetch 的 url 最大长度。
	URLMaxLen = 8192

	// SearchDefaultTimeout / FetchDefaultTimeout 单请求全程超时默认值
	// (Search/Fetch 各一份;NewClient 默认与 CLI 帮助文案共用)。
	SearchDefaultTimeout = 30 * time.Second
	FetchDefaultTimeout  = 60 * time.Second
	defaultTotalLimit    = 64 << 20 // 64MB 响应体上限
	retryBackoffBase     = 500 * time.Millisecond
	retryBackoffMax      = 4 * time.Second
	defaultUserAgent     = "dsweb-go/1.0"
)

// Client 调用 DeepSeek Search/Fetch 接口的客户端,零第三方依赖。
type Client struct {
	apiKey    string
	baseURL   string
	http      *http.Client
	retries   int
	userAgent string

	searchTimeout time.Duration
	fetchTimeout  time.Duration
}

// Option 客户端选项。
type Option func(*Client)

// WithAPIKey 设置 API Key(通常 sk- 开头)。
func WithAPIKey(k string) Option {
	return func(c *Client) {
		if k = strings.TrimSpace(k); k != "" {
			c.apiKey = k
		}
	}
}

// WithBaseURL 覆盖基地址,默认 https://api.deepseek.com。
func WithBaseURL(u string) Option {
	return func(c *Client) {
		if u = strings.TrimSpace(u); u != "" {
			c.baseURL = strings.TrimRight(u, "/")
		}
	}
}

// WithRetries 设置可重试错误(408/429/5xx/网络错误)的最大重试次数,默认 2,0 = 不重试。
func WithRetries(n int) Option {
	return func(c *Client) {
		if n < 0 {
			n = 0
		}
		c.retries = n
	}
}

// WithHTTPClient 替换底层 *http.Client(自定义代理、超时等)。
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) {
		if h != nil {
			c.http = h
		}
	}
}

// WithTimeout 覆盖单次请求的全程超时(search 与 fetch 均生效),默认 search 30s / fetch 60s。
func WithTimeout(d time.Duration) Option {
	return func(c *Client) {
		if d > 0 {
			c.searchTimeout, c.fetchTimeout = d, d
		}
	}
}

// NewClient 创建客户端。未配置 key 时调用会返回 ErrNoAPIKey。
func NewClient(opts ...Option) *Client {
	c := &Client{
		baseURL:       BaseURL,
		http:          &http.Client{},
		retries:       2,
		userAgent:     defaultUserAgent,
		searchTimeout: SearchDefaultTimeout,
		fetchTimeout:  FetchDefaultTimeout,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// KeyAvailable 返回客户端是否已配置 API Key。
func (c *Client) KeyAvailable() bool { return c.apiKey != "" }

// SearchRequest Search 接口请求字段。
type SearchRequest struct {
	// Query 搜索词,1~128 个 Unicode 字符,不能全为空白。
	// 支持 site: / -site: / -词 / -"短语" / "短语" / lang: / after: / before: 语法。
	Query string `json:"query"`
	// Count 返回条数 1~50,默认 10。
	Count int `json:"count,omitempty"`
	// OriQuestion 触发搜索的用户原始问题(1~128 字符),帮助服务端理解真实意图。
	OriQuestion string `json:"ori_question,omitempty"`
	// Localization 用于优先返回本地化搜索结果的 IP 地址。
	Localization string `json:"localization,omitempty"`
	// NeedContent 为 true 时在结果 content 字段返回网页正文,默认 false。
	NeedContent bool `json:"need_content,omitempty"`
}

func (r *SearchRequest) validate() error {
	q := strings.TrimSpace(r.Query)
	if q == "" {
		return &ParamError{Field: "query", Reason: "query is required and must not be blank"}
	}
	if n := utf8.RuneCountInString(q); n > QueryMaxLen {
		return &ParamError{Field: "query", Reason: fmt.Sprintf("exceeds %d characters (got %d)", QueryMaxLen, n)}
	}
	if r.Count != 0 && (r.Count < 1 || r.Count > CountMax) {
		return &ParamError{Field: "count", Reason: fmt.Sprintf("must be between 1 and %d (got %d)", CountMax, r.Count)}
	}
	if r.Count == 0 {
		r.Count = 10 // 与服务端默认一致,显式化便于 CLI 校验
	}
	if utf8.RuneCountInString(strings.TrimSpace(r.OriQuestion)) > QueryMaxLen {
		return &ParamError{Field: "ori_question", Reason: fmt.Sprintf("exceeds %d characters", QueryMaxLen)}
	}
	return nil
}

// SearchResult 单条搜索结果。
type SearchResult struct {
	RerankScore float64 `json:"rerank_score"`
	Title       string  `json:"title"`
	URL         string  `json:"url"`
	Snippet     string  `json:"snippet"`
	Summary     string  `json:"summary"`
	Content     string  `json:"content"`
	SiteName    *string `json:"site_name"`
	SiteIcon    *string `json:"site_icon"`
	Date        *string `json:"date"`
	Authority   int     `json:"authority"`
}

// SearchResponse Search 接口响应。
type SearchResponse struct {
	Results []SearchResult `json:"results"`
}

// FetchRequest Fetch 接口请求字段。
type FetchRequest struct {
	// URL 要抓取的网页地址: ≤8192 字符、HTTP/HTTPS、不能带用户名密码。
	URL string `json:"url"`
}

func (r *FetchRequest) validate() error {
	if r.URL == "" {
		return &ParamError{Field: "url", Reason: "url is required"}
	}
	if n := utf8.RuneCountInString(r.URL); n > URLMaxLen {
		return &ParamError{Field: "url", Reason: fmt.Sprintf("exceeds %d characters", URLMaxLen)}
	}
	return nil
}

// FetchPage Fetch 接口响应。
type FetchPage struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

type requestBody interface{ validate() error }

// Search 调用 Search 接口。
func (c *Client) Search(ctx context.Context, req *SearchRequest) (*SearchResponse, error) {
	return post[SearchResponse](ctx, c, SearchEndpoint, req, c.searchTimeout)
}

// Fetch 调用 Fetch 接口,返回抓取后的 Markdown 页面。
func (c *Client) Fetch(ctx context.Context, req *FetchRequest) (*FetchPage, error) {
	return post[FetchPage](ctx, c, FetchEndpoint, req, c.fetchTimeout)
}

// post 发送 JSON 请求并解码响应,带指数退避重试。
func post[T any](ctx context.Context, c *Client, endpoint string, body requestBody, maxTimeout time.Duration) (*T, error) {
	if err := body.validate(); err != nil {
		return nil, err
	}
	if !c.KeyAvailable() {
		return nil, ErrNoAPIKey
	}
	ctx, cancel := context.WithTimeout(ctx, maxTimeout)
	defer cancel()

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("dsweb: marshal request: %w", err)
	}
	url := c.baseURL + endpoint

	for attempt := 0; ; attempt++ {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
		if err != nil {
			return nil, fmt.Errorf("dsweb: build request: %w", err)
		}
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("User-Agent", c.userAgent)

		httpResp, err := c.http.Do(httpReq)
		if err != nil {
			if attempt < c.retries && backoffSleep(ctx, attempt, 0) {
				continue
			}
			return nil, &TransportError{Err: err}
		}

		if httpResp.StatusCode == http.StatusOK {
			out := new(T)
			if err := json.NewDecoder(io.LimitReader(httpResp.Body, defaultTotalLimit)).Decode(out); err != nil {
				httpResp.Body.Close()
				return nil, fmt.Errorf("dsweb: decode response: %w", err)
			}
			httpResp.Body.Close()
			return out, nil
		}

		apiErr := newAPIError(httpResp)
		httpResp.Body.Close()
		if apiErr.Retryable() && attempt < c.retries {
			if !backoffSleep(ctx, attempt, apiErr.retryAfter) {
				return nil, apiErr
			}
			continue
		}
		return nil, apiErr
	}
}

// backoffSleep 按 0.5s*2^n 指数退避(封顶 4s);Retry-After 存在时优先生效。
// 返回 false 表示 ctx 结束,调用方应立即返回。
func backoffSleep(ctx context.Context, attempt int, retryAfter time.Duration) bool {
	d := min(retryBackoffBase<<min(attempt, 3), retryBackoffMax)
	if retryAfter > d {
		d = retryAfter
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
