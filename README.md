# deepseek-websearch-cli

> npm package: `ds-search-cli` · command: `ds-search`

English | [简体中文](README.zh-CN.md)

A CLI wrapper and Go client library for the [DeepSeek Web Search & Fetch API](https://api-docs.deepseek.com/zh-cn/guides/search/) (`POST /v1/tools/search`, `POST /v1/tools/fetch`), designed for AI agents: structured output, fully non-interactive, scriptable exit codes, and a single static binary with zero dependencies.

[![npm version](https://img.shields.io/npm/v/ds-search-cli)](https://www.npmjs.com/package/ds-search-cli)
[![license](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

## Features

- **Two capabilities, one command surface**: `search` for real-time web search (DeepSeek's in-house engine), `fetch` to capture a page as Markdown
- **AI-friendly output**: JSON Lines (`--jsonl`) for one-line-per-result parsing, `--fields` whitelist projection; errors go to stderr only, stdout stays clean
- **Scriptable exit codes**: five tiers — success / server / auth / network / usage — so orchestration never needs to parse stderr
- **API key chain**: CLI flag > environment variable > config file (0600), with quick commands `set-token=` / `clear-token`
- **Zero retry plumbing**: built-in exponential backoff on 408/429/5xx (honors `Retry-After`); other 4xx are never retried (matches official behavior)
- **Single cross-platform binary**: Windows / Linux / macOS × amd64 / arm64, statically built, no CGO
- **npm distribution** (esbuild pattern): `npm i -g ds-search-cli` automatically installs only the subpackage matching your platform

| API | CLI |
| --- | --- |
| Search — query the web, get structured results | `ds-search search "query"` |
| Fetch — capture a web page, get Markdown | `ds-search fetch "URL" --raw` |

## Install

```bash
# npm (recommended — AI can call ds-search right after)
npm i -g ds-search-cli

# Verify
ds-search version      # ds-search 0.2.0 (go1.27.0); exit 0 means you're set
ds-search help         # built-in full help: commands / search syntax / exit codes
```

> Note for AI agents: `ds-search help` embeds the complete command reference, search syntax, and exit-code table — reading it once is enough to use the tool correctly.

## Build from source

```bash
git clone https://github.com/zhangshichun/ds-search-cli.git && cd ds-search-cli
go build -o bin/ds-search.exe ./cmd/ds-search     # or .\scripts\build-release.ps1 for the full 6-platform matrix
```

Platform subpackages ship prebuilt binaries. `win32-x64` is published as a scoped package (`@zhangshichun/ds-search-cli-win32-x64`) because npmjs's spam filter misjudges plain unscoped platform names — the shim handles both transparently, so users and scripts don't need to care.

## API key (create one first at https://platform.deepseek.com — keys start with sk-)

Resolution order, highest first:

| Source | Usage | Best for |
| --- | --- | --- |
| 1. CLI flag | `--api-key sk-xxx` (highest priority) | CI / one-off scripts |
| 2. Environment | `DEEPSEEK_API_KEY=sk-xxx` | containers / per-session |
| 3. Config file | `ds-search set-token=sk-xxx` (persistent) | personal machines |

```bash
# Quick save (one step; both forms are equivalent)
ds-search set-token=sk-xxx
ds-search set-token sk-xxx      # NOTE: passing the key as an visible argument writes it into shell history

# Save via stdin (history-safe)
echo "sk-xxx" | ds-search auth set --stdin
ds-search auth set --stdin < key.txt

ds-search auth show              # masked: key=sk-2723…7f9e source=config config_file=…
ds-search clear-token            # quick clear (= auth clear)
ds-search config path            # print the config file path
```

The config file lives in the per-user config dir with 0600 permissions: `%APPDATA%\ds-search\config.json` (Windows) / `~/.config/ds-search/config.json` (Linux) / `~/Library/Application Support/ds-search/config.json` (macOS). The key is stored in plaintext — on shared machines prefer the env var or `--api-key`.

## search

```bash
# Basic search (human-readable text by default)
ds-search search "DeepSeek V4 release"

# Recommended for AI: one JSON object per line, only the fields you need
ds-search search "DeepSeek V4 release" --jsonl --fields title,url,summary

# Result count 1-50, default 10
ds-search search "rust async runtime" --json --count 20

# Return page content (default false; content stays empty unless enabled — it's large, use when needed)
ds-search search "DeepSeek engine caching" --content --jsonl --fields title,url,content

# Read the query from stdin
echo "MCP streamable HTTP" | ds-search search --jsonl
```

**Search syntax** (typed directly into the query, forwarded as-is, freely combinable):

| Operator | Example | Meaning |
| --- | --- | --- |
| `site:` | `DeepSeek site:github.com` | restrict to a site; multiple sites: space-separated clauses joined by `OR` |
| `-site:` | `-site:example.com` | exclude a site |
| `-word` | `-Ads` | exclude results containing the word |
| `-"phrase"` | `-"release notes"` | exclude a phrase (may contain spaces) |
| `"phrase"` | `"exact phrase"` | exact phrase match |
| `lang:` | `lang:zh,en` | restrict to languages (comma-separated) |
| `after:` / `before:` | `after:2025-01-01` | publication-time window |

**Result fields**: `title url snippet summary content site_name site_icon date rerank_score (0–3 relevance, ≥1.5 = high quality) authority (0–4 authority tier, -1 unknown)`. `--fields` aliases: `score`→`rerank_score`, `site`→`site_name`.

Triage strategy: judge candidates by `rerank_score` / `authority` first, then `fetch` the full text of the best hits.

## fetch

```bash
ds-search fetch "https://example.com/article" --raw        # print the Markdown body directly
ds-search fetch "https://example.com/article"              # JSON {"title","content"}
ds-search fetch "https://example.com/article" --json --fields title
```

URL must be http/https and ≤ 8192 chars. The page's Markdown full text is a ready-made input for RAG / summarization.

## Exit codes

| Code | Meaning |
| --- | --- |
| 0 | success (a search with 0 results is still success) |
| 1 | server-side error (incl. 402 insufficient balance, 404 fetch failed, 400 bad request) |
| 2 | API key missing or invalid (401 / not configured) |
| 3 | network error (DNS / connect / timeout) |
| 4 | local usage error (count out of range, unknown flag, missing query, …) |

Errors go to stderr as a single `error: ...` line. 408/429/5xx are retried automatically with backoff (2 retries by default, `--retries 0` to disable).

## Agent skill package

[`skills/ds-search/SKILL.md`](skills/ds-search/SKILL.md) is a ready-made Agent Skill (install, token set/clear, core search/fetch usage, exit-code semantics for orchestration). Link or copy it to `.claude/skills/ds-search/` and AI sessions can load it on demand.

## As a Go library

```go
import "deepseek-websearch-cli/dsweb"

client := dsweb.NewClient(dsweb.WithAPIKey(os.Getenv("DEEPSEEK_API_KEY")))

resp, err := client.Search(ctx, &dsweb.SearchRequest{Query: "DeepSeek V4 release", Count: 5})
page, err := client.Fetch(ctx, &dsweb.FetchRequest{URL: "https://example.com/article"})
```

Errors classify precisely: `dsweb.APIError` (with `Retryable()` / status code), `dsweb.TransportError`, `dsweb.ParamError`, `dsweb.ErrNoAPIKey`.

## Project layout

```
deepseek-websearch-cli/
├── cmd/ds-search/          CLI entry (command dispatch)
├── dsweb/                  Go client library (stdlib only, zero deps)
├── internal/cli/           CLI implementation (search / fetch / auth / config / output / flag reordering)
├── npm/bin.js              npm shim: locate the platform binary, passthrough args/stdio/exit code
├── scripts/                build-release.ps1 · pack-npm.mjs · publish-npm.ps1
├── skills/ds-search/       AI agent skill (SKILL.md)
├── docs/PUBLISH.md         full npm publishing manual
└── VERSION                 single source of version truth (build injection + npm manifests)
```

## Development

```powershell
go test ./...                  # unit tests (httptest mock servers cover lib + CLI)
.\scripts\build-release.ps1    # 6-platform build matrix → dist\bin\
node scripts/pack-npm.mjs      # assemble npm artifacts → dist\npm\ + post-install smoke layout
```

## Publishing (npm)

See **[docs/PUBLISH.md](docs/PUBLISH.md)** for the full publishing manual (prereqs, per-step walkthrough, verification checklist, troubleshooting, security notes). One command for daily use:

```powershell
.\scripts\publish-npm.ps1 -DryRun         # dry run: lists what will publish and what gets skipped
.\scripts\publish-npm.ps1 -Build          # real: rebuild + pack + publish in order (6 subpackages → main)
.\scripts\publish-npm.ps1 -Otp 123456     # 2FA one-time code
.\scripts\publish-npm.ps1 -Only win32-x64 # resume publishing one package
```

Key points: the main package's `optionalDependencies` pin exact subpackage versions, so **subpackages first, main package last**; same-version republication is impossible (EPUBLISHCONFLICT), and the script is idempotent for safe resume. npmjs's new publish flow is asynchronous (PUT 202, "being processed") — `npm view` shows the package only minutes later, so don't misread that as failure.

To release a new version: change `VERSION` → `.\scripts\publish-npm.ps1 -Build`.

## Billing & rate limits (official)

- Search / Fetch are billed per **successful, non-empty** request — roughly ¥5 per 1,000 calls (failures, empty results, and timeouts are not billed)
- QPS: Search 5, Fetch 5 (per account; exceeding returns 429)
- Details: [Search/Fetch guide](https://api-docs.deepseek.com/zh-cn/guides/search/)

## License

[MIT](LICENSE) © ds-search-cli contributors