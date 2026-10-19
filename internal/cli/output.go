package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"deepseek-websearch-cli/dsweb"
)

// 输出格式。
const (
	FormatText  = "text"  // 人读(默认)
	FormatJSON  = "json"  // 完整 JSON(单文档)
	FormatJSONL = "jsonl" // 每行一条 JSON — AI 解析最友好,流式安全
)

// Output 控制结果输出格式。
type Output struct {
	Format string   // text | json | jsonl
	Fields []string // --fields: 非空时过滤结果字段
}

// parseOutput 归一化输出选项:--json/--jsonl/--text 互斥,--fields 校验字段名。
func parseOutput(useJSON, useJSONL, useText bool, fields string) (*Output, error) {
	n := 0
	for _, b := range []bool{useJSON, useJSONL, useText} {
		if b {
			n++
		}
	}
	if n > 1 {
		return nil, errors.New("--json / --jsonl / --text are mutually exclusive")
	}
	o := &Output{Format: FormatText}
	switch {
	case useJSON:
		o.Format = FormatJSON
	case useJSONL:
		o.Format = FormatJSONL
	}
	if f := strings.TrimSpace(fields); f != "" {
		for _, s := range strings.Split(f, ",") {
			s = strings.ToLower(strings.TrimSpace(s))
			if _, known := resultFieldMap[s]; !known {
				return nil, fmt.Errorf("unknown field %q (valid: %s)", s, fieldWhitelist())
			}
			o.Fields = append(o.Fields, s)
		}
	}
	return o, nil
}

// resultFieldMap --fields 字段别名到 API 原生 JSON 键的映射。
var resultFieldMap = map[string]string{
	"rerank_score": "rerank_score",
	"score":        "rerank_score",
	"title":        "title",
	"url":          "url",
	"snippet":      "snippet",
	"summary":      "summary",
	"content":      "content",
	"site_name":    "site_name",
	"site":         "site_name",
	"site_icon":    "site_icon",
	"date":         "date",
	"authority":    "authority",
}

// searchFieldExtractors 原生 JSON 键 → 取值函数的全集注册表。
// filter/白名单清单/--fields 文档全部派生自它:新增 API 字段只改这一处。
var searchFieldExtractors = map[string]func(dsweb.SearchResult) any{
	"rerank_score": func(r dsweb.SearchResult) any { return r.RerankScore },
	"title":        func(r dsweb.SearchResult) any { return r.Title },
	"url":          func(r dsweb.SearchResult) any { return r.URL },
	"snippet":      func(r dsweb.SearchResult) any { return r.Snippet },
	"summary":      func(r dsweb.SearchResult) any { return r.Summary },
	"content":      func(r dsweb.SearchResult) any { return r.Content },
	"site_name":    func(r dsweb.SearchResult) any { return r.SiteName },
	"site_icon":    func(r dsweb.SearchResult) any { return r.SiteIcon },
	"date":         func(r dsweb.SearchResult) any { return r.Date },
	"authority":    func(r dsweb.SearchResult) any { return r.Authority },
}

// fetchFieldExtractors fetch 响应的字段注册表(同上,新增字段只改这里)。
var fetchFieldExtractors = map[string]func(dsweb.FetchPage) any{
	"title":   func(p dsweb.FetchPage) any { return p.Title },
	"content": func(p dsweb.FetchPage) any { return p.Content },
}

// fieldWhitelist 生成 --fields 合法字段清单(报错与帮助文本共用,不自手抄)。
func fieldWhitelist() string {
	names := make([]string, 0, len(resultFieldMap))
	for k := range resultFieldMap {
		names = append(names, k)
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

// WriteSearch 把 Search 响应写到 w。
func (o *Output) WriteSearch(w io.Writer, resp *dsweb.SearchResponse) error {
	if resp == nil {
		return nil
	}
	switch o.Format {
	case FormatJSON:
		return o.writeFilteredDocument(w, resp.Results)
	case FormatJSONL:
		for _, r := range resp.Results {
			line, err := json.Marshal(o.filter(r))
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintln(w, string(line)); err != nil {
				return err
			}
		}
		return nil
	default: // text
		for i, r := range resp.Results {
			meta := []string{}
			if r.SiteName != nil && *r.SiteName != "" {
				meta = append(meta, *r.SiteName)
			}
			if r.Date != nil && *r.Date != "" {
				meta = append(meta, *r.Date)
			}
			metaStr := strings.Join(meta, " | ")
			fmt.Fprintf(w, "%d. %s\n", i+1, strings.TrimSpace(r.Title))
			fmt.Fprintf(w, "   %s  [score %.1f, authority %d %s]\n", r.URL, r.RerankScore, r.Authority, metaStr)
			if s := strings.TrimSpace(r.Summary); s != "" {
				fmt.Fprintf(w, "   %s\n", firstLine(s, 240))
			} else if s := strings.TrimSpace(r.Snippet); s != "" {
				fmt.Fprintf(w, "   %s\n", firstLine(s, 240))
			}
			fmt.Fprintln(w)
		}
		return nil
	}
}

// WriteFetch 把 Fetch 页面写到 w。raw 为 true 时直接输出 Markdown 正文。
func (o *Output) WriteFetch(w io.Writer, page *dsweb.FetchPage, raw bool) error {
	if raw {
		_, err := io.WriteString(w, page.Content)
		return err
	}
	switch o.Format {
	case FormatJSON, FormatJSONL: // 单文档场景两者同形(单行 JSON)
		return json.NewEncoder(w).Encode(o.filterFetch(page))
	default:
		if t := strings.TrimSpace(page.Title); t != "" {
			fmt.Fprintln(w, t)
			fmt.Fprintln(w)
		}
		_, err := io.WriteString(w, page.Content)
		return err
	}
}

// filterFetch 对 fetch 单文档应用 --fields(由 fetchFieldExtractors 驱动)。
func (o *Output) filterFetch(p *dsweb.FetchPage) map[string]any {
	m := map[string]any{}
	if len(o.Fields) == 0 {
		for key, get := range fetchFieldExtractors {
			m[key] = get(*p)
		}
		return m
	}
	for _, f := range o.Fields {
		key := resultFieldMap[f] // parseOutput 已校验白名单并小写化
		if get, ok := fetchFieldExtractors[key]; ok {
			m[key] = get(*p)
		}
	}
	return m
}

// writeFilteredDocument --fields 应用于 json 单文档模式:结果仍包在 {"results": []} 内。
func (o *Output) writeFilteredDocument(w io.Writer, results []dsweb.SearchResult) error {
	if len(o.Fields) == 0 {
		return json.NewEncoder(w).Encode(map[string]any{"results": results})
	}
	out := make([]map[string]any, 0, len(results))
	for _, r := range results {
		out = append(out, o.filter(r))
	}
	return json.NewEncoder(w).Encode(map[string]any{"results": out})
}

// filter 结果项按 --fields 过滤(由 searchFieldExtractors 驱动);
// 未识别字段宽容忽略(与 API 的"未知字段会忽略"一致)。
func (o *Output) filter(r dsweb.SearchResult) map[string]any {
	m := map[string]any{}
	if len(o.Fields) == 0 {
		for key, get := range searchFieldExtractors {
			m[key] = get(r)
		}
		return m
	}
	for _, f := range o.Fields {
		key := resultFieldMap[f] // parseOutput 已校验白名单并小写化
		if get, ok := searchFieldExtractors[key]; ok {
			m[key] = get(r)
		}
	}
	return m
}

// firstLine 取字符串第一行并截断到 n 个 rune。
func firstLine(s string, n int) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
