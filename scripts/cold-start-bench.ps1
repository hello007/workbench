# WorkBench GUI 冷启动自动化测量脚本（perf-baseline.md §6 收口，2026-09-27）
#
# 测量口径（每轮一次采样，跑 N 轮取中位数）：
#   进程拉起           -> 脚本 Start-Process 前后时钟（t0）
#   logger initialized -> app.log slog JSON 时间戳（NewAppServices 起点附近）
#   web serve listening-> app.log slog JSON 时间戳（浏览器通道就绪，晚于实际 net.Listen）
#   TCP 36115 就绪     -> 主事件循环内单次 TcpClient Connect 探测（与日志事件同循环
#                         采样，消除串行高估；listen 就绪早于 workbench started 日志）
#   主窗口句柄出现     -> 轮询 MainWindowHandle != 0（WebView2 窗口创建完成的自动化代理下界，
#                         不含首帧绘制；人工秒表口径见 docs/spec/perf-baseline.md §6.1）
#   workbench started  -> app.log slog JSON 时间戳（Go startup 全部完成）
#   进程工作集         -> 稳态后 Get-Process WorkingSet64 / PrivateMemorySize64
#
# 运行：powershell -ExecutionPolicy Bypass -File scripts/cold-start-bench.ps1 [-Runs 5]
# 依赖：build/bin/workbench.exe 已 wails build；无其他进程占用 36115（脚本会预检并退出）。
# 注意：测量期间桌面窗口会闪现 N 次，属正常现象；每轮结束发 WM_CLOSE 优雅退出
#       （触发 shutdown 清 crash.flag），超时才 taskkill /F 并手工清 data/crash.flag。
# 只测量不改生产代码；Windows 专用（Get-Process / MainWindowHandle）。

param(
    [int]$Runs = 5,                                        # 采样轮数（另有 3 轮不计入的热身）
    [string]$ExePath = "build/bin/workbench.exe",
    [int]$Port = 36115,                                    # 浏览器通道默认端口（data/settings.json webServe.bindAddress）
    [string]$LogPath = "data/logs/app.log"
)

$ErrorActionPreference = "Stop"
# 输出统一 UTF-8（PS 5.1 重定向管道默认 OEM 代码页，中文乱码）
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$repoRoot = Split-Path -Parent $PSScriptRoot
Set-Location $repoRoot

if (-not (Test-Path $ExePath)) {
    Write-Error "未找到 $ExePath，请先执行 wails build"
    exit 1
}
if (-not (Test-Path $LogPath)) {
    Write-Error "未找到 $LogPath（应用至少启动过一次生成日志）"
    exit 1
}

# 预检：已有 workbench 实例运行（常驻实例）会占用 36115 且日志串场，拒绝测量
$existing = Get-Process workbench -ErrorAction SilentlyContinue
if ($existing) {
    Write-Error "检测到已在运行的 workbench.exe（PID $($existing.Id -join ','))，请先关闭后再测量（避免端口占用与日志串场）"
    exit 1
}

function Read-LogTail([long]$Offset) {
    # 从指定字节偏移读取新增日志行，解析 slog JSON。
    # offset 只推进到最后一个换行处：跨轮询边界的半截尾行留待下轮读全——被测事件行
    # （logger initialized / workbench started）恰可能落在边界上，截断即静默丢失。
    # lumberjack 5MB 轮转后新文件变短：offset 复位 0 并告警，防后续事件永久丢失。
    $fs = [System.IO.File]::Open($LogPath, 'Open', 'Read', 'ReadWrite')
    try {
        if ($fs.Length -lt $Offset) {
            Write-Warning "日志轮转检测到（文件 $([math]::Round($fs.Length/1KB,1)) KB < offset $Offset B），offset 复位 0 重读"
            $script:logOffset = 0
            $Offset = 0
        }
        if ($fs.Length -eq $script:logOffset) { return @() }
        $fs.Position = $Offset
        $sr = New-Object System.IO.StreamReader($fs)
        $text = $sr.ReadToEnd()
        $lastNl = $text.LastIndexOf("`n")
        if ($lastNl -lt 0) { return @() }   # 无完整行，offset 不动
        $script:logOffset = $script:logOffset + $lastNl + 1
        $text = $text.Substring(0, $lastNl)
    } finally { $fs.Close() }
    $lines = $text -split "`n" | ForEach-Object { $_.TrimEnd("`r") } | Where-Object { $_.EndsWith("}") }
    $events = @()
    foreach ($line in $lines) {
        try {
            $json = $line | ConvertFrom-Json
            $events += [pscustomobject]@{
                Msg  = $json.msg
                Time = [DateTimeOffset]::Parse($json.time).UtcDateTime
            }
        } catch { }  # 非 JSON 行（历史格式）忽略
    }
    return $events
}

function Test-TcpOnce {
    # 单次 TCP 连接尝试（后端服务就绪点探测）。在主事件循环内随日志轮询反复调用，
    # 与被测日志事件同一时钟循环采样——listen 就绪早于 `workbench started` 日志，
    # 若放在循环之后串行测会把 TCP 就绪系统性高估（下界为 startup 检测时刻）。
    $client = New-Object System.Net.Sockets.TcpClient
    try {
        $task = $client.ConnectAsync("127.0.0.1", $Port)
        if ($task.Wait(50) -and $client.Connected) { return [DateTime]::UtcNow }
    } catch { } finally { $client.Close() }
    return $null
}

function Wait-WindowHandle($Proc, [int]$TimeoutSec) {
    # 轮询主窗口句柄出现（WebView2 宿主窗口创建完成的代理）。实测句柄晚于
    # `workbench started` 日志出现，无法并入主事件循环，startup 检测后启动本轮询；
    # 读数为「startup 检测时刻 + 本段耗时」，对窗口真实创建时刻是上界代理。
    $deadline = [DateTime]::UtcNow.AddSeconds($TimeoutSec)
    while ([DateTime]::UtcNow -lt $deadline) {
        $Proc.Refresh()
        if ($Proc.HasExited) { return $null }
        if ($Proc.MainWindowHandle -ne [IntPtr]::Zero) { return [DateTime]::UtcNow }
        Start-Sleep -Milliseconds 10
    }
    return $null
}

function Stop-MeasuredProcess($Proc) {
    # 优先优雅退出（WM_CLOSE 触发 shutdown 清 crash.flag），超时强杀并清理崩溃标记。
    # Stop-Process 加 SilentlyContinue：WaitForExit 返回 false 与强杀之间进程恰好
    # 自行退出的竞态窗口下，不因「找不到进程」终止错误丢掉整轮汇总。
    if ($Proc.HasExited) { return }
    $null = $Proc.CloseMainWindow()
    if (-not $Proc.WaitForExit(10000)) {
        Stop-Process -Id $Proc.Id -Force -ErrorAction SilentlyContinue
        $flag = Join-Path $repoRoot "data/crash.flag"
        if (Test-Path $flag) { Remove-Item $flag -Force }
    }
}

function Get-Median([double[]]$Values) {
    # 空集返回 $null（汇总显示 N/A）：全轮测量失败时不得伪装成中位数 0 掩盖故障
    if (-not $Values -or $Values.Count -eq 0) { return $null }
    $sorted = $Values | Sort-Object
    return [math]::Round($sorted[[int][math]::Floor(($sorted.Count - 1) / 2)], 1)
}

$rows = @()

# 热身 3 轮不计入（OS 文件缓存 / Defender 缓存 / WebView2 profile 首建离散大，实测
# 逐轮递减 1580→963 ms，3 轮后趋于平稳）
Write-Host "=== WorkBench GUI 冷启动测量（$Runs 轮 + 3 轮热身）===" -ForegroundColor Cyan
Write-Host "热身轮（不计入统计）..."
foreach ($warm in 1..3) {
    $logOffset = (Get-Item $LogPath).Length
    $p = Start-Process -FilePath $ExePath -WorkingDirectory $repoRoot -PassThru
    while (-not (Test-TcpOnce)) { Start-Sleep -Milliseconds 10 }
    Start-Sleep -Milliseconds 1500
    Stop-MeasuredProcess $p
    Start-Sleep -Milliseconds 500
    Write-Host "  热身 $warm 完成"
}

for ($i = 1; $i -le $Runs; $i++) {
    Start-Sleep -Milliseconds 500
    $logOffset = (Get-Item $LogPath).Length
    # 统一 UtcNow（Kind=Utc）与日志解析出的 UtcDateTime 同 Kind：.NET DateTime
    # 相减不看 Kind 只看 ticks，Local/Utc 混用会差出整个时区偏移（实测 +8h）
    $t0 = [DateTime]::UtcNow

    $p = Start-Process -FilePath $ExePath -WorkingDirectory $repoRoot -PassThru

    # 主事件循环：日志事件与 TCP 就绪同一循环采样（消除串行高估），直到
    # workbench started 出现（Go startup 完成标志）
    $events = @()
    $tcpReady = $null
    $deadline = [DateTime]::UtcNow.AddSeconds(30)
    while ([DateTime]::UtcNow -lt $deadline) {
        $events += Read-LogTail $logOffset
        if (-not $tcpReady) { $tcpReady = Test-TcpOnce }
        if ($events | Where-Object Msg -eq "workbench started") { break }
        Start-Sleep -Milliseconds 5
    }

    $winHandle = Wait-WindowHandle $p 30
    Start-Sleep -Milliseconds 1500   # 稳态后采样内存
    $p.Refresh()
    $wsMB  = [math]::Round($p.WorkingSet64 / 1MB, 1)
    $privMB = [math]::Round($p.PrivateMemorySize64 / 1MB, 1)
    Stop-MeasuredProcess $p

    $ev = @{ }
    foreach ($e in $events) { if (-not $ev.ContainsKey($e.Msg)) { $ev[$e.Msg] = $e.Time } }

    $ms = { param($t) if ($t) { [math]::Round(($t - $t0).TotalMilliseconds, 1) } else { $null } }
    $rows += [pscustomobject]@{
        轮次                  = $i
        进程拉起至logger初始化ms = & $ms $ev["logger initialized"]
        进程拉起至webServe监听ms = & $ms $ev["web serve listening"]
        后端TCP就绪ms          = if ($tcpReady) { [math]::Round(($tcpReady - $t0).TotalMilliseconds, 1) } else { $null }
        主窗口句柄出现ms        = if ($winHandle) { [math]::Round(($winHandle - $t0).TotalMilliseconds, 1) } else { $null }
        GoStartup完成ms        = & $ms $ev["workbench started"]
        工作集MB              = $wsMB
        私有内存MB            = $privMB
    }
    Write-Host "第 $i 轮完成：GoStartup $($rows[-1].GoStartup完成ms) ms / TCP就绪 $($rows[-1].后端TCP就绪ms) ms / 窗口句柄 $($rows[-1].主窗口句柄出现ms) ms / 工作集 $wsMB MB"
}

Write-Host ""
Write-Host "=== 逐轮明细（ms）===" -ForegroundColor Cyan
$rows | Format-Table -AutoSize

Write-Host "=== 中位数汇总（$Runs 轮）===" -ForegroundColor Cyan
$summary = [ordered]@{
    "进程拉起至logger初始化" = Get-Median ($rows.进程拉起至logger初始化ms | Where-Object { $_ })
    "进程拉起至webServe监听" = Get-Median ($rows.进程拉起至webServe监听ms | Where-Object { $_ })
    "后端TCP就绪"           = Get-Median ($rows.后端TCP就绪ms | Where-Object { $_ })
    "主窗口句柄出现(WebView2代理)" = Get-Median ($rows.主窗口句柄出现ms | Where-Object { $_ })
    "GoStartup完成"         = Get-Median ($rows.GoStartup完成ms | Where-Object { $_ })
    "工作集MB"              = Get-Median ($rows.工作集MB | Where-Object { $_ })
    "私有内存MB"            = Get-Median ($rows.私有内存MB | Where-Object { $_ })
}
$summary.GetEnumerator() | ForEach-Object { "{0,-30} {1}" -f $_.Key, $_.Value }

Write-Host ""
Write-Host "环境：Windows 11 / workbench.exe=$(Split-Path $ExePath -Leaf) / 端口 $Port"
Write-Host "口径说明：主窗口句柄出现 = WebView2 宿主窗口创建完成下界（不含首帧绘制）；"
Write-Host "          前端首屏可交互请另跑 scripts/cold-start-frontend.mjs（浏览器通道代理）。"
