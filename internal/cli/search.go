package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"deepseek-websearch-cli/dsweb"
)

// RunSearch `ds-search search "query" [flags]` — 搜索网页。
// stdin 非 nil 且 query 为空时从 stdin 读 query(管道输入)。
// 返回进程退出码。
func RunSearch(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, args []string) int {
	fs := newFlagSet("search")
	var cf CommonFlags
	registerCommon(fs, &cf)

	explicitQuery := fs.String("query", "", "显式传递 query(query 以 \"-\" 开头时用此参数)")
	count := fs.Int("count", 10, "返回条数 1~50")
	needContent := fs.Bool("content", false, "返回网页正文(增大流量与耗时,仅在需要正文时使用)")
	oriQuestion := fs.String("ori-question", "", "触发搜索的用户原始问题(1~128 字符,可选,优化意图理解)")
	localization := fs.String("localization", "", "本地化结果期望的出口 IP 地址(可选)")

	var (
		useJSON  = fs.Bool("json", false, "输出完整 JSON(单文档)")
		useJSONL = fs.Bool("jsonl", false, "每条结果输出一行 JSON(推荐给 AI 解析)")
		useText  = fs.Bool("text", false, "输出人读文本(默认)")
		fields   = fs.String("fields", "", "逗号分隔字段过滤;valid: "+fieldWhitelist())
	)

	if code, parsed := parseOrHelp(fs, stdout, stderr, args, searchHelpText); !parsed {
		return code
	}
	query := strings.TrimSpace(*explicitQuery)
	if query == "" {
		query = strings.TrimSpace(strings.Join(fs.Args(), " "))
	}
	if query == "" && stdin != nil {
		data, _ := io.ReadAll(stdin)
		query = strings.TrimSpace(string(data))
	}
	if query == "" {
		return writeUsageErr(stderr, fs, errors.New("missing query: pass it as an argument, --query, or via stdin pipe"))
	}

	out, uerr := parseOutput(*useJSON, *useJSONL, *useText, *fields)
	if uerr != nil {
		return writeUsageErr(stderr, fs, uerr)
	}

	req := &dsweb.SearchRequest{
		Query:        query,
		Count:        *count,
		OriQuestion:  strings.TrimSpace(*oriQuestion),
		Localization: strings.TrimSpace(*localization),
		NeedContent:  *needContent,
	}

	client, cerr := resolveClient(&cf)
	if cerr != nil {
		return writeErr(stderr, cerr)
	}
	resp, err := client.Search(ctx, req)
	if err != nil {
		return writeErr(stderr, err)
	}
	if err := out.WriteSearch(stdout, resp); err != nil {
		return writeErr(stderr, err)
	}
	// 简明提示到 stderr,不污染 stdout 解析。
	fmt.Fprintf(stderr, "%d result(s)\n", len(resp.Results))
	return ExitOK
}

// StdinIsPiped 判断进程 stdin 是否为管道输入(非交互终端)。
// 由 main 在分发子命令前判定,把 os.Stdin 或 nil 传给对应的 Run*。
func StdinIsPiped() bool {
	stat, err := os.Stdin.Stat()
	return err == nil && (stat.Mode()&os.ModeCharDevice) == 0
}

// writeErr 统一错误出口(单行,stderr),按类型归类退出码。
func writeErr(stderr io.Writer, err error) int {
	fmt.Fprintf(stderr, "error: %v\n", err)
	return classify(err)
}

// writeUsageErr 参数/flag 错误:提示 + 子命令 usage,退出码 4。
func writeUsageErr(stderr io.Writer, fs *flagSet, err error) int {
	fmt.Fprintf(stderr, "error: %v\n", err)
	fmt.Fprintf(stderr, "Run `ds-search %s -h` for usage.\n", fs.Name())
	return ExitUsage
}

// searchHelpText `search -h` 帮助文本。完整列出 flag 与搜索语法,
// AI 只需读一次即可正确构造调用。
// 字段清单、超时默认、语法表、退出码全部由共享来源拼接(不做手抄副本)。
var searchHelpText = `usage: ds-search search ["query" | --query "..." | stdin] [flags]

搜索网页(DeepSeek 自研搜索接口 POST /v1/tools/search)。
query 可作位置参数、--query 显式传入,或在非终端时经 stdin 管道读取。

Flags:
  --query string          显式 query(query 以 "-" 开头时必须用此参数)
  --count int             返回条数 1~50(默认 10)
  --content               需要网页正文(content 字段),默认只给摘要
  --ori-question string   触发搜索的原始问题(1~128 字符,优化意图理解)
  --localization string   期望本地化的出口 IP
  --json                  完整 JSON 单文档 {"results":[...]}
  --jsonl                 每条结果一行 JSON(推荐 AI 解析)
  --text                  人读文本(默认)
  --fields a,b,c          字段过滤(作用同上三种格式);valid: ` + fieldWhitelist() + `
  --api-key string        API Key(优先级最高)
  --base-url string       API 基地址(默认 https://api.deepseek.com)
  --timeout duration      请求超时(默认 ` + dsweb.SearchDefaultTimeout.String() + `),如 45s、2m
  --retries int           可重试错误重试次数(默认 2,0 禁用)

` + SearchSyntaxDoc + `

result 字段:
  rerank_score (0~3, >=1.5 优质) | title | url | snippet (100~200字短摘要)
  summary (600~800字长摘要, 适合模型阅读) | content (正文, 需 --content)
  site_name | site_icon | date (UTC RFC3339, 可为 null) | authority (0~4, -1未知, >=3 极权威)

stdin 用法: echo "查询词" | ds-search search --jsonl

` + ExitCodesDoc + `
`
