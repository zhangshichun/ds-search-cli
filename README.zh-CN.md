# deepseek-websearch-cli

> npm 包:`ds-search-cli` · 命令名:`ds-search`

[English](README.md) | 简体中文

[DeepSeek 网页搜索与抓取 API](https://api-docs.deepseek.com/zh-cn/guides/search/)(`POST /v1/tools/search`、`POST /v1/tools/fetch`)的命令行封装与 Go 客户端库,**面向 AI Agent 设计**:结构化输出、非交互、可判定退出码、单二进制零依赖。

[![npm version](https://img.shields.io/npm/v/ds-search-cli)](https://www.npmjs.com/package/ds-search-cli)
[![license](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

## 特性

- **两个能力,一组命令**:`search` 实时联网检索(DeepSeek 自研引擎),`fetch` 抓单页转 Markdown
- **AI 友好输出**:JSON Lines(`--jsonl`)单行可解析、`--fields` 字段白名单裁剪;错误只走 stderr,stdout 纯净
- **可判定退出码**:成功 / 服务端 / 认证 / 网络 / 参数五档,AI 编排无需解析 stderr
- **Key 三级链**:命令行参数 > 环境变量 > 配置文件(0600),快捷命令 `set-token=` / `clear-token`
- **零重试心智**:408/429/5xx 内置指数退避重试(遵循 Retry-After),其余 4xx 不重试(与官方一致)
- **单二进制跨平台**:Windows / Linux / macOS × amd64 / arm64 六平台静态编译,无 CGO
- **npm 分发**(esbuild 模式):`npm i -g ds-search-cli` 自动只装当前平台子包

| 接口 | CLI |
| --- | --- |
| Search — 按查询词搜索网页,返回结构化结果 | `ds-search search "查询词"` |
| Fetch — 抓取网页,返回 Markdown | `ds-search fetch "URL" --raw` |

## 安装

```bash
# npm(推荐 — 装好后 AI 直接调用 ds-search)
npm i -g ds-search-cli

# 验证
ds-search version      # ds-search 0.2.0 (go1.27.0),exit 0 即成功
ds-search help         # 内嵌完整帮助:命令 / 搜索语法 / 退出码
```

**给 AI 的注意点**:`ds-search help` 已内嵌完整命令/搜索语法/退出码说明,读完一次即可正确调用。

## 从源码构建

```bash
git clone https://github.com/zhangshichun/ds-search-cli.git && cd ds-search-cli
go build -o bin/ds-search.exe ./cmd/ds-search      # 或 .\scripts\build-release.ps1 全平台矩阵
```

平台子包随包发布预编译二进制;`win32-x64` 因 npmjs 对 unscoped 平台名的反垃圾误判走 scoped 包(`@zhangshichun/ds-search-cli-win32-x64`),垫片自动兼容,使用方无感。

## API Key 配置(先到 https://platform.deepseek.com 获取 sk- 开头的 key)

优先级从高到低:

| 方式 | 用法 | 适用 |
| --- | --- | --- |
| 1. 命令行参数 | `--api-key sk-xxx`(最高) | CI / 脚本一次性 |
| 2. 环境变量 | `DEEPSEEK_API_KEY=sk-xxx` | 容器 / 会话级 |
| 3. 配置文件 | `ds-search set-token=sk-xxx`(持久) | 个人机推荐 |

```bash
# 快捷保存(一步,两种写法等价)
ds-search set-token=sk-xxx
ds-search set-token sk-xxx      # 注意:参数形式会进 shell history,慎用

# 经 stdin 保存(history 安全)
echo "sk-xxx" | ds-search auth set --stdin
ds-search auth set --stdin < key.txt

ds-search auth show              # key=sk-2723…7f9e source=config config_file=…(掩码展示)
ds-search clear-token            # 快捷清除(= auth clear)
ds-search config path            # 打印配置文件路径
```

配置文件在用户配置目录(0600):`%APPDATA%\ds-search\config.json`(Windows)/ `~/.config/ds-search/config.json`(Linux)/ `~/Library/Application Support/ds-search/config.json`(macOS),内容为明文——共享机器请用环境变量或 `--api-key`。

## search 用法

```bash
# 基本搜索(默认人读文本)
ds-search search "DeepSeek V4 发布"

# AI 推荐:每条结果一行 JSON,只取关键字段(输出最紧凑,解析最稳)
ds-search search "DeepSeek V4 发布" --jsonl --fields title,url,summary

# 条数 1~50,默认 10
ds-search search "rust async runtime" --json --count 20

# 需要正文(默认只给摘要;content 默认恒空,按需开)
ds-search search "DeepSeek 引擎缓存" --content --jsonl --fields title,url,content

# 从管道读 query
echo "MCP streamable HTTP" | ds-search search --jsonl
```

**搜索语法**(写在 query 里,原样透传,可组合):

| 操作符 | 示例 | 说明 |
| --- | --- | --- |
| `site:` | `DeepSeek site:github.com` | 限定站点;多个用空格 + `OR` 连接 |
| `-site:` | `-site:example.com` | 排除站点 |
| `-词` | `-广告` | 排除含关键词的结果 |
| `-"短语"` | `-"release notes"` | 排除短语(可含空格) |
| `"短语"` | `"精确短语"` | 短语精确匹配 |
| `lang:` | `lang:zh,en` | 限定语言(逗号分隔) |
| `after:` / `before:` | `after:2025-01-01` | 发布时间窗 |

**结果字段**:`title url snippet summary content site_name site_icon date rerank_score(0~3 相关性,≥1.5 高质量) authority(0~4 权威度,-1 未知)`;`--fields` 支持别名 `score`→`rerank_score`、`site`→`site_name`。

快筛策略:先用 `rerank_score` / `authority` 判断价值,高质量条目再 `fetch` 全文。

## fetch 用法

```bash
ds-search fetch "https://example.com/article" --raw        # 直接输出 Markdown 正文
ds-search fetch "https://example.com/article"              # JSON {"title","content"}
ds-search fetch "https://example.com/article" --json --fields title
```

URL ≤ 8192 字符;返回页面 Markdown 全文,可直接作为 RAG / 摘要输入。

## 退出码

| 码 | 含义 |
| --- | --- |
| 0 | 成功(搜索 0 结果也算成功) |
| 1 | 服务端错误(含 402 余额不足、404 抓取失败、400 请求非法等) |
| 2 | API key 缺失或无效(401 / 未配置) |
| 3 | 网络错误(DNS / 连接 / 超时) |
| 4 | 本地参数错误(count 越界、未知 flag、query 缺失等) |

错误只写 stderr(单行 `error: ...`);408/429/5xx 自动退避重试(默认 2 次,`--retries 0` 关闭)。

## AI 技能包

[`skills/ds-search/SKILL.md`](skills/ds-search/SKILL.md) 是沉淀好的 Agent Skill(安装、token 配置/清除、search/fetch 核心用法、编排退出码语义)——链接或复制到 `.claude/skills/ds-search/` 即可让 AI 会话按需加载。

## 作为 Go 库使用

```go
import "deepseek-websearch-cli/dsweb"

client := dsweb.NewClient(dsweb.WithAPIKey(os.Getenv("DEEPSEEK_API_KEY")))

resp, err := client.Search(ctx, &dsweb.SearchRequest{Query: "DeepSeek V4 发布", Count: 5})
page, err := client.Fetch(ctx, &dsweb.FetchRequest{URL: "https://example.com/article"})
```

错误可精确分类:`dsweb.APIError`(带 `Retryable()` / 状态码)、`dsweb.TransportError`、`dsweb.ParamError`、`dsweb.ErrNoAPIKey`。

## 项目结构

```
deepseek-websearch-cli/
├── cmd/ds-search/          CLI 入口(命令分发)
├── dsweb/                  Go 客户端库(标准库零依赖)
├── internal/cli/           CLI 实现(search / fetch / auth / config / output / flag 重排)
├── npm/bin.js              npm 垫片:定位平台二进制并透传参数、stdio、退出码
├── scripts/                build-release.ps1 · pack-npm.mjs · publish-npm.ps1
├── skills/ds-search/       AI Agent 技能(SKILL.md)
├── docs/PUBLISH.md         npm 发包完整手册
└── VERSION                 版本唯一源(构建注入 + npm manifest)
```

## 开发

```powershell
go test ./...                  # 单测(httptest 假服务器覆盖库与 CLI)
.\scripts\build-release.ps1    # 6 平台矩阵构建 → dist\bin\
node scripts/pack-npm.mjs      # 组装 npm 产物 → dist\npm\ + 安装后冒烟布局
```

## 发布(npm)

完整发包手册见 **[docs/PUBLISH.md](docs/PUBLISH.md)**(前置准备、逐环节说明、验证清单、排查表、安全注意)。日常一条命令:

```powershell
.\scripts\publish-npm.ps1 -DryRun         # 预演:动态列出将发的包与已跳过项
.\scripts\publish-npm.ps1 -Build          # 正式:重新构建+打包+按序发布(子包 ×6 → 主包)
.\scripts\publish-npm.ps1 -Otp 123456     # 2FA 验证码
.\scripts\publish-npm.ps1 -Only win32-x64 # 断点补发
```

要点:主包 `optionalDependencies` 指向子包精确版本,必须**先子包后主包**;同版本不可覆盖(EPUBLISHCONFLICT),脚本幂等续发;npmjs 新发布流程为异步(PUT 202,"being processed"),`npm view` 数分钟后才能查到,勿误判失败。

发新版:只改 `VERSION` → `.\scripts\publish-npm.ps1 -Build`。

## 计费与限速(官方)

- Search / Fetch 均按**成功且非空**的请求计费,约 5 元 / 1000 次(失败、空结果、超时不计费)
- QPS:Search 5、Fetch 5(账号粒度,超限 429)
- 详情见 [Search/Fetch 指南](https://api-docs.deepseek.com/zh-cn/guides/search/)

## 许可证

[MIT](LICENSE) © ds-search-cli contributors