package service

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// VaultEntry obsidian.json 中单个 vault 条目。
// 字段含义：path=vault 根绝对路径；ts=注册/最后打开毫秒时间戳；open=是否最后打开（可选）。
type VaultEntry struct {
	Path string `json:"path"`
	Ts   int64  `json:"ts"`
	Open bool   `json:"open,omitempty"`
}

// obsidianConfig obsidian.json 顶层结构。
// 通过自定义 JSON 编解码保留未知顶层字段（如 updateDisabled），避免回写时丢失：
//   - UnmarshalJSON：解析为 map[string]json.RawMessage，单独处理 vaults 键，其余原样保留到 Rest。
//   - MarshalJSON：将 vaults 与 Rest 中保留的未知顶层字段一并输出。
type obsidianConfig struct {
	Vaults map[string]VaultEntry      `json:"vaults"`
	Rest   map[string]json.RawMessage `json:"-"` // 保留未知顶层字段，回写时原样输出
}

// UnmarshalJSON 解析 obsidian.json：单独处理 vaults 键，其余键原样保留到 Rest。
// 防御性解析：vaults 值非对象时返回错误；其余顶层字段即使值异常也不影响 vaults 提取（作为 RawMessage 原样保留）。
func (c *obsidianConfig) UnmarshalJSON(data []byte) error {
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	c.Vaults = nil
	if vRaw, ok := raw["vaults"]; ok {
		if err := json.Unmarshal(vRaw, &c.Vaults); err != nil {
			return err
		}
		delete(raw, "vaults")
	}
	c.Rest = raw
	return nil
}

// MarshalJSON 序列化 obsidian.json：vaults 与 Rest 中保留的未知顶层字段一并输出。
// 临时文件写入时调用，确保 updateDisabled 等字段不丢失。
func (c obsidianConfig) MarshalJSON() ([]byte, error) {
	out := map[string]json.RawMessage{}
	if c.Rest != nil {
		for k, v := range c.Rest {
			out[k] = v
		}
	}
	vaultsData, err := json.Marshal(c.Vaults)
	if err != nil {
		return nil, err
	}
	out["vaults"] = vaultsData
	return json.Marshal(out)
}

// loadVaultsFromFile 解析指定路径的 obsidian.json 为 vault 映射。
// 防御性解析：文件不存在/结构异常时返回错误，不 panic；
// 未知顶层字段与未知 vault 字段忽略；vaults 为 nil 时返回空表而非 nil。
// 抽出 path 参数便于单测，生产路径由 loadObsidianVaults 注入。
func loadVaultsFromFile(path string) (map[string]VaultEntry, error) {
	if path == "" {
		return nil, errors.New("当前平台不支持读取 Obsidian vault 注册表")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg obsidianConfig
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, err
	}
	if cfg.Vaults == nil {
		return map[string]VaultEntry{}, nil
	}
	return cfg.Vaults, nil
}

// loadObsidianVaults 读取并解析 Obsidian vault 注册表（Windows: %APPDATA%\obsidian\obsidian.json）。
// 注册表路径由 obsidianConfigPath() 按平台提供，非 Windows 平台返回空串->此处返回错误（触发降级）。
func loadObsidianVaults() (map[string]VaultEntry, error) {
	return loadVaultsFromFile(obsidianConfigPath())
}

// resolvePath 解析符号链接并 Clean，失败时回退到 Clean。
// 用于归属判断前规范化 vault 路径与目标路径，规避符号链接导致的前缀比较失败。
func resolvePath(p string) string {
	if rp, err := filepath.EvalSymlinks(p); err == nil {
		return filepath.Clean(rp)
	}
	return filepath.Clean(p)
}

// isAncestorOrEqual 判断 parent 是否等于 child 或为 child 的祖先（完整路径段匹配）。
// Windows 大小写不敏感；比较前统一转 slash 规避 \ vs / 差异。
// 必须以 "parent/" 前缀匹配，避免 C:\Vault 误匹配 C:\VaultChild（前缀字符串相同但非完整段）。
func isAncestorOrEqual(parent, child string) bool {
	ps := filepath.ToSlash(filepath.Clean(parent))
	cs := filepath.ToSlash(filepath.Clean(child))
	if strings.EqualFold(ps, cs) {
		return true
	}
	// child 必须以 "parent/" 开头（大小写不敏感），确保完整路径段
	return strings.HasPrefix(strings.ToLower(cs), strings.ToLower(ps)+"/")
}

// findVaultForPath 复刻 Obsidian「最具体包含 vault」语义：
// 遍历已注册 vault，找 Path 等于 absPath 或为 absPath 祖先且路径最长（嵌套最深）者。
// ok=false 表示无 vault 包含 absPath（将触发 Obsidian 的 Vault not found）。
func findVaultForPath(vaults map[string]VaultEntry, absPath string) (vaultPath string, ok bool) {
	target := resolvePath(absPath)
	bestLen := -1
	for _, v := range vaults {
		vp := resolvePath(v.Path)
		if !isAncestorOrEqual(vp, target) {
			continue
		}
		// 最具体：取路径最长者（嵌套最深）
		if len(vp) > bestLen {
			bestLen = len(vp)
			vaultPath, ok = vp, true
		}
	}
	return
}

// loadFullConfig 读取完整 obsidian.json（保留未知顶层字段，供回写不丢失 updateDisabled 等）。
// 防御性解析：空路径/文件不存在/非法 JSON 返回错误；nil vaults 返回空表（便于调用方追加条目）。
// 与 loadVaultsFromFile 的区别：后者仅返回 vaults 映射（丢弃未知字段），用于只读归属判断；
// 本函数返回完整结构，用于自动注册的读-改-写场景。
func loadFullConfig(path string) (*obsidianConfig, error) {
	if path == "" {
		return nil, errors.New("当前平台不支持读取 Obsidian 配置")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg obsidianConfig
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, err
	}
	if cfg.Vaults == nil {
		cfg.Vaults = map[string]VaultEntry{}
	}
	return &cfg, nil
}

// newVaultID 生成 16 位小写 hex vault ID（crypto/rand 读 8 字节），并做冲突检测。
// 与官方 Obsidian 生成格式一致（如 ef6ca3e3b524d22f）；冲突概率极低但仍检测，成本极小。
func newVaultID(existing map[string]VaultEntry) string {
	var b [8]byte
	for {
		_, _ = rand.Read(b[:])
		id := hex.EncodeToString(b[:])
		if _, dup := existing[id]; !dup {
			return id
		}
	}
}

// backupConfig 将 obsidian.json 备份到同目录 .bak.<unix> 文件，返回备份路径。
// 自动注册写入前调用，便于异常时手动恢复。
func backupConfig(cfgPath string) (string, error) {
	b, err := os.ReadFile(cfgPath)
	if err != nil {
		return "", err
	}
	bak := cfgPath + ".bak." + strconv.FormatInt(time.Now().Unix(), 10)
	return bak, os.WriteFile(bak, b, 0644)
}

// atomicWriteConfig 原子写入 obsidian.json：MarshalIndent -> 写同目录临时文件 -> os.Rename 替换。
// 同目录临时文件保证同分区，os.Rename 在 Windows 上为原子替换（覆盖目标），避免半写损坏。
// 临时文件命名带 pid，规避并发实例竞争；失败时清理临时文件。
func atomicWriteConfig(cfgPath string, cfg *obsidianConfig) error {
	tmp := cfgPath + ".tmp." + strconv.Itoa(os.Getpid())
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, cfgPath); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
