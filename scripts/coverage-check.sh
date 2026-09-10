#!/usr/bin/env bash
# 后端测试覆盖率门禁脚本
#
# 阈值（分层）：
#   - model / server ≥ 80%   业务核心，纯逻辑/文件 IO
#   - service      ≥ 76%   业务密集，含 RunStage/pumpOutput/pty/网络下载等系统调用核心，无法单测
#   - util         ≥ 40%   排除 pty_windows.go 系统调用文件后计算
#   - workbench 主包不设门禁：app_*.go 是转发 service 的薄包装，service 已测则重复验证价值低
#
# 运行：bash scripts/coverage-check.sh
# 超阈值 exit 1，打印各包实际值与目标。

set -u

cd "$(dirname "$0")/.."

echo "=== 后端测试覆盖率门禁 ==="

# 1. 生成覆盖率 profile
if ! go test ./... -coverprofile=coverage.out >/dev/null 2>&1; then
    echo "FAIL: go test 失败，先修复测试再检查覆盖率"
    exit 1
fi

# 阈值
MODEL_MIN=80
SERVER_MIN=80
SERVICE_MIN=76
UTIL_MIN=40

# calc_cov <包路径前缀> [排除文件子串]
# 从 coverprofile 按语句加权计算覆盖率百分比。
# coverprofile 行格式：workbench/pkg/file.go:start.col,end.col numStmts count
calc_cov() {
    local prefix="$1"
    local exclude="$2"
    awk -v prefix="$prefix" -v exclude="$exclude" '
    BEGIN { total=0; covered=0 }
    /^workbench/ {
        file=$1
        sub(/:[0-9].*/, "", file)   # 去掉 :start.col,end.col，保留模块相对路径
        if (file !~ "^" prefix) next
        if (exclude != "" && index(file, exclude) > 0) next
        total += $2
        if ($3+0 > 0) covered += $2
    }
    END {
        if (total == 0) print 0
        else printf "%.1f", covered * 100 / total
    }
    ' coverage.out
}

fail=0

# check <显示名> <包前缀> <阈值> [排除文件]
check() {
    local name="$1"
    local prefix="$2"
    local min="$3"
    local exclude="${4:-}"
    local cov
    cov=$(calc_cov "$prefix" "$exclude")
    if awk "BEGIN {exit !($cov < $min)}"; then
        echo "FAIL  $name: ${cov}% < ${min}%"
        fail=1
    else
        echo "PASS  $name: ${cov}% >= ${min}%"
    fi
}

check "model"             "workbench/model/"   "$MODEL_MIN"
check "server"            "workbench/server/"  "$SERVER_MIN"
check "service"           "workbench/service/" "$SERVICE_MIN"
check "util(排除pty_windows.go)" "workbench/util/" "$UTIL_MIN" "pty_windows.go"

echo "=== 门禁结束（fail=$fail）==="
# 清理临时文件
rm -f coverage.out
exit $fail
