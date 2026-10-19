---
name: ds-search
description: 安装、配置与使用 ds-search — DeepSeek 网页搜索/抓取 CLI。当需要实时联网检索(search)、抓取网页全文(fetch)时，使用本SKILL。
---

# ds-search — DeepSeek 网页搜索/抓取 CLI

单一二进制、零依赖、非交互,专为 AI Agent 设计:`search` 做实时联网检索,`fetch` 抓单页转 Markdown。

## 何时用

- 需要训练数据之外的**实时信息**(版本发布、时间敏感事实、站内资料)→ `search`
- 已有具体 URL,要读页面全文(转 RAG 输入、提炼内容)→ `fetch`
- 上下文里已有答案的问题**不要调用**(每次成功请求计费)

## 安装与验证

```bash
npm i -g ds-search-cli        # 装好后命令名是 ds-search(包名 ds-search-cli)
ds-search version             # 形如 ds-search 0.2.0 (go1.27.0);exit 0 即安装成功
ds-search help                # 内嵌完整帮助(命令/语法/退出码),输出即权威参考
```

## API Key 配置

三种方式按优先级:`--api-key sk-xxx`(单次) > 环境变量 `DEEPSEEK_API_KEY` > 配置文件(持久)。

```bash
ds-search set-token=sk-xxx            # 快捷保存(推荐;等价 auth set)
echo "sk-xxx" | ds-search auth set --stdin    # 经 stdin 保存,不进 shell history
ds-search auth show                   # 显示掩码 key=sk-2723…7f9e source=config config_file=…,验证已配置
ds-search clear-token                 # 快捷清除(= auth clear)
ds-search config path                 # 打印配置文件路径
```

配置文件:`%APPDATA%\ds-search\config.json`(Windows)/ `~/.config/ds-search/config.json`(Linux)等用户配置目录,权限 0600,内容明文。

**安全纪律**:key/验证码绝不写入对话、脚本或版本库;共享机器优先用环境变量或 `--api-key` 参数,不用配置文件。

## search — 联网检索

```bash
# AI 推荐模式:每条一行 JSON + 只取关键字段(紧凑、单行可解析)
ds-search search "DeepSeek V4 发布" --jsonl --fields title,url,summary

# 控制条数(1~50,默认 10)
ds-search search "rust async runtime" --jsonl --fields title,url --count 5

# 搜索语法直接写在 query 里(原样透传,可组合):
#   site:go.dev 限定站点 / -site:example.com 排除站点(多 site 用 OR 连接)
#   -word -"短语" 排除词/短语 / "精确短语" 精确匹配
#   lang:zh,en 限定语言 / after:2025-01-01 before:2026-06-01 时间窗
ds-search search "Golang generics site:go.dev after:2024" --jsonl

# 需要网页正文时(默认 false,content 恒空;正文大,按需开)
ds-search search "DeepSeek 引擎缓存" --content --jsonl --fields title,url,content

# query 也可来自 stdin 管道
echo "MCP streamable HTTP" | ds-search search --jsonl
```

- query:1~128 字符,可含上述操作符;`--ori-question "原始问题"` 可选传入触发搜索的用户原话,提升意图理解
- 结果字段:`title url snippet summary content site_name site_icon date rerank_score(0~3 相关性,≥1.5 高质量) authority(0~4 权威度,-1 未知)`
- `--fields` 白名单别名:`score`→`rerank_score`、`site`→`site_name`;`--jsonl`/`--json`/`--text` 三选一
- 需要快筛时优先看 `rerank_score` 与 `authority`,高分优先 `fetch` 全文

## fetch — 抓单页转 Markdown

```bash
ds-search fetch "https://example.com/article" --raw           # 直接输出 Markdown 正文(推荐)
ds-search fetch "https://example.com/article"                 # JSON {"title","content"}
ds-search fetch "https://example.com/article" --json --fields title    # 只要标题
```

URL 为 http/https,最长 8192 字符。输出为页面 Markdown 全文,可直接作为 RAG/摘要的输入。

## 退出码与故障解读(AI 编排依据)

| 码  | 含义                                               | 建议动作                        |
| --- | -------------------------------------------------- | ------------------------------- |
| 0   | 成功(search 0 结果也算成功)                        | 按输出内容继续                  |
| 1   | 服务端错误(402 余额不足、404 页面抓不到、400 参数) | 402 → 告知用户充值;404 → 换 URL |
| 2   | API key 缺失或无效                                 | 提示 `set-token=` 重新配置      |
| 3   | 网络错误                                           | 可重试;查网络                   |
| 4   | 本地参数错误                                       | 修正参数再试                    |

错误只写 stderr(单行 `error: ...`),stdout 纯净可解析。408/429/5xx 已内置指数退避重试,无需 AI 层重试;持续 429 = QPS 超限(Search/Fetch 各 5/账号),应降频或分散请求。

## 计费与限制

- 仅**成功且非空**请求计费,约 ¥5/1000 次(以官方页 https://api-docs.deepseek.com/zh-cn/guides/search/ 为准)
- Search 5 QPS、Fetch 5 QPS,账号粒度
