# ds-search-cli — npm 发包完整指南

> 适用仓库:`deepseek-websearch-cli`(命令 `ds-search`,npm 主包 `ds-search-cli` + 6 个平台子包)
> 模式:esbuild 式「主包 + optionalDependencies 平台子包」。本文是完整操作手册,平时只看第 3 节即可。

## 1. 总览

```
改 VERSION ──► build-release.ps1 ──► pack-npm.mjs ──► (垫片冒烟) ──► publish-npm.ps1 ──► npm 验证
   版本唯一源      6 平台二进制矩阵       npm 包组装       自检布局        7 连发(先子后主)
```

- **版本唯一源** = 仓库根 `VERSION` 文本文件。构建注入 `--version`、npm manifest 里的 version 都从它来。
- **发布顺序固定**:先 6 个平台子包、最后主包。npm 没有「整套原子发布」,而主包 `optionalDependencies` 指向子包的精确版本 — 该版本必须在 registry 上已经存在,否则任何人 `npm i -g` 都会失败(`ETARGET`)。

## 2. 前置准备(一次性)

| 事项 | 命令/动作 | 说明 |
| --- | --- | --- |
| npmjs 账号 | https://www.npmjs.com/signup | 邮箱验证 |
| 登录本机 npm | `npm login` | 浏览器完成验证;`npm whoami` 确认 |
| 2FA(推荐) | npmjs 账号设置里开 | 开启后 publish 需要 6 位码,脚本用 `-Otp` 传入 |
| 自动化 token(可选,免打扰) | npmjs → Access Tokens → Generate(Granular, 包读写) | 嫌逐包弹浏览器烦时用;`npm config set //registry.npmjs.org/:_authToken <token>` 配本机后,发布全程无弹窗 |
| GitHub 仓库 | https://github.com/zhangshichun/ds-search-cli | 已配置:`scripts/pack-npm.mjs` 的 `HOMEPAGE`/`repository` 均由此生成 |
| LICENSE 文件 | 仓库根补 `LICENSE`(MIT) | package.json 已声明 MIT,仓库本体也应有一份 |
| 包名检查 | `npm view ds-search-cli` | 返回 404/E404 = 可用;已占用则需换名 |

## 3. 标准发布流程(日常全部用这一节)

### 一条命令版

```powershell
cd D:\codes\AI\deepseek-websearch-cli

# 新版本:先改 VERSION 文件,再 -Build(构建+打包+发布一次完成)
.\scripts\publish-npm.ps1 -Build

# 常用参数
.\scripts\publish-npm.ps1                 # 不带 -Build:用 dist\npm 现有产物直接发布
.\scripts\publish-npm.ps1 -DryRun         # 预演:显示将发布什么,不实际发包
.\scripts\publish-npm.ps1 -Otp 123456     # 2FA 验证码(6 位)
.\scripts\publish-npm.ps1 -Only subs      # 只发全部平台子包,跳过主包
.\scripts\publish-npm.ps1 -Only win32-x64 # 只补发名称含 win32-x64 的子包(支持部分名匹配)
```

脚本内建:

1. **版本一致性校验** — 主包与 6 个子包 version 必须完全相同,不一致即中止(防「主包指向不存在的子包版本」事故)。
2. **顺序发布** — 先 6 个子包,后主包。
3. **两段式交互(新窗口弹窗)** — 每个 npm publish 都在**新开的独立 PowerShell 窗口**里交互执行(脚本宿主内 stdin 不可交互,PIN 输不进去——踩过的坑):桌面弹出新窗口 → 在那里按回车打开浏览器 → 输 PIN/勾选"信任本 IP"(约 5 分钟)→ 窗口跑完自动关闭;脚本等待完成后提示**按回车**,剩余包在窗口内连续自动发完,连发每满约 4 分钟再插一次检查点。Ctrl+C 随时中止。
4. **幂等断点续发** — 发包前用 `npm view` 预查 registry:该版本已存在则跳过。中断(网络、验证超时)后直接重跑即可,已发布的不会重复发,也不会再插入回车检查点(除非又到了下一个未发布的首包)。
5. **失败即时中止** + 可操作提示(重跑续发;彻底免交互见第 2 节 automation token)。

### 分步版(等效,理解链路时用)

```powershell
.\scripts\build-release.ps1          # ① 6 平台二进制 → dist\bin\<os>-<arch>\ds-search(.exe)
node scripts/pack-npm.mjs            # ② 组装 npm 包 → dist\npm\(主包 + 6 子包 + 冒烟布局)
node dist\npm-install-test\node_modules\ds-search-cli\bin.js --help   # ③ 垫片自检(可选)

cd dist\npm
Get-ChildItem -Directory -Filter 'ds-search-cli-*' |                  # ④ 先发子包
  ForEach-Object { Push-Location $_.FullName; npm publish; Pop-Location }
cd ..\npm\ds-search-cli              # ⑤ 最后发主包
npm publish
```

## 4. 每个环节在做什么

### ① build-release.ps1 — 跨平台构建矩阵

- 平台:windows/linux/darwin × amd64/arm64(GOOS/GOARCH 标准交叉编译,无需本机工具链)
- `CGO_ENABLED=0` 静态单二进制、`-trimpath`、`-ldflags "-s -w -X main.mainVersion=<VERSION>"`(去掉调试信息、剥离路径、注入版本)
- 版本必须来自 `VERSION` 文件,不要手输

### ② pack-npm.mjs — npm 包组装

```
dist\npm\
├── ds-search-cli\                  主包
│   ├── package.json                bin: {"ds-search": "bin.js"},optionalDependencies 指同版本 6 子包
│   ├── bin.js                      垫片:按 process.platform/arch 定位子包二进制,spawn 透传参数/stdio/退出码
│   └── README.md
└── ds-search-cli-<platform>-<arch>\  6 个平台子包
    ├── package.json                os/cpu 字段(npm 安装时自动只装匹配平台)
    └── bin\ds-search(.exe)         对应平台静态二进制
```

- 子包命名 = npm 生态标准(`win32-x64`/`darwin-arm64`…),**不是** GOOS 命名 — bin.js 垫片按 `process.platform`-/`process.arch` 拼包名找子包,不一致就找不到(踩过的坑)。
- 例外:unscoped `ds-search-cli-win32-x64` 被 npmjs spam 检测稳定 E403,该平台子包包名为 **`@zhangshichun/ds-search-cli-win32-x64`**(scoped,public 免费);dist 目录名仍无 scope 前缀,主包 optionalDependencies 混引用,垫片 scoped 优先 + unscoped 兜底。
- 发布前会生成 `dist\npm-install-test\node_modules\` 冒烟布局:模拟 npm 安装后的依赖形态,用于发布前验证垫片能否定位二进制。

### ③ publish-npm.ps1 — 发布

见第 3 节。Windows 上调用 `npm.cmd` 并把 stderr 落临时文件判读(PS 5.1 下 native stderr 不能用 `2>&1` 可靠收集)。

## 5. 发布后验证清单

```powershell
npm view ds-search-cli version                       # = VERSION 的值
npm view ds-search-cli optionalDependencies          # 5 个 unscoped + 1 个 scoped 子包 + 精确版本
npm view "@zhangshichun/ds-search-cli-win32-x64" os cpu   # ["win32"] ["x64"](PowerShell 里带引号防 @ 解析)
```

干净环境真装一次(这是 AI 真实的调用路径):

```powershell
mkdir C:\temp\ds-verify; cd C:\temp\ds-verify
npm i -g ds-search-cli
ds-search version                                    # ds-search x.y.z (go…)
ds-search set-token=sk-xxx
ds-search search "测试" --jsonl --fields title,url    # 真实检索,应有结果/退出码 0
```

Linux/mac 上同流程验证一下 `ds-search` 不带 `.exe` 的调用与执行位(npm 会把 bin.js 软链到 PATH,垫片内部的二进制只需可执行权限 — tar 构建产生的文件在 Windows 上无 unix 位概念,npm 包安装时默认 644;若 Linux 上报 EACCES 需检查子包 `files` 是否保留执行位)。

## 6. 迭代新版本

```powershell
# 1. 改仓库根 VERSION(唯一版本源),如 0.1.0 → 0.2.0
# 2. 一条命令
.\scripts\publish-npm.ps1 -Build
```

- npm 不允许覆盖已发布版本,发坏版本**不能重发同版本号**,只能递增。
- 撤回(unpublish)政策严格:发布 72h 内、且无下游依赖,才可 `npm unpublish <pkg>@<ver>`,别当成常规手段。

## 7. 问题排查表

| 现象 | 原因 | 处置 |
| --- | --- | --- |
| 构建/发布脚本里找不到 `go` | 安装 Go 后未重开终端,旧会话拿不到新 PATH | 脚本已自动重载系统 PATH 自愈;仍失败则安装 `winget install GoLang.Go` |
| publish 报 `ENEEDAUTH` / `E403 forbidden` | 未登录 / 邮箱未验证 / 包名被抢 | `npm whoami` 检查;`npm view <name>` 查占用 |
| publish 报 `E403 "Package name triggered spam detection"` | npmjs 对新账号发 unscoped 的"通用平台后缀"裸名(win32-x64 等)误判为垃圾包;评分黑盒,同批 5 过 1 拦常见,重试与元数据都救不了 | 平台子包改 scoped(pack-npm.mjs 的 PLATFORMS 标 `scoped: true`;bin.js 垫片已内建 scoped 优先/unscoped 兜底);或给 https://npmjs.com/support 申诉 |
| publish 显示成功(`exit 0` / `PUT 202`、提示 being processed)但 `npm view` 仍 404 | npmjs 新发布流程是**异步**:接受后进入处理队列,数分钟才进 registry 索引 | 等几分钟再查;生效前别重跑发布 |
| publish 报 `EOTP`(web-auth 型 2FA) | 账号 2FA 是浏览器/PIN 验证型 | 每个 publish 都在【新弹出的 PowerShell 窗口】里交互执行——按回车/输 PIN/勾选"信任本 IP"要在**那个新窗口**里做,不是主终端;窗口被关掉就重跑脚本续发(已发的自动跳过) |
| publish 报 `EOTP`(TOTP 型 2FA) | 账号用验证器 App | 脚本 `-Otp <6位码>`;码 30 秒窗口过期则换新码重跑(已发包自动跳过) |
| 安装报 `ETARGET … not found` | 主包先发或子包版本号不一致 | 不可能通过脚本的版本校验;检查是否绕过脚本手动发 |
| publish 报 `EPUBLISHCONFLICT` | 同版本已存在 | 一条命令发布时自动跳过视为成功;若需重发只能升版本 |
| `npm i -g` 装上后 `ds-search` 未找到命令 | PATH 未含 npm 全局 bin | `npm bin -g`(或 `npm config get prefix` + /bin)检查 |
| 垫片报 unsupported platform | 子包没随主包装上(如 freebsd) | 垫片会提示;或手动 `npm i -g --force --os x --cpu y` |
| `error: HTTP 401`(运行时) | key 无效/配置文件里混入脏字符 | `auth clear` 后 `set-token=` 重设(CLI 已自动剥离 BOM) |
| `error: HTTP 402`(运行时) | DeepSeek 账户余额不足 | platform.deepseek.com 充值 |
| `error: HTTP 429`(运行时) | QPS 超限(Search/Fetch 各 5/账号) | CLI 已自动退避重试;持续 429 降低调用频率 |

## 8. 安全注意

- **API key 永不进包**:二进制与 npm 包里没有 key 存储逻辑,key 只在用户机器的配置文件(0600)或环境变量里。
- **绝不把 token 写进仓库/脚本**;CI 场景用 npmjs 的 granular access token 存 `NPM_TOKEN` secret,`publish-npm.ps1` 目前按交互登录设计,CI 化时改为 `--//registry.npmjs.org/:_authToken` 方式。
- postinstall 脚本:**不用**(安全审查高危,CI `--ignore-scripts` 也会装坏);二进制由平台子包携带,垫片只做定位。

---

*修订:2026-10-19(与仓库脚本同期;流程变更时同步本文件)。*