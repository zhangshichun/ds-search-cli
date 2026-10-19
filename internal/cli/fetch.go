package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"deepseek-websearch-cli/dsweb"
)

// RunFetch `ds-search fetch "<url>" [--raw]` — 抓取单个网页为 Markdown。
func RunFetch(ctx context.Context, stdout, stderr io.Writer, args []string) int {
	fs := newFlagSet("fetch")
	var cf CommonFlags
	registerCommon(fs, &cf)

	raw := fs.Bool("raw", false, "直接输出 Markdown 正文(不包 title/content JSON)")
	var (
		useJSON  = fs.Bool("json", false, "输出 JSON {title, content}(默认)")
		useJSONL = fs.Bool("jsonl", false, "同 --json(fetch 单文档)")
		useText  = fs.Bool("text", false, "人读格式:title + 正文")
		fields   = fs.String("fields", "", "字段过滤:title,content")
	)
	if code, parsed := parseOrHelp(fs, stdout, stderr, args, fetchHelpText); !parsed {
		return code
	}
	if fs.NArg() < 1 {
		return writeUsageErr(stderr, fs, fmt.Errorf("missing url argument"))
	}
	url := strings.TrimSpace(fs.Arg(0))

	out, uerr := parseOutput(*useJSON, *useJSONL, *useText, *fields)
	if uerr != nil {
		return writeUsageErr(stderr, fs, uerr)
	}

	client, cerr := resolveClient(&cf)
	if cerr != nil {
		return writeErr(stderr, cerr)
	}
	page, err := client.Fetch(ctx, &dsweb.FetchRequest{URL: url})
	if err != nil {
		return writeErr(stderr, err)
	}
	if err := out.WriteFetch(stdout, page, *raw); err != nil {
		return writeErr(stderr, err)
	}
	fmt.Fprintf(stderr, "fetched %d chars, title: %s\n", len(page.Content), page.Title)
	return ExitOK
}

// fetchHelpText `fetch -h` 帮助文本(退出码/超时默认由共享来源拼接)。
var fetchHelpText = `usage: ds-search fetch "URL" [flags]

抓取网页,以 Markdown 返回正文(POST /v1/tools/fetch)。

Flags:
  --raw             直接输出 Markdown 正文(不套 JSON,适合管道/直接阅读)
  --json            JSON {"title","content"}(默认)
  --jsonl           同 --json(fetch 为单文档)
  --text            title + 人读正文
  --fields a,b      字段过滤: title,content
  --api-key string  API Key(优先级最高)
  --base-url string API 基地址(默认 https://api.deepseek.com)
  --timeout duration 请求超时(默认 ` + dsweb.FetchDefaultTimeout.String() + `),如 90s、2m
  --retries int     可重试错误重试次数(默认 2,0 禁用)

说明: HTTP 404 = 目标 URL 不可用/无正文(不计费); 403 = 安全策略拒绝;
计费按成功请求次数(每千次 5 元),失败与超时不计费。

` + ExitCodesDoc + `
`
