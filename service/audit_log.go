package service

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// 审计事件名（JSONL 行的 event 字段取值）。auth_* 由 server 包认证点回调
// （server.AuthEvent.Event）透传至此落盘；token_* 由 WebTokenService 生命周期
// 操作直接产生。
const (
	AuditEventAuthSuccess  = "auth_success"  // 认证成功（HTTP requireToken / WS 握手认证点）
	AuditEventAuthFailure  = "auth_failure"  // 认证失败（同上两处认证点）
	AuditEventTokenCreate  = "token_create"  // 新建 token
	AuditEventTokenRevoke  = "token_revoke"  // 撤销 token（含轮换编排的逐条撤旧）
	AuditEventTokenRotate  = "token_rotate"  // 轮换编排（新建 + 撤旧）完成（App 层编排事件）
	AuditEventTokenMigrate = "token_migrate" // 存量单 token 迁移为多 token 首条记录 / 初始引导
)

// webAuditMaxBytes 审计日志单文件大小上限：超过即轮转为 <path>.1（覆盖旧 .1），
// 继续向新文件追加。5MB 对认证事件级频度足够（约 3 万行），轮转保留一份对
// 单用户排查定位足够；更久历史靠外部归档，不做多份轮转链。
const webAuditMaxBytes int64 = 5 * 1024 * 1024

// AuditRecord 审计日志单行（JSONL）。禁落 token 明文与完整 URL（?token= 泄露面，
// 对齐 server 层「不落完整 URL」既有约定）。
type AuditRecord struct {
	Ts         int64  `json:"ts"`                   // 事件时间（unix 秒）
	Event      string `json:"event"`                // 事件名，见 AuditEvent* 常量
	TokenID    string `json:"tokenId,omitempty"`    // 关联 token 记录 ID（认证失败时为空）
	Name       string `json:"name,omitempty"`       // token 名称（token 生命周期事件携带）
	RemoteAddr string `json:"remoteAddr,omitempty"` // 来源地址（r.RemoteAddr；直连形态不做 XFF 信任，PRD R2）
	UserAgent  string `json:"userAgent,omitempty"`  // 客户端 User-Agent
}

// AuditLogger 认证与 token 生命周期审计日志（JSONL append-only，data/web_audit.log）。
//
// append 语义独立实现，不走 util.SaveJSON 原子写——原子写针对「整体覆盖型 JSON
// 快照」的半截文件问题，append-only 日志以 O_APPEND 追加单行（O_APPEND 下单次
// write 为原子 append），属 data-atomic-write 规范的「非 JSON 写入」豁免口径，
// 主数据文件截断写禁令不适用于本旁路日志。
//
// 写入为旁路：打开/写入失败仅 slog.Warn 降级，不阻塞认证与 token 管理主链路
// （审计是旁路观测面，非正确性依赖）。全部事件写入收敛单方法 Log：server 认证
// 回调（OnAuth）与 service token 生命周期操作都经此处落盘。
type AuditLogger struct {
	mu       sync.Mutex
	path     string
	maxBytes int64 // 单文件大小上限（测试注入小值驱动轮转；生产 webAuditMaxBytes）
}

// NewAuditLogger 构造审计日志器（path 如 data/web_audit.log）。
func NewAuditLogger(path string) *AuditLogger {
	return &AuditLogger{path: path, maxBytes: webAuditMaxBytes}
}

// Log 追加一条审计事件（单写锁串行，并发安全）。
func (a *AuditLogger) Log(event, tokenID, name, remoteAddr, userAgent string) {
	rec := AuditRecord{
		Ts:         time.Now().Unix(),
		Event:      event,
		TokenID:    tokenID,
		Name:       name,
		RemoteAddr: remoteAddr,
		UserAgent:  userAgent,
	}
	line, err := json.Marshal(rec)
	if err != nil {
		// 全为基本类型字段不会失败，防御保留
		slog.Error("marshal audit record failed", "event", event, "err", err)
		return
	}
	line = append(line, '\n')

	a.mu.Lock()
	defer a.mu.Unlock()
	a.rotateLocked(int64(len(line)))
	// 父目录不存在时自动创建（data/ 目录约定；裸目录创建失败由下方 open 兜底告警）
	if err := os.MkdirAll(filepath.Dir(a.path), 0o755); err != nil {
		slog.Warn("create audit log dir failed", "path", a.path, "err", err)
		return
	}
	f, err := os.OpenFile(a.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		slog.Warn("open audit log failed", "path", a.path, "err", err)
		return
	}
	if _, err := f.Write(line); err != nil {
		slog.Warn("write audit log failed", "path", a.path, "err", err)
	}
	_ = f.Close()
}

// rotateLocked 持锁轮转：当前文件追加本行后将超过上限时，rename 为 <path>.1
// （Go os.Rename 在 Windows 走 MoveFileEx REPLACE_EXISTING，覆盖旧 .1），后续
// 追加落新文件。损坏行不处理（读侧按行解析，坏行跳过属读方容错）。
func (a *AuditLogger) rotateLocked(incoming int64) {
	info, err := os.Stat(a.path)
	if err != nil || info.Size()+incoming <= a.maxBytes {
		return
	}
	if err := os.Rename(a.path, a.path+".1"); err != nil {
		// 轮转失败不阻塞本行写入：继续向当前文件追加（超限容忍，下条再试轮转）
		slog.Warn("rotate audit log failed", "path", a.path, "err", err)
	}
}
