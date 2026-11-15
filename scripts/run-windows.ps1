# OpsMini 本地运行脚本（Windows 开发/试用）
#
# 作用：构建 → 启动面板 → 等端口就绪 → 用默认浏览器打开 → 打印初始密码。
# 用法（普通 PowerShell 即可）：
#   .\scripts\run-windows.ps1
#   .\scripts\run-windows.ps1 -Port 18888
#
# 数据目录默认 %LOCALAPPDATA%\OpsMini-run，与正式安装（Program Files + ProgramData）隔离。

[CmdletBinding()]
param(
    [int]$Port = 18888,
    [string]$DataDir = "$env:LOCALAPPDATA\OpsMini-run"
)

$ErrorActionPreference = "Stop"
$repo = Split-Path -Parent $PSScriptRoot
$binDir = Join-Path $repo "dist"
$bin = Join-Path $binDir "opsmini.exe"

New-Item -ItemType Directory -Force -Path $binDir | Out-Null
New-Item -ItemType Directory -Force -Path $DataDir | Out-Null

Write-Host "构建 opsmini.exe ..."
Push-Location $repo
try {
    go build -o $bin ./cmd/agent
    if ($LASTEXITCODE -ne 0) { throw "构建失败" }
} finally {
    Pop-Location
}

$configPath = Join-Path $DataDir "config.yaml"
$dbPath = (Join-Path $DataDir "opsmini.db") -replace '\\', '/'
@"
server:
    host: "127.0.0.1"
    port: $Port
    secret_entry: ""
database:
    path: "$dbPath"
jwt:
    access_ttl_seconds: 900
    refresh_ttl_seconds: 604800
log:
    level: warn
    path: ""
agent:
    server_addr: ""
    server_token: ""
    heartbeat_seconds: 30
"@ | Set-Content -Path $configPath -Encoding UTF8

Write-Host "启动面板（数据目录 $DataDir）..."
$proc = Start-Process -FilePath $bin -ArgumentList "-config", "`"$configPath`"" -PassThru -WindowStyle Hidden

$url = "http://127.0.0.1:$Port/"
for ($i = 0; $i -lt 60; $i++) {
    Start-Sleep -Milliseconds 700
    try {
        $resp = Invoke-WebRequest -Uri $url -UseBasicParsing -TimeoutSec 2
        if ($resp.StatusCode -eq 200) { break }
    } catch { }
}

Write-Host "用默认浏览器打开 $url"
Start-Process $url

$pwdFile = Join-Path $DataDir ".init_passwd"
if (Test-Path $pwdFile) {
    Write-Host ""
    Write-Host "首次启动，初始账号：opsmini"
    Write-Host ("初始密码：" + (Get-Content $pwdFile -Raw).Trim())
    Write-Host "（登录后请立即修改；密码文件 $pwdFile 可删除）"
} else {
    Write-Host "面板已初始化过，沿用原有账号密码。"
}
Write-Host ""
Write-Host "进程 PID $($proc.Id)，停止：Stop-Process -Id $($proc.Id)"
