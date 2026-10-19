package cli

import (
	"errors"
	"fmt"
	"time"

	"deepseek-websearch-cli/dsweb"
)

// 进程退出码(README 中有完整列表,AI 依赖它判断失败类型)。
const (
	ExitOK      = 0 // 成功(含搜索 0 结果)
	ExitAPI     = 1 // 服务端错误(HTTP 4xx/5xx 中不可重试或已达重试上限)
	ExitAuth    = 2 // 认证/配置问题:API key 缺失、401
	ExitNetwork = 3 // 网络错误:DNS / 连接 / 超时
	ExitUsage   = 4 // 本地参数错误(缺参数、count 越界、未知 flag 等)
)

// ExitCodesDoc 退出码说明的唯一来源:
// search/fetch 帮助文本与 main 的命令总览都拼接引用,不再各处手抄。
const ExitCodesDoc = "Exit codes: 0 成功(含搜索 0 结果) | 1 服务端错误 | 2 API key 缺失/无效 | 3 网络错误 | 4 参数错误"

// SearchSyntaxDoc 搜索语法说明的唯一来源(search 帮助与 main 总览共用)。
const SearchSyntaxDoc = `Search syntax (query 原样透传):
  site:github.com            限制站点;多个用空格 + OR: site:a.com OR site:b.com
  -site:example.com          排除站点(多个空格分隔)
  -word / -"exclude phrase"  排除关键词/短语(多个空格分隔)
  "exact phrase"             精确匹配短语(多个空格分隔)
  lang:zh,en                 限定语言(逗号分隔)
  after:2025-01-01           发布日期之后(年份/年月也可: after:2025)
  before:2026-06-01          发布日期之前(年份/年月也可: before:2026)
  组合例: DeepSeek V4 site:github.com OR site:huggingface.co lang:zh after:2025`

// CommonFlags 全局公共选项。
type CommonFlags struct {
	APIKey  string
	BaseURL string
	Timeout time.Duration // 0 = 用 dsweb 默认(search 30s / fetch 60s)
	Retries int
}

// registerCommon 注册全局通用 flag。
func registerCommon(fs *flagSet, cf *CommonFlags) {
	fs.StringVar(&cf.APIKey, "api-key", "", "API Key;也可用环境变量 DEEPSEEK_API_KEY 或 `ds-search auth set` 持久化")
	fs.StringVar(&cf.BaseURL, "base-url", dsweb.BaseURL, "API 基地址(代理/网关调试用)")
	fs.DurationVar(&cf.Timeout, "timeout", 0,
		fmt.Sprintf("单次请求超时,如 45s / 2m;默认 search %s、fetch %s", dsweb.SearchDefaultTimeout, dsweb.FetchDefaultTimeout))
	fs.IntVar(&cf.Retries, "retries", 2, "可重试错误(408/429/5xx/网络错误)的重试次数;0 禁用")
}

// buildClient 用公共选项构造 dsweb.Client。
func buildClient(cf *CommonFlags) *dsweb.Client {
	opts := []dsweb.Option{
		dsweb.WithAPIKey(cf.APIKey),
		dsweb.WithBaseURL(cf.BaseURL),
		dsweb.WithRetries(cf.Retries),
	}
	if cf.Timeout > 0 {
		opts = append(opts, dsweb.WithTimeout(cf.Timeout))
	}
	return dsweb.NewClient(opts...)
}

// resolveClient 完整解析 key 链路(flag > env > 配置文件)后构造 client。
// 返回 error 表示配置文件读取失败(如 JSON 损坏),应反馈给用户而非静默降级。
func resolveClient(cf *CommonFlags) (*dsweb.Client, error) {
	key, _, err := ResolveKey(cf.APIKey)
	if err != nil {
		return nil, err
	}
	cf.APIKey = key
	return buildClient(cf), nil
}

// 退出码总管:把执行错误归类为退出码。
func classify(err error) int {
	var pe *dsweb.ParamError
	var ae *dsweb.APIError
	var te *dsweb.TransportError
	switch {
	case errors.Is(err, dsweb.ErrNoAPIKey):
		return ExitAuth
	case errors.As(err, &ae) && ae.Status == 401:
		return ExitAuth // key 无效
	case errors.As(err, &pe):
		return ExitUsage // 本地校验失败
	case errors.As(err, &te):
		return ExitNetwork
	default:
		return ExitAPI
	}
}
