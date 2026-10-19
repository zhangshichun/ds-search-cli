// Command ds-search 是 DeepSeek 网页搜索 / 抓取(Search/Fetch)API 的命令行客户端,
// 面向 AI Agent 与自动化脚本设计:结构化输出、可判定退出码、无交互。
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"deepseek-websearch-cli/internal/cli"
)

// mainVersion 由构建脚本注入(-ldflags -X)。
var mainVersion = "dev"

// usageHeader 与子命令帮助共享退出码表(cli.ExitCodesDoc)与搜索语法(cli.SearchSyntaxDoc),
// 不再各自手抄。
const usageHeader = `ds-search — DeepSeek 网页搜索 / 抓取 CLI(Search+Fetch)

USAGE:
  ds-search search "查询词" [flags]     网页搜索
  ds-search fetch "URL" [flags]        抓取网页为 Markdown
  ds-search set-token=sk-xxx           快捷保存 API Key(= auth set)
  ds-search clear-token                快捷清除保存的 Key(= auth clear)
  ds-search auth set [--stdin|key]     保存 API Key(--stdin 防 history 泄露)
  ds-search auth show                  查看生效的 Key
  ds-search auth clear                 清除保存的 Key
  ds-search config path                打印配置文件路径
  ds-search version                    版本信息

CONFIG:
  API Key 解析优先级:  --api-key 参数 > 环境变量 DEEPSEEK_API_KEY > 配置文件(auth set)
  配置文件: 见 ` + "`ds-search config path`" + `(跨平台用户配置目录,0600)

` + cli.ExitCodesDoc + `

` + cli.SearchSyntaxDoc + `

TYPICAL (AI):
  ds-search search "DeepSeek V4 发布" --jsonl --fields title,url,summary
  ds-search search "Golang generics guide" --content --jsonl --fields title,url,content
  ds-search fetch "https://example.com/article" --raw
`

func main() {
	// 无参数或显式 help。
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stdout, usageHeader)
		os.Exit(0)
	}
	ctx := context.Background()
	stdout, stderr := os.Stdout, os.Stderr

	// search 支持隐式管道输入:tty 时传 nil,由 RunSearch 只用位置参数。
	var stdin io.Reader // search 用;auth 显式 --stdin 无条件拿 os.Stdin
	if cli.StdinIsPiped() {
		stdin = os.Stdin
	}

	var code int
	switch cmd := os.Args[1]; {
	case cmd == "search":
		code = cli.RunSearch(ctx, stdin, stdout, stderr, os.Args[2:])
	case cmd == "fetch":
		code = cli.RunFetch(ctx, stdout, stderr, os.Args[2:])
	case cmd == "auth":
		code = cli.RunAuth(os.Stdin, stdout, stderr, os.Args[2:])
	case cmd == "config":
		code = cli.RunConfig(stdout, stderr, os.Args[2:])
	case cmd == "set-token", cmd == "set-key":
		code = cli.RunSetToken(stdout, stderr, os.Args[2:])
	case strings.HasPrefix(cmd, "set-token="), strings.HasPrefix(cmd, "set-key="):
		// 等号一体写法:原样传给 RunSetToken,解析统一收口在那一层
		code = cli.RunSetToken(stdout, stderr, []string{cmd})
	case cmd == "clear-token", cmd == "clear-key":
		code = cli.RunClearToken(stdout, stderr)
	case cmd == "version", cmd == "--version", cmd == "-v":
		fmt.Printf("ds-search %s (%s)\n", mainVersion, runtime.Version())
		code = 0
	case cmd == "help", cmd == "--help", cmd == "-h":
		fmt.Fprint(stdout, usageHeader)
		code = 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n%s", cmd, usageHeader)
		code = cli.ExitUsage
	}
	os.Exit(code)
}
