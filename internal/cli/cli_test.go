package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestReorder(t *testing.T) {
	names := map[string]bool{"json": true, "jsonl": true, "text": true, "content": true, "raw": true, "count": true, "fields": true, "api-key": true, "base-url": true, "query": true, "timeout": true}
	bools := map[string]bool{"json": true, "jsonl": true, "text": true, "content": true, "raw": true}
	cases := []struct {
		name string
		in   []string
		want string // 空格连接
	}{
		{"plain", []string{"hi"}, "hi"},
		{"flag after positional", []string{"hi", "--jsonl", "--count", "5"}, "--jsonl --count 5 hi"},
		{"mixed", []string{"hi", "--jsonl", "world", "--count", "3"}, "--jsonl --count 3 hi world"},
		{"equals form", []string{"hi", "--count=5"}, "--count=5 hi"},
		{"bool cluster", []string{"hi", "--jsonl", "--content"}, "--jsonl --content hi"},
		{"search syntax positional", []string{"-site:github.com", "golang", "--json"}, "--json -site:github.com golang"},
		{"dash terminator", []string{"--", "--count", "x"}, "--count x"},
		{"missing value", []string{"--count"}, "--count"},
		{"negated syntax", []string{`DeepSeek -"release notes"`, "--text"}, `--text DeepSeek -"release notes"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := strings.Join(reorder(names, bools, c.in), " ")
			if got != c.want {
				t.Errorf("got  %s\nwant %s", got, c.want)
			}
		})
	}
}

// newTestEnv 起假 API 并把用户配置目录隔离到临时 HOME/USERPROFILE,避免污染真实配置。
func newTestEnv(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	dir := t.TempDir()
	if v := os.Getenv("USERPROFILE"); v != "" {
		t.Setenv("USERPROFILE", dir)
	}
	if v := os.Getenv("HOME"); v != "" {
		t.Setenv("HOME", dir)
	}
	if v := os.Getenv("APPDATA"); v != "" {
		t.Setenv("APPDATA", dir)
	}
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		t.Setenv("XDG_CONFIG_HOME", dir)
	}
	os.Unsetenv(APIKeyEnv) // 保证 key 不会从环境链路漏入
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestRunSearchJSONL(t *testing.T) {
	var stdout, stderr bytes.Buffer
	baseURL := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"results": []map[string]any{
			{"rerank_score": 2.5, "title": "T1", "url": "https://e.com/1", "snippet": "s1", "summary": "sm1", "authority": 3},
			{"rerank_score": 1.0, "title": "T2", "url": "https://e.com/2"},
		}})
	})
	code := RunSearch(t.Context(), nil, &stdout, &stderr, []string{"--jsonl", "--fields", "title,url,score", "--api-key", "sk-x", "--base-url", baseURL, "hello"})
	if code != ExitOK {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 jsonl lines, got %d: %q", len(lines), stdout.String())
	}
	var first map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("parse jsonl: %v", err)
	}
	if first["title"] != "T1" || first["url"] != "https://e.com/1" || first["rerank_score"] != 2.5 {
		t.Errorf("unexpected fields: %v", first)
	}
	if _, has := first["snippet"]; has {
		t.Errorf("snippet should be filtered out by --fields")
	}
}

func TestRunSearchText(t *testing.T) {
	var stdout, stderr bytes.Buffer
	baseURL := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"results": []map[string]any{
			{"rerank_score": 2.0, "title": "标题一", "url": "https://e.com/1", "summary": "摘要文本", "authority": 3, "site_name": "新闻站"},
		}})
	})
	code := RunSearch(t.Context(), nil, &stdout, &stderr, []string{"--api-key", "sk-x", "--base-url", baseURL, "测试"})
	if code != ExitOK {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "标题一") || !strings.Contains(stdout.String(), "https://e.com/1") {
		t.Errorf("text output unexpected:\n%s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "1 result(s)") {
		t.Errorf("stderr hint missing: %s", stderr.String())
	}
}

// TestRunSearchStdinQuery 隐式管道输入:stdin 非 nil 且无位置参数时从中读 query。
func TestRunSearchStdinQuery(t *testing.T) {
	var stdout, stderr bytes.Buffer
	baseURL := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"results": []map[string]any{}})
	})
	code := RunSearch(t.Context(), strings.NewReader("  通过管道来的查询词  \n"), &stdout, &stderr, []string{"--api-key", "sk-x", "--base-url", baseURL})
	if code != ExitOK {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
}

func TestRunSearchNoKey(t *testing.T) {
	var stdout, stderr bytes.Buffer
	baseURL := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) { t.Fatal("must not call") })
	code := RunSearch(t.Context(), nil, &stdout, &stderr, []string{"--base-url", baseURL, "hi"})
	if code != ExitAuth {
		t.Fatalf("want ExitAuth, got %d", code)
	}
}

func TestRunFetchRaw(t *testing.T) {
	var stdout, stderr bytes.Buffer
	baseURL := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"title": "Doc", "content": "# md body"})
	})
	code := RunFetch(t.Context(), &stdout, &stderr, []string{"--raw", "--api-key", "sk-x", "--base-url", baseURL, "https://e.com/a"})
	if code != ExitOK {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	if strings.TrimSpace(stdout.String()) != "# md body" {
		t.Errorf("raw output: %q", stdout.String())
	}
}

func TestAuthSetShowClear(t *testing.T) {
	var stdout, stderr bytes.Buffer
	newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {})

	// set --stdin(stdin 经参数注入,不再全局替换 os.Stdin)
	code := RunAuth(strings.NewReader("sk-abcdef1234567890xyz\n"), &stdout, &stderr, []string{"set", "--stdin"})
	if code != ExitOK {
		t.Fatalf("auth set exit=%d stderr=%s", code, stderr.String())
	}

	// show
	stdout.Reset()
	code = RunAuth(nil, &stdout, &stderr, []string{"show"})
	if code != ExitOK {
		t.Fatalf("auth show exit=%d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "source=config") || !strings.Contains(stdout.String(), "sk-abcde") {
		t.Errorf("show output: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), "sk-abcdef1234567890xyz") {
		t.Errorf("full key leaked in show output: %s", stdout.String())
	}

	// clear
	stdout.Reset()
	code = RunAuth(nil, &stdout, &stderr, []string{"clear"})
	if code != ExitOK {
		t.Fatalf("auth clear exit=%d", code)
	}
	cfg, err := LoadConfig()
	if err != nil || cfg.APIKey != "" {
		t.Fatalf("config after clear: %+v err=%v", cfg, err)
	}
}

// TestSetToken 验证顶层快捷命令 set-token 的两种写法。
func TestSetToken(t *testing.T) {
	var stdout, stderr bytes.Buffer
	newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {})

	// 空格写法
	code := RunSetToken(&stdout, &stderr, []string{"sk-space00011122233344x"})
	if code != ExitOK {
		t.Fatalf("set-token space form exit=%d stderr=%s", code, stderr.String())
	}
	cfg, err := LoadConfig()
	if err != nil || cfg.APIKey != "sk-space00011122233344x" {
		t.Fatalf("config after set-token: %+v err=%v", cfg, err)
	}

	// 等号一体写法(main 原样传整段参数,解析收口在 RunSetToken)
	stdout.Reset()
	code = RunSetToken(&stdout, &stderr, []string{"set-token=sk-eqform000111222333xx"})
	if code != ExitOK {
		t.Fatalf("set-token eq-tail form exit=%d stderr=%s", code, stderr.String())
	}
	if cfg, _ := LoadConfig(); cfg.APIKey != "sk-eqform000111222333xx" {
		t.Fatalf("eq-tail form not saved: %+v", cfg)
	}

	// 等号空值 → usage,退出码 4
	stdout.Reset()
	stderr.Reset()
	code = RunSetToken(&stdout, &stderr, []string{"set-token="})
	if code != ExitUsage || !strings.Contains(stderr.String(), "set-token") {
		t.Fatalf("empty eq form: exit=%d stderr=%s", code, stderr.String())
	}

	// 无参 → usage,退出码 4
	stdout.Reset()
	stderr.Reset()
	code = RunSetToken(&stdout, &stderr, nil)
	if code != ExitUsage || !strings.Contains(stderr.String(), "set-token") {
		t.Fatalf("empty args: exit=%d stderr=%s", code, stderr.String())
	}
}

// TestSaveKeyStripsBOM 管道输入带 UTF-8 BOM 时(PowerShell echo | exe 常态)必须剥离,
// 否则 BOM 混入 key 导致 401。
func TestSaveKeyStripsBOM(t *testing.T) {
	var stdout, stderr bytes.Buffer
	newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {})

	code := RunSetToken(&stdout, &stderr, []string{string(rune(0xFEFF)) + "sk-bom000111222333444x"})
	if code != ExitOK {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIKey != "sk-bom000111222333444x" {
		t.Errorf("BOM not stripped: %q", cfg.APIKey)
	}
}

// TestRunSearchKeyFromConfig 回归:配置文件中的 key 必须能被 search 命令用上
// (flag/env 均空时,ResolveKey 链路要生效于实际请求)。
func TestRunSearchKeyFromConfig(t *testing.T) {
	var stdout, stderr bytes.Buffer
	var authSeen string
	baseURL := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		authSeen = r.Header.Get("Authorization")
		json.NewEncoder(w).Encode(map[string]any{"results": []map[string]any{}})
	})
	// 只写配置文件,不传 --api-key、不设 env。
	if err := SaveConfig(&Config{APIKey: "sk-fromconfig000111222"}); err != nil {
		t.Fatal(err)
	}
	code := RunSearch(t.Context(), nil, &stdout, &stderr, []string{"--base-url", baseURL, "hi"})
	if code != ExitOK {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	if authSeen != "Bearer sk-fromconfig000111222" {
		t.Fatalf("config-file key not used, auth=%q", authSeen)
	}
}
