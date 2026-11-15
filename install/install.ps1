# OpsMini Windows 安装 / 升级 / 卸载脚本
#
# 用法（管理员 PowerShell）：
#   .\install.ps1                       # 安装到 C:\Program Files\OpsMini，端口 8888
#   .\install.ps1 -Port 9999            # 自定义端口
#   .\install.ps1 -InstallDir D:\opsmini
#   .\install.ps1 -Uninstall            # 停止并移除服务（保留数据目录）
#   .\install.ps1 -ServerAddr 1.2.3.4:4507 -ServerToken <token>   # 装完即接入 OpsAnt
#
# 一键安装（管理员 PowerShell）：
#   iwr -useb http://<opsant>/uploads/agent/opsmini/install.ps1 -OutFile $env:TEMP\opsmini.ps1
#   & $env:TEMP\opsmini.ps1 -ServerAddr <ip>:4507 -ServerToken <token>
#
# 首次安装完成后，初始管理员密码写入 <数据目录>\.init_passwd。
# 数据目录默认 C:\ProgramData\OpsMini。

[CmdletBinding()]
param(
    [string]$InstallDir = "C:\Program Files\OpsMini",
    [string]$DataDir = "C:\ProgramData\OpsMini",
    [int]$Port = 8888,
    [string]$Binary = "",
    # 安装包所在目录（如 http://<opsant>/uploads/agent/opsmini）；留空则回退官方 OSS
    [string]$BaseUrl = "",
    # 二进制下载地址，优先级最高；一般不用填
    [string]$BinaryUrl = "",
    [string]$Version = "v1.5.0",
    # 接入 OpsAnt 的 OpsMini Server（host:port）与 token。
    # 给了这两项，服务启动后自己就会注册，不需要装完再进面板手工填一遍。
    [string]$ServerAddr = "",
    [string]$ServerToken = "",
    [switch]$Uninstall
)

$ErrorActionPreference = "Stop"
$ServiceName = "OpsMini"
$DefaultUrlBase = "https://opsmini.oss-cn-beijing.aliyuncs.com"

function Test-Admin {
    $id = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = New-Object Security.Principal.WindowsPrincipal($id)
    return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

# 没有管理员权限就自己拉一个提权实例跑完，而不是甩一句"请以管理员身份运行"。
# 「应用和功能」里的卸载入口按 UninstallString 启动，默认不带提权；只报错的话
# 用户点完卸载看起来毫无反应，是这类脚本最常见的坑。
if (-not (Test-Admin)) {
    $argList = @("-NoProfile", "-ExecutionPolicy", "Bypass", "-File", ('"' + $PSCommandPath + '"'))
    foreach ($name in $PSBoundParameters.Keys) {
        $value = $PSBoundParameters[$name]
        if ($value -is [System.Management.Automation.SwitchParameter]) {
            if ($value.IsPresent) { $argList += "-$name" }
        } else {
            $argList += "-$name"
            $argList += ('"' + $value + '"')
        }
    }
    Write-Host "需要管理员权限，正在请求提权..."
    try {
        Start-Process -FilePath "powershell.exe" -ArgumentList $argList -Verb RunAs | Out-Null
    } catch {
        throw "提权被拒绝或失败：$($_.Exception.Message)"
    }
    return
}

# ---------- 卸载 ----------
if ($Uninstall) {
    $svc = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
    if ($svc) {
        if ($svc.Status -ne "Stopped") {
            Stop-Service -Name $ServiceName -Force
            (Get-Service $ServiceName).WaitForStatus("Stopped", "00:00:30")
        }
        sc.exe delete $ServiceName | Out-Null
        Write-Host "已移除服务 $ServiceName（数据目录 $DataDir 保留）"
    } else {
        Write-Host "服务 $ServiceName 不存在"
    }
    # 不清掉这一项，卸载完「应用和功能」里还挂着一条点不动的残留。
    $arpKey = "HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\OpsMini"
    if (Test-Path $arpKey) {
        Remove-Item -Path $arpKey -Recurse -Force
        Write-Host "已从「应用和功能」移除 OpsMini"
    }
    return
}

# ---------- 定位二进制 ----------
if (-not $Binary) {
    $candidate = Join-Path $PSScriptRoot "opsmini.exe"
    if (Test-Path $candidate) {
        $Binary = $candidate
    }
}

if (-not $Binary) {
    if (-not $BinaryUrl) {
        $base = if ($BaseUrl) { $BaseUrl.TrimEnd('/') } else { $DefaultUrlBase }
        # 目标机架构：包名里的 amd64 / arm64 必须与之一致
        $arch = if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "amd64" }
        $BinaryUrl = "$base/opsmini-$Version-windows-$arch.exe"
    }
    $tmpExe = Join-Path $env:TEMP ("opsmini-" + $Version + "-windows-" + $arch + ".exe")
    Write-Host "正在下载 $BinaryUrl"
    try {
        # 进度条会让 iwr 在小文件上慢一个数量级，这里关掉
        $ProgressPreference = "SilentlyContinue"
        Invoke-WebRequest -UseBasicParsing -Uri $BinaryUrl -OutFile $tmpExe -TimeoutSec 120
    } catch {
        throw "下载失败：$BinaryUrl`n$($_.Exception.Message)`n请用 -Binary <本机路径> 或 -BinaryUrl <可访问地址> 重试。"
    }
    $Binary = $tmpExe
}

if (-not (Test-Path $Binary)) {
    throw "未找到 opsmini.exe：$Binary"
}
$Binary = (Resolve-Path $Binary).Path

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
New-Item -ItemType Directory -Force -Path $DataDir | Out-Null

# ---------- 停止旧服务，替换二进制 ----------
$svc = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
$isUpgrade = $false
if ($svc) {
    $isUpgrade = $true
    if ($svc.Status -ne "Stopped") {
        Stop-Service -Name $ServiceName -Force
        (Get-Service $ServiceName).WaitForStatus("Stopped", "00:00:30")
    }
}

Copy-Item -Path $Binary -Destination (Join-Path $InstallDir "opsmini.exe") -Force
Write-Host "已复制二进制到 $InstallDir\opsmini.exe"

# 脚本自身也落地一份：卸载入口（UninstallString）必须指向稳定路径，
# 不能指向 %TEMP% 里那份随时会被清掉的下载。
$scriptCopy = Join-Path $InstallDir "install.ps1"
if ($PSCommandPath -and (Test-Path $PSCommandPath)) {
    if ((Resolve-Path $PSCommandPath).Path -ne $scriptCopy) {
        Copy-Item -Path $PSCommandPath -Destination $scriptCopy -Force
    }
}

# ---------- 配置文件 ----------
$configPath = Join-Path $InstallDir "config.yaml"

# 在已有配置里就地改写 agent 段：升级时带 -ServerAddr 也能把接入信息写进去，
# 而不是因为"文件已存在"整段被跳过。
function Set-AgentSection {
    param([string]$Path, [string]$Addr, [string]$Token)

    $lines = Get-Content -Path $Path
    $out = New-Object System.Collections.Generic.List[string]
    $inAgent = $false
    $foundAgent = $false

    foreach ($line in $lines) {
        if ($line -match '^agent:\s*$') {
            $inAgent = $true
            $foundAgent = $true
            $out.Add($line)
            $out.Add("    server_addr: `"$Addr`"")
            $out.Add("    server_token: `"$Token`"")
            continue
        }
        if ($inAgent) {
            if ($line -match '^\s+server_addr:') { continue }
            if ($line -match '^\s+server_token:') { continue }
            # 遇到下一个顶级键，agent 段结束
            if ($line -match '^[A-Za-z_]') { $inAgent = $false }
        }
        $out.Add($line)
    }

    if (-not $foundAgent) {
        $out.Add("agent:")
        $out.Add("    server_addr: `"$Addr`"")
        $out.Add("    server_token: `"$Token`"")
        $out.Add("    heartbeat_seconds: 30")
    }

    Set-Content -Path $Path -Value $out -Encoding UTF8
}

if (-not (Test-Path $configPath)) {
    $dbPath = (Join-Path $DataDir "opsmini.db") -replace '\\', '/'
    $yaml = @"
server:
    host: "0.0.0.0"
    port: $Port
    secret_entry: ""
database:
    path: "$dbPath"
jwt:
    access_ttl_seconds: 900
    refresh_ttl_seconds: 604800
log:
    level: error
    path: ""
agent:
    server_addr: "$ServerAddr"
    server_token: "$ServerToken"
    heartbeat_seconds: 30
"@
    Set-Content -Path $configPath -Value $yaml -Encoding UTF8
    Write-Host "已生成配置 $configPath"
} elseif ($ServerAddr) {
    Set-AgentSection -Path $configPath -Addr $ServerAddr -Token $ServerToken
    Write-Host "已把 OpsAnt 接入信息写入 $configPath"
}

# ---------- 注册服务 ----------
$binPath = '"' + (Join-Path $InstallDir "opsmini.exe") + '" -config "' + $configPath + '"'
if (-not $isUpgrade) {
    sc.exe create $ServiceName binPath= $binPath start= auto DisplayName= "OpsMini 运维面板" | Out-Null
    sc.exe description $ServiceName "OpsMini 单机运维面板（Web $Port）" | Out-Null
    # 面板需要读性能计数器、管理服务与防火墙，LocalSystem 是最省事且可用的身份。
    sc.exe config $ServiceName obj= "LocalSystem" | Out-Null
    Write-Host "已注册服务 $ServiceName"
} else {
    sc.exe config $ServiceName binPath= $binPath | Out-Null
    Write-Host "已更新服务 $ServiceName 的启动参数"
}

# ---------- 防火墙 ----------
$rule = Get-NetFirewallRule -DisplayName "OpsMini Panel" -ErrorAction SilentlyContinue
if (-not $rule) {
    New-NetFirewallRule -DisplayName "OpsMini Panel" -Direction Inbound -Protocol TCP `
        -LocalPort $Port -Action Allow | Out-Null
    Write-Host "已放行入站 TCP $Port"
}

Start-Service -Name $ServiceName
(Get-Service $ServiceName).WaitForStatus("Running", "00:00:30")

# ---------- 注册表：让「应用和功能」能看到并卸载 ----------
# 没有这一项，用户只能翻文档找 -Uninstall；资产清点工具（Intune/SCCM）也看不到这个软件。
$exePath = Join-Path $InstallDir "opsmini.exe"
$installedVersion = $Version
try {
    $vi = (Get-Item $exePath).VersionInfo
    if ($vi.FileVersion) { $installedVersion = ($vi.FileVersion -replace '\s', '') }
} catch { }
$sizeKB = 0
try { $sizeKB = [int]((Get-Item $exePath).Length / 1KB) } catch { }

$arpKey = "HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\OpsMini"
New-Item -Path $arpKey -Force | Out-Null
$uninstallCmd = 'powershell.exe -NoProfile -ExecutionPolicy Bypass -File "' + (Join-Path $InstallDir "install.ps1") + '" -Uninstall'
Set-ItemProperty -Path $arpKey -Name DisplayName         -Value "OpsMini 运维面板"
Set-ItemProperty -Path $arpKey -Name DisplayVersion      -Value $installedVersion
Set-ItemProperty -Path $arpKey -Name Publisher           -Value "OpsMini"
Set-ItemProperty -Path $arpKey -Name DisplayIcon         -Value $exePath
Set-ItemProperty -Path $arpKey -Name InstallLocation     -Value $InstallDir
Set-ItemProperty -Path $arpKey -Name UninstallString     -Value $uninstallCmd
Set-ItemProperty -Path $arpKey -Name QuietUninstallString -Value $uninstallCmd
Set-ItemProperty -Path $arpKey -Name URLInfoAbout        -Value "https://opsmini.com"
Set-ItemProperty -Path $arpKey -Name InstallDate         -Value (Get-Date -Format "yyyyMMdd")
Set-ItemProperty -Path $arpKey -Name EstimatedSize       -Value $sizeKB -Type DWord
Set-ItemProperty -Path $arpKey -Name NoModify            -Value 1 -Type DWord
Set-ItemProperty -Path $arpKey -Name NoRepair            -Value 1 -Type DWord
Write-Host "已注册「应用和功能」条目（版本 $installedVersion，卸载入口 $scriptCopy）"

# ---------- 结果 ----------
Write-Host ""
Write-Host "OpsMini 已启动：http://<本机IP>:$Port（版本 $installedVersion）"
if ($ServerAddr) {
    Write-Host "已配置接入 OpsAnt：$ServerAddr（服务启动后自动注册，稍后到 OpsAnt 的「待纳管」确认即可）"
}
$pwdFile = Join-Path $DataDir ".init_passwd"
if (Test-Path $pwdFile) {
    Write-Host "初始管理员账号：opsmini"
    Write-Host ("初始密码：" + (Get-Content $pwdFile -Raw).Trim() + "（已保存到 " + $pwdFile + "，登录后请立即修改）")
} else {
    Write-Host "未发现 .init_passwd：面板可能已初始化过，密码沿用原有设置。"
}
