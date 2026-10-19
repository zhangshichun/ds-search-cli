# build-release.ps1 — 跨平台构建矩阵(GOOS/GOARCH 全平台二进制)
# 用法:
#   .\scripts\build-release.ps1                 # 全平台矩阵 → dist\bin\
#   .\scripts\build-release.ps1 -Targets windows-amd64   # 指定平台
#   .\scripts\build-release.ps1 -Version 0.1.2  # 覆盖 VERSION 文件
param(
    [string]$Version,
    [string[]]$Targets = @(
        'windows-amd64', 'windows-arm64',
        'linux-amd64',   'linux-arm64',
        'darwin-amd64',  'darwin-arm64'
    )
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

# PATH 自愈:安装 Go 后未重开终端的会话里找不到 go 时,自动重载系统级 PATH。
function Ensure-GoOnPath {
    if (Get-Command go -ErrorAction SilentlyContinue) { return }
    $env:Path = [System.Environment]::GetEnvironmentVariable('Path', 'Machine') + ';' +
                [System.Environment]::GetEnvironmentVariable('Path', 'User')
    if (Get-Command go -ErrorAction SilentlyContinue) {
        Write-Host '(PATH 已自动刷新找到 go — 无需重开终端)'
        return
    }
    Write-Error @'
找不到 go。安装一次即可: winget install GoLang.Go
装完重开终端,或把 C:\Program Files\Go\bin 加入 PATH。
'@
    exit 1
}
Ensure-GoOnPath

if (-not $Version) {
    $Version = (Get-Content (Join-Path $root 'VERSION') -Raw).Trim()
    if (-not $Version) { Write-Error 'VERSION 文件为空'; exit 1 }
}
Write-Host "== 构建版本 $Version =="

$ldflags = "-s -w -X main.mainVersion=$Version"

foreach ($t in $Targets) {
    if ($t -notmatch '^(windows|linux|darwin)-(amd64|arm64)$') {
        Write-Error "未知目标 $t(应为 <os>-<arch>)"; exit 1
    }
    $goos  = $Matches[1]
    $goarch = $Matches[2]
    $exe = if ($goos -eq 'windows') { 'ds-search.exe' } else { 'ds-search' }
    $outDir = Join-Path $root "dist\bin\$goos-$goarch"
    New-Item -ItemType Directory -Force $outDir | Out-Null
    $out = Join-Path $outDir $exe

    $env:GOOS = $goos; $env:GOARCH = $goarch; $env:CGO_ENABLED = '0'
    try {
        go build -trimpath -ldflags $ldflags -o $out ./cmd/ds-search
        if ($LASTEXITCODE -ne 0) { Write-Error "构建失败: $t"; exit 1 }
        $size = (Get-Item $out).Length / 1MB
        Write-Host ("  {0,-14} -> {1}  ({2:N1} MB)" -f $t, $out, $size)
    }
    finally {
        Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED -ErrorAction SilentlyContinue
    }
}

# 全部目标构建完成。
Write-Host "== 完成: dist\bin\ =="