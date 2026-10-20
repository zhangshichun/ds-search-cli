---
name: ds-search
description: 安装、配置和使用 ds-search 命令行工具（DeepSeek 网页搜索与网页抓取）。当需要获取训练数据之外的实时信息，或需要读取某个网页的完整正文时使用。
---

# ds-search：DeepSeek 网页搜索与抓取命令行工具

ds-search 提供两项能力：`search` 用于联网搜索实时信息，`fetch` 用于抓取指定网页并转换为 Markdown 正文。命令不读取交互输入，适合在脚本和自动化流程中直接调用。

## 适用场景

- 需要训练数据之外的实时信息，例如软件版本发布、时间敏感的事实、某个站点内的资料时，使用 `search`。
- 已经拿到具体网址，需要读取页面全文用于 RAG、摘要或内容提炼时，使用 `fetch`。
- 如果上下文里已经有答案，不要调用：每次成功的请求都会计费。

## 安装与验证

```bash
npm i -g ds-search-cli        # npm 包名是 ds-search-cli，安装后的命令名是 ds-search
ds-search version             # 输出形如 ds-search 0.2.0 (go1.27.0)，退出码 0 表示安装成功
ds-search help                # 输出完整的命令、搜索语法和退出码说明
```

## 配置 API Key

API Key 按以下优先级读取：`--api-key` 参数（仅本次调用）> 环境变量 `DEEPSEEK_API_KEY` > 配置文件（持久保存）。

```bash
ds-search set-token=sk-xxx                    # 保存到配置文件（推荐）
echo "sk-xxx" | ds-search auth set --stdin    # 通过标准输入保存，不会留在 shell 历史里
ds-search auth show                           # 查看当前生效的 Key（掩码显示）及来源
ds-search clear-token                         # 清除已保存的 Key
ds-search config path                         # 查看配置文件位置
```

Key 属于敏感信息，不要写进对话、脚本或版本库。多人共用的机器上，优先使用环境变量或 `--api-key`，不要保存到配置文件。

## search：联网搜索

```bash
# 推荐用法：每条结果一行 JSON，并用 --fields 只保留需要的字段
ds-search search "DeepSeek V4 发布" --jsonl --fields title,url,summary

# 指定返回条数（1~50，默认 10）
ds-search search "rust async runtime" --jsonl --fields title,url --count 5

# 搜索语法直接写在 query 里，原样传给服务端，可以组合使用：
#   site:go.dev                          只搜索指定站点；-site:example.com 排除指定站点
#   -word  -"短语"                       排除某个词或短语
#   "精确短语"                           精确匹配
#   lang:zh,en                           限定语言
#   after:2025-01-01 before:2026-06-01   限定时间范围
ds-search search "Golang generics site:go.dev after:2024" --jsonl

# 需要网页正文时加 --content（默认不返回，因为正文体积较大）
ds-search search "DeepSeek 引擎缓存" --content --jsonl --fields title,url,content

# query 也可以通过管道从标准输入传入
echo "MCP streamable HTTP" | ds-search search --jsonl
```

- query 长度为 1~128 个字符。可选的 `--ori-question "原始问题"` 用于传入触发搜索的用户原话，帮助服务端理解搜索意图。
- 每条结果包含这些字段：`title`、`url`、`snippet`（短摘要）、`summary`（长摘要）、`content`（正文，需要 `--content`）、`site_name`、`site_icon`、`date`、`rerank_score`（0~3 的相关性评分，1.5 以上为高质量）、`authority`（0~4 的权威度评分，-1 表示未知）。
- `--fields` 支持别名：`score` 等同于 `rerank_score`，`site` 等同于 `site_name`。输出格式 `--jsonl`、`--json`、`--text` 三选一。
- 快速筛选时优先看 `rerank_score` 和 `authority`，分数高的结果再用 `fetch` 读取全文。

## fetch：抓取网页并转为 Markdown

```bash
ds-search fetch "https://example.com/article" --raw                    # 直接输出 Markdown 正文（推荐）
ds-search fetch "https://example.com/article"                          # 输出 JSON：{"title","content"}
ds-search fetch "https://example.com/article" --json --fields title    # 只输出标题
```

URL 必须是 http 或 https 地址。输出是页面正文的 Markdown，可以直接作为 RAG 或摘要的输入。

## 退出码与常见故障

| 退出码 | 含义 | 建议处理 |
| --- | --- | --- |
| 0 | 成功（search 返回 0 条结果也算成功） | 按输出继续处理 |
| 1 | 服务端错误（402 余额不足、404 页面抓不到、400 参数错误） | 402：告知用户充值；404：换一个 URL |
| 2 | API Key 缺失或无效 | 用 `set-token=` 重新配置 |
| 3 | 网络错误 | 可以重试，或检查网络 |
| 4 | 本地参数错误 | 修正参数后重试 |

错误信息只写到 stderr（单行 `error: ...`），stdout 始终是可解析的纯净输出。网络抖动和限流（408/429/5xx）CLI 会自动重试，调用方不必自己重试；如果持续返回 429，说明超过了 QPS 限制（见下节）。

## 计费与调用限制

- 只有成功且返回非空的请求才计费，约 ¥5/1000 次，具体价格以[官方文档](https://api-docs.deepseek.com/zh-cn/guides/search/)为准。
- Search 和 Fetch 各有 5 QPS 的限制，按账号计算。
