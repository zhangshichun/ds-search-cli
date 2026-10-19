# publish-npm.ps1 — 一条命令发布整套 npm 包(自动先子包后主包)
#
# 用法:
#   .\scripts\publish-npm.ps1                       # 直接发(要求 dist\npm 已打包)
#   .\scripts\publish-npm.ps1 -Build                # 发布前先重新构建全平台 + 打包
#   .\scripts\publish-npm.ps1 -DryRun               # 只显示将要发布什么,不发
#   .\scripts\publish-npm.ps1 -Otp 123456           # 2FA 账号一次性带验证码
#   .\scripts\publish-npm.ps1 -Only subs             # 只发全部平台子包,跳过主包
#   .\scripts\publish-npm.ps1 -Only win32-x64        # 只发名称含 win32-x64 的子包
#   .\scripts\publish-npm.ps1 -Build -Only subs      # 组合
#
# 幂等:同版本已存在于 registry 的包会自动跳过(先子包后主包),
# 可以安全地断点续发。
# 交互节奏:首个真实发布的包会弹出浏览器(web-auth PIN/授权),
# 完成后脚本暂停等回车,剩余包在 IP 信任窗口(npmjs 约 5 分钟)内连续自动发完。
param(
    [switch]$Build,
    [switch]$DryRun,
    [string]$Otp,
    [string[]]$Only
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot

if ($Build) {
    Write-Host '== 重新构建 =='
    & (Join-Path $PSScriptRoot 'build-release.ps1')
    if ($LASTEXITCODE -ne 0) { Write-Error '构建失败'; exit 1 }
    node (Join-Path $PSScriptRoot 'pack-npm.mjs')
    if (-not (Test-Path (Join-Path $root 'dist\npm\ds-search-cli\package.json'))) { Write-Error '打包失败'; exit 1 }
}

$npmDir = Join-Path $root 'dist\npm'
$allSubs = @(Get-ChildItem $npmDir -Directory -Filter 'ds-search-cli-*' | Sort-Object Name)
$mainItem = Get-Item (Join-Path $npmDir 'ds-search-cli')
if (-not $mainItem) { Write-Error '主包目录不存在'; exit 1 }

# -Only 解析:'subs' = 全部子包;其余值匹配子包名(支持部分名前缀)。
$subs = $allSubs
$includeMain = $true
if ($Only) {
    if ($Only -contains 'subs') {
        $includeMain = $false
    } else {
        $subs = @($allSubs | Where-Object { $n = $_.Name; ($Only | Where-Object { $n -like "*$_" }) })
        $includeMain = $false
        if (-not $subs) { Write-Error "没有匹配 -Only 的子包;$($allSubs.Name -join ', ')"; exit 1 }
    }
}

# 一轮读全部 manifest(主包 + 全部子包各读一次),同时完成:
# (1)版本一致性校验 — 与 -Only 选了哪些无关,整包产物必须互洽;
# (2)登记各包真名 — scoped 包目录名与包名不同,幂等预查与发布都用真名。
$metaByDir = @{}
$mainVer = $null
$mainPkgName = $null
foreach ($item in (@($mainItem) + $allSubs)) {
    $meta = Get-Content (Join-Path $item.FullName 'package.json') -Raw | ConvertFrom-Json
    $metaByDir[$item.FullName] = $meta
    if ($item.FullName -eq $mainItem.FullName) {
        $mainVer = $meta.version
        $mainPkgName = $meta.name
        continue
    }
    if ($meta.version -ne $mainVer) { Write-Error "版本不一致: $($item.Name)=$($meta.version) != 主包=$mainVer"; exit 1 }
}

# registry 预检:该子包版本是否已存在(幂等断点续发的判断依据)。
function Test-Published([string]$pkgName) {
    $errTmp = [IO.Path]::GetTempFileName()
    $outTmp = [IO.Path]::GetTempFileName()
    try {
        # -Wait 同步;exit 0 = 已发布,非 0(404/E404) = 未发布。
        # stdout/stderr 都落临时文件 —— 主终端里不应该出现 npm view 的输出。
        $p = Start-Process 'npm.cmd' -ArgumentList @('view', "$pkgName@$mainVer", 'version') -Wait -NoNewWindow -PassThru -RedirectStandardOutput $outTmp -RedirectStandardError $errTmp
        return ($p.ExitCode -eq 0)
    }
    finally {
        Remove-Item $errTmp, $outTmp -Force -ErrorAction SilentlyContinue
    }
}

# 单包发布:在【新开的真实 console 窗口】里交互执行 npm publish。
# 教训:PS 宿主进程里的子命令拿不到可交互 stdin,npm 的 web-auth
# "Press ENTER to open in the browser" 读 stdin 失败 → 直接 EOTP,PIN 没法输。
# 新窗口 = 普通 PowerShell 终端,回车/浏览器/PIN/勾选信任 IP 都在里面完成,
# 与手动发布体验一致;本脚本 -Wait 等该窗口跑完,再继续下一个包。
function Publish-One([IO.DirectoryInfo]$dir) {
    $pkg = Join-Path $dir.FullName 'package.json'
    if (-not (Test-Path $pkg)) { Write-Error "缺 package.json: $($dir.Name)"; exit 1 }

    # 发前净化:npm 依据 CI 检测与 npm_config_* env 决定弹窗还是直接 EOTP;
    # 摘掉所有能触发非交互路径的变量,发布后恢复。Windows env 名不分大小写。
    $ciVars = @(
        'CI', 'CI_NAME', 'CI_SERVER_*', 'CONTINUOUS_INTEGRATION', 'BUILD_NUMBER', 'BUILD_ID',
        'BUILD_URL', 'BUILDKITE', 'TEAMCITY_VERSION', 'GITHUB_ACTIONS', 'GITHUB_WORKFLOW',
        'GITLAB_CI', 'CIRCLECI', 'TRAVIS', 'APPVEYOR', 'DRONE', 'AGENT_NAME', 'AGENT_JOBNAME',
        'JENKINS_URL', 'NODE_AUTH_TOKEN', 'NPM_CONFIG_OTP', 'NPM_CONFIG_REGISTRY',
        'NPM_CONFIG_*', 'npm_config_*'
    )
    $backup = @{}   # env 名 → 值;PowerShell hashtable 默认不分大小写,贴合 Windows env 语义
    foreach ($e in (Get-ChildItem Env:)) {
        foreach ($pattern in $ciVars) {
            if ($e.Name -ilike $pattern) { $backup[$e.Name] = $e.Value; break }
        }
    }
    foreach ($name in @($backup.Keys)) { Remove-Item "Env:$name" -Force -ErrorAction SilentlyContinue }

    Write-Host ("== 发布 {0} @ {1} ==" -f $dir.Name, $mainVer) -NoNewline
    Write-Host '  → 关注桌面新弹出的 PowerShell 窗口:按回车弹浏览器、输 PIN、勾选"信任本 IP"' -ForegroundColor DarkGray

    # 在新 console 窗口里交互执行;不做任何输出重定向(子窗口自带真实
    # console,stdin/stdout/stderr 都是它的),env 继承父进程(已净化)。
    # --access public:scoped 包必须显式声明(默认走私有收费路径 → E402);
    # unscoped 包显式 public 与默认一致,无副作用。
    $subCmd = "Set-Location '$($dir.FullName)'; npm publish --access public"
    if ($Otp) { $subCmd += " --otp=$Otp" }
    $subCmd += '; exit $LASTEXITCODE'
    try {
        $p = Start-Process 'powershell.exe' -ArgumentList @('-NoProfile', '-ExecutionPolicy', 'Bypass', '-Command', $subCmd) -Wait -PassThru
        $code = $p.ExitCode
    }
    finally {
        foreach ($kv in $backup.GetEnumerator()) { Set-Item -Path "Env:$($kv.Key)" -Value $kv.Value }
    }
    if ($code -ne 0) {
        Write-Host "  ✗ 发布失败 $($dir.Name)(上方是 npm 原始输出)" -ForegroundColor Red
        Write-Host '  直接重跑本命令续发(已发布的自动跳过);彻底免交互可配 automation token(见 docs/PUBLISH.md)' -ForegroundColor Yellow
        return $false
    }
    Write-Host "  ✓ $($dir.Name)" -ForegroundColor Green
    return $true
}

# 组建发布队列:已选子包在先,主包最后(npm 无原子整套发布,顺序是硬约束);
# PkgName 从前面读好的 manifest 取真名。
$queue = @($subs | ForEach-Object { [pscustomobject]@{ Dir = $_; PkgName = $metaByDir[$_.FullName].name } })
if ($includeMain) {
    $queue += [pscustomobject]@{ Dir = $mainItem; PkgName = $mainPkgName }
}

# dry-run 与实跑共用的 registry 预检:该包版本已存在 → 打印跳过并返回 $true。
function Skip-IfPublished($item) {
    if (Test-Published $item.PkgName) {
        Write-Host "  ✓ $($item.PkgName)($mainVer 已在 registry,跳过)" -ForegroundColor Green
        return $true
    }
    return $false
}

if ($DryRun) {
    # dry-run 同样动态预查 registry:显示"跳过哪些、发哪几个",与实跑一致。
    $pending = 0
    foreach ($item in $queue) {
        if (Skip-IfPublished $item) { continue }
        Write-Host ("[dry-run] npm publish  {0,-44} @ {1}" -f $item.PkgName, $mainVer)
        $pending++
    }
    Write-Host "[dry-run] 共 $($queue.Count) 个:$pending 个待发,$($queue.Count - $pending) 个已在 registry。未实际发布。" -ForegroundColor Cyan
    exit 0
}

# 逐包按序发布;已发布的(断点续发)直接跳过。
# 交互节奏:npmjs web 授权在首个真实发布的包处弹窗(勾选"信任本 IP"后约 5 分钟),
# 完成第一个包后暂停等用户回车,随后剩余包在窗口内连续自动发完;
# 连发每满约 4 分钟再插一次检查点 — 5 分钟窗口静默到期会让后面的包整批 EOTP,
# 与其如此,不如到点暂停,让用户重新过一次弹窗即重建窗口。
$realPublished = 0
$windowStart = $null
for ($i = 0; $i -lt $queue.Count; $i++) {
    $item = $queue[$i]
    if (Skip-IfPublished $item) { continue }
    if (-not (Publish-One $item.Dir)) { exit 1 }
    $realPublished++
    if ($realPublished -eq 1) {
        # 记录信任窗口起点(首个真实发布完成即开始计时)。
        $windowStart = Get-Date
    }
    $remaining = $queue.Count - $i - 1
    if ($remaining -le 0) { break }

    $firstOfRun = ($realPublished -eq 1)
    $expiring = $windowStart -and (((Get-Date) - $windowStart).TotalMinutes -gt 4)
    if (-not ($firstOfRun -or $expiring)) { continue }

    Write-Host ''
    if ($firstOfRun) {
        Write-Host '✓ 首包已发布,信任窗口已建立。' -ForegroundColor Cyan
        Write-Host "  剩余 $remaining 个未发布:回车后在窗口内连续自动发完。" -ForegroundColor Cyan
    }
    else {
        Write-Host '⏳ 连发已超 4 分钟,本 IP 信任窗口(约 5 分钟)临近到期。' -ForegroundColor Yellow
        Write-Host "  剩余 $remaining 个:回车后继续;若下一个包重新弹窗验证,完成即重建 5 分钟窗口。" -ForegroundColor Cyan
    }
    if ([Environment]::UserInteractive -and -not [Console]::IsInputRedirected) {
        Read-Host -Prompt '  回车继续(Ctrl+C 中止)' | Out-Null
    }
    Write-Host ''
    $windowStart = Get-Date   # 从检查点重新计时(若用户刚重新验证,窗口同样刷新)
}

$mainPart = ''
if ($includeMain) { $mainPart = '+ 主包 ds-search-cli' }
if ($realPublished -eq 0) {
    Write-Host "全部包已在 registry($mainVer),无需发布。" -ForegroundColor Green
} else {
    Write-Host "发布完成:实发 $realPublished 个包 $mainPart @ $mainVer。" -ForegroundColor Green
}
Write-Host "验证: npm view $mainPkgName version"