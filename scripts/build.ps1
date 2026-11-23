<#
.SYNOPSIS
  本地构建 gmail-proxy，输出到 .\bin\ 目录
.DESCRIPTION
  默认构建当前平台。可通过 -Target linux 交叉编译。
#>
param(
    [ValidateSet("windows", "linux")]
    [string]$Target = "windows"
)

$ErrorActionPreference = "Stop"

$outDir = Join-Path $PSScriptRoot "..\bin"
New-Item -ItemType Directory -Force -Path $outDir | Out-Null

$ext = if ($Target -eq "windows") { ".exe" } else { "" }
$outFile = Join-Path $outDir "gmail-proxy$ext"

Write-Host "==> 构建目标: $Target"
Write-Host "==> 输出:     $outFile"

$env:CGO_ENABLED = "0"
$env:GOOS = $Target
$env:GOARCH = "amd64"

go build -ldflags="-s -w" -o $outFile .\cmd\proxy

if ($LASTEXITCODE -ne 0) {
    Write-Error "构建失败"
    exit 1
}

Write-Host ""
Write-Host "构建完成 ✓" -ForegroundColor Green
Get-Item $outFile | Select-Object Name, Length, LastWriteTime
