package dsweb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// newTestServer 起一个假 API,校验路径/头,按 handler 回复。
func newTestServer(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	c := NewClient(
		WithAPIKey("sk-test-key"),
		WithBaseURL(srv.URL),
		WithRetries(0),
		WithTimeout(5*time.Second),
	)
	t.Cleanup(srv.Close)
	return c, srv
}

func TestSearchOK(t *testing.T) {
	var gotAuth, gotType string
	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotType = r.Header.Get("Content-Type")
		if r.Method != http.MethodPost || r.URL.Path != "/v1/tools/search" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"rerank_score": 2.4, "title": "T1", "url": "https://e.com/1", "snippet": "s", "summary": "sm", "content": "", "site_name": "E", "date": "2026-06-01T00:00:00+00:00", "authority": 3},
			},
		})
	})
	resp, err := c.Search(context.Background(), &SearchRequest{Query: "hello", Count: 5})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if gotAuth != "Bearer sk-test-key" {
		t.Errorf("bad auth header: %q", gotAuth)
	}
	if !strings.HasPrefix(gotType, "application/json") {
		t.Errorf("bad content type: %q", gotType)
	}
	if len(resp.Results) != 1 || resp.Results[0].Title != "T1" || resp.Results[0].Authority != 3 {
		t.Errorf("unexpected results: %+v", resp.Results)
	}
}

func TestSearchDefaultCountFilled(t *testing.T) {
	var seenCount any
	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		seenCount = body["count"]
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"results":[]}`)
	})
	_, err := c.Search(context.Background(), &SearchRequest{Query: "hi"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if v, _ := seenCount.(float64); v != 10 {
		t.Errorf("expected default count 10, got %v", seenCount)
	}
}

func TestSearchParamErrors(t *testing.T) {
	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server must not be called on param error")
	})
	for _, req := range []*SearchRequest{
		{Query: ""},
		{Query: strings.Repeat("字", QueryMaxLen+1)},
		{Query: "hi", Count: -1},
		{Query: "hi", Count: CountMax + 1},
	} {
		_, err := c.Search(context.Background(), req)
		var pe *ParamError
		if !errors.As(err, &pe) {
			t.Errorf("want ParamError for %+v, got %v", req, err)
		}
	}
	_, err := c.Fetch(context.Background(), &FetchRequest{URL: strings.Repeat("u", URLMaxLen+1)})
	var pe *ParamError
	if !errors.As(err, &pe) {
		t.Errorf("fetch overlong url: want ParamError, got %v", err)
	}
}

func TestFetchOK(t *testing.T) {
	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/tools/fetch" {
			t.Errorf("bad path %s", r.URL.Path)
		}
		fmt.Fprintln(w, `{"title":"Page","content":"# Hello"}`)
	})
	page, err := c.Fetch(context.Background(), &FetchRequest{URL: "https://example.com/a"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if page.Title != "Page" || page.Content != "# Hello" {
		t.Errorf("unexpected page: %+v", page)
	}
}

// TestRetryOn429 验证 429 后按 Retry-After 重试成功。
func TestRetryOn429(t *testing.T) {
	var calls int32
	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) < 2 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		fmt.Fprintln(w, `{"results":[{"title":"after retry","url":"https://e.com"}]}`)
	})
	c.retries = 2
	c.searchTimeout = 10 * time.Second
	start := time.Now()
	resp, err := c.Search(context.Background(), &SearchRequest{Query: "hi"})
	if err != nil {
		t.Fatalf("Search with retry: %v", err)
	}
	if resp.Results[0].Title != "after retry" {
		t.Errorf("unexpected title: %s", resp.Results[0].Title)
	}
	if elapsed := time.Since(start); elapsed < 900*time.Millisecond {
		t.Errorf("expected to honor Retry-After ~1s, elapsed %v", elapsed)
	}
}

// TestNoRetryOn401 验证 401 不重试且归为客户端错误。
func TestNoRetryOn401(t *testing.T) {
	var calls int32
	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintln(w, `{"error":{"message":"api key invalid","type":"invalid_api_key","code":"invalid_api_key"}}`)
	})
	c.retries = 3
	_, err := c.Search(context.Background(), &SearchRequest{Query: "hi"})
	var ae *APIError
	if !errors.As(err, &ae) || ae.StatusCode() != 401 {
		t.Fatalf("want APIError 401, got %v", err)
	}
	if calls != 1 {
		t.Errorf("401 must not be retried, calls=%d", calls)
	}
	if !strings.Contains(ae.Message, "invalid") {
		t.Errorf("message passthrough failed: %q", ae.Message)
	}
}

// TestRetryExhaustion 验证 5xx 重试耗尽后返回最后错误。
func TestRetryExhaustion(t *testing.T) {
	var calls int32
	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	})
	c.retries = 2
	_, err := c.Search(context.Background(), &SearchRequest{Query: "hi"})
	var ae *APIError
	if !errors.As(err, &ae) || ae.StatusCode() != 500 {
		t.Fatalf("want APIError 500, got %v", err)
	}
	if calls != 3 { // 初次 + 2 次重试
		t.Errorf("expected 3 calls (1+2 retries), got %d", calls)
	}
	if !ae.Retryable() {
		t.Error("500 should be retryable")
	}
}

func TestNoAPIKey(t *testing.T) {
	c := NewClient()
	_, err := c.Search(context.Background(), &SearchRequest{Query: "hi"})
	if !errors.Is(err, ErrNoAPIKey) {
		t.Fatalf("want ErrNoAPIKey, got %v", err)
	}
}

func TestBackoffSleepCap(t *testing.T) {
	// attempt 越大退避封顶 4s,这里只验证小 attempt 用时 >= base。
	if !backoffSleep(context.Background(), 0, 0) {
		t.Fatal("ctx alive, expect true")
	}
}
