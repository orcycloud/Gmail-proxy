<#
.SYNOPSIS
  将 gmail-proxy 注册为 Windows 服务（使用内置 sc.exe，无需额外工具）
.DESCRIPTION
  1. 确保 .\bin\gmail-proxy.exe 和 .\configs\config.yaml 存在
  2. 创建服务，自动启动
  3. 停止 / 删除服务使用 -Action stop|delete
#>
param(
    [ValidateSet("install", "start", "stop", "restart", "delete", "status")]
    [string]$Action = "install"
)

$ErrorActionPreference = "Stop"
$ServiceName = "GmailProxy"
$ExePath = (Resolve-Path (Join-Path $PSScriptRoot "..\bin\gmail-proxy.exe")).Path
$ConfigPath = (Resolve-Path (Join-Path $PSScriptRoot "..\configs\config.yaml")).Path

switch ($Action) {
    "install" {
        if (-not (Test-Path $ExePath)) {
            Write-Error "找不到 $ExePath，请先运行 .\scripts\build.ps1"
            exit 1
        }
        if (-not (Test-Path $ConfigPath)) {
            Write-Error "找不到 $ConfigPath"
            exit 1
        }
        $args = "-config `"$ConfigPath`""
        $binPath = "`"$ExePath`" $args"

        sc.exe create $ServiceName binPath= $binPath start= auto DisplayName= "Gmail Proxy (TLS Forwarder"
        sc.exe description $ServiceName "TLS 透传代理：POP3S(995) -> pop.gmail.com:995, SMTPS(465) -> smtp.gmail.com:465"
        sc.exe failure $ServiceName reset= 86400 actions= restart/5000/restart/5000/restart/5000

        Write-Host ""
        Write-Host "服务已创建，正在启动..." -ForegroundColor Cyan
        sc.exe start $ServiceName
        Write-Host "完成 ✓" -ForegroundColor Green
    }
    "start"   { sc.exe start   $ServiceName }
    "stop"    { sc.exe stop    $ServiceName }
    "restart" { sc.exe stop    $ServiceName; Start-Sleep -Seconds 2; sc.exe start $ServiceName }
    "delete"  {
        sc.exe stop    $ServiceName 2>$null | Out-Null
        Start-Sleep -Seconds 1
        sc.exe delete  $ServiceName
    }
    "status"  { sc.exe query   $ServiceName }
}
