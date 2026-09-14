# Windows 系统剪贴板双向互通 实现计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 通过 Win32 API 实现 CF_HDROP 格式的系统剪贴板读写，使应用内复制/剪切可在资源管理器粘贴，反之亦然，并支持 Ctrl+C/X/V 快捷键。

**Architecture:** 后端新增 `util/clipboard_windows.go` 封装 Win32 剪贴板 API（DROPFILES 结构体 + Preferred DropEffect），通过 service 层暴露给 Wails 绑定；前端在现有 handleCopy/handleCut 中追加系统剪贴板写入，在 handlePaste 中增加系统剪贴板回退读取。

**Tech Stack:** Go syscall (Win32 API: user32.dll, kernel32.dll), Vue 3, Wails v2

---

### Task 1: 后端 Win32 剪贴板 API 封装

**Files:**
- Create: `util/clipboard_windows.go`

**Step 1: 创建 `util/clipboard_windows.go`**

```go
//go:build windows

package util

import (
	"encoding/binary"
	"errors"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

const (
	cfHDrop        = 15
	gmemMoveable   = 0x0002
	dropEffectCopy = 1
	dropEffectMove = 2
	dropFilesSize  = 20 // DROPFILES 结构体大小
)

var (
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	user32   = syscall.NewLazyDLL("user32.dll")

	procOpenClipboard              = user32.NewProc("OpenClipboard")
	procCloseClipboard             = user32.NewProc("CloseClipboard")
	procEmptyClipboard             = user32.NewProc("EmptyClipboard")
	procSetClipboardData           = user32.NewProc("SetClipboardData")
	procGetClipboardData           = user32.NewProc("GetClipboardData")
	procIsClipboardFormatAvailable = user32.NewProc("IsClipboardFormatAvailable")
	procRegisterClipboardFormatW   = user32.NewProc("RegisterClipboardFormatW")
	procGlobalAlloc                = kernel32.NewProc("GlobalAlloc")
	procGlobalLock                 = kernel32.NewProc("GlobalLock")
	procGlobalUnlock               = kernel32.NewProc("GlobalUnlock")
	procGlobalSize                 = kernel32.NewProc("GlobalSize")
)

// WriteClipboardFiles 将文件路径写入系统剪贴板（CF_HDROP 格式）
// isCut=true 时写入 Preferred DropEffect 标记剪切操作
func WriteClipboardFiles(paths []string, isCut bool) error {
	if len(paths) == 0 {
		return errors.New("no paths provided")
	}

	// 计算缓冲区大小：DROPFILES 头 + UTF-16 路径 + 终止符
	totalSize := uintptr(dropFilesSize)
	for _, p := range paths {
		totalSize += uintptr((len(utf16.Encode([]rune(p))) + 1) * 2)
	}
	totalSize += 2 // 列表末尾额外 null

	// 分配全局内存
	hMem, _, _ := procGlobalAlloc.Call(gmemMoveable, totalSize)
	if hMem == 0 {
		return errors.New("GlobalAlloc failed")
	}

	ptr, _, _ := procGlobalLock.Call(hMem)
	if ptr == 0 {
		return errors.New("GlobalLock failed")
	}

	buf := unsafe.Slice((*byte)(unsafe.Pointer(ptr)), totalSize)

	// 写入 DROPFILES 头
	// pFiles=20(偏移), pt=(0,0), fNC=0, fWide=1(Unicode)
	binary.LittleEndian.PutUint32(buf[0:4], dropFilesSize)
	// buf[4:12] = POINT (0)
	// buf[12:16] = fNC (0)
	binary.LittleEndian.PutUint32(buf[16:20], 1) // fWide=1

	// 写入 UTF-16 文件路径
	offset := dropFilesSize
	for _, p := range paths {
		encoded := utf16.Encode([]rune(p))
		for _, c := range encoded {
			binary.LittleEndian.PutUint16(buf[offset:], uint16(c))
			offset += 2
		}
		binary.LittleEndian.PutUint16(buf[offset:], 0) // 路径 null 终止符
		offset += 2
	}
	binary.LittleEndian.PutUint16(buf[offset:], 0) // 列表终止符

	procGlobalUnlock.Call(hMem)

	// 打开剪贴板并写入
	r, _, _ := procOpenClipboard.Call(0)
	if r == 0 {
		return errors.New("OpenClipboard failed")
	}
	defer procCloseClipboard.Call()

	procEmptyClipboard.Call()

	r, _, _ = procSetClipboardData.Call(cfHDrop, hMem)
	if r == 0 {
		return errors.New("SetClipboardData failed")
	}

	// 剪切模式：写入 Preferred DropEffect
	if isCut {
		fmtName, _ := syscall.UTF16PtrFromString("Preferred DropEffect")
		cfDropEffect, _, _ := procRegisterClipboardFormatW.Call(uintptr(unsafe.Pointer(fmtName)))

		hEffect, _, _ := procGlobalAlloc.Call(gmemMoveable, 4)
		if hEffect != 0 {
			effectPtr, _, _ := procGlobalLock.Call(hEffect)
			if effectPtr != 0 {
				effectBuf := unsafe.Slice((*byte)(unsafe.Pointer(effectPtr)), 4)
				binary.LittleEndian.PutUint32(effectBuf, dropEffectMove)
				procGlobalUnlock.Call(hEffect)
				procSetClipboardData.Call(cfDropEffect, hEffect)
			}
		}
	}

	return nil
}

// ReadClipboardFiles 读取系统剪贴板中的文件路径列表
// 返回文件路径数组和是否为剪切操作
func ReadClipboardFiles() (paths []string, isCut bool, err error) {
	r, _, _ := procOpenClipboard.Call(0)
	if r == 0 {
		return nil, false, errors.New("OpenClipboard failed")
	}
	defer procCloseClipboard.Call()

	// 检查 CF_HDROP 是否可用
	available, _, _ := procIsClipboardFormatAvailable.Call(cfHDrop)
	if available == 0 {
		return nil, false, nil
	}

	hData, _, _ := procGetClipboardData.Call(cfHDrop)
	if hData == 0 {
		return nil, false, errors.New("GetClipboardData failed")
	}

	size, _, _ := procGlobalSize.Call(hData)
	if size == 0 {
		return nil, false, errors.New("GlobalSize failed")
	}

	ptr, _, _ := procGlobalLock.Call(hData)
	if ptr == 0 {
		return nil, false, errors.New("GlobalLock failed")
	}
	defer procGlobalUnlock.Call(hData)

	buf := unsafe.Slice((*byte)(unsafe.Pointer(ptr)), size)

	// 读取 DROPFILES 头
	pFiles := binary.LittleEndian.Uint32(buf[0:4])
	fWide := binary.LittleEndian.Uint32(buf[16:20])

	if fWide == 0 {
		return nil, false, errors.New("ANSI format not supported")
	}

	// 读取 UTF-16 路径
	offset := uintptr(pFiles)
	for offset+1 < uintptr(len(buf)) {
		var chars []uint16
		for offset+1 < uintptr(len(buf)) {
			c := binary.LittleEndian.Uint16(buf[offset:])
			offset += 2
			if c == 0 {
				break
			}
			chars = append(chars, c)
		}
		if len(chars) == 0 {
			break
		}
		paths = append(paths, string(utf16.Decode(chars)))
	}

	// 检查 Preferred DropEffect
	fmtName, _ := syscall.UTF16PtrFromString("Preferred DropEffect")
	cfDropEffect, _, _ := procRegisterClipboardFormatW.Call(uintptr(unsafe.Pointer(fmtName)))

	hEffect, _, _ := procGetClipboardData.Call(cfDropEffect)
	if hEffect != 0 {
		effectPtr, _, _ := procGlobalLock.Call(hEffect)
		if effectPtr != 0 {
			effectBuf := unsafe.Slice((*byte)(unsafe.Pointer(effectPtr)), 4)
			effect := binary.LittleEndian.Uint32(effectBuf)
			procGlobalUnlock.Call(hEffect)
			if effect == dropEffectMove {
				isCut = true
			}
		}
	}

	return paths, isCut, nil
}
```

**Step 2: 确认编译通过**

Run: `cd workbench && go build ./...`
Expected: 无错误

---

### Task 2: 后端 Service 层 + Wails 绑定

**Files:**
- Create: `service/clipboard.go`
- Modify: `app.go`（末尾追加，import 添加 `"encoding/json"`）

**Step 1: 创建 `service/clipboard.go`**

```go
package service

import "workbench/util"

// CopyToSystemClipboard 写入系统剪贴板（复制模式）
func (s *FileOperationService) CopyToSystemClipboard(paths []string) error {
	return util.WriteClipboardFiles(paths, false)
}

// CutToSystemClipboard 写入系统剪贴板（剪切模式）
func (s *FileOperationService) CutToSystemClipboard(paths []string) error {
	return util.WriteClipboardFiles(paths, true)
}

// ReadFromSystemClipboard 读取系统剪贴板文件列表
func (s *FileOperationService) ReadFromSystemClipboard() ([]string, bool, error) {
	return util.ReadClipboardFiles()
}
```

**Step 2: 在 `app.go` import 中添加 `"encoding/json"`**

当前 import 块：
```go
import (
	"context"
	"fmt"
	"path/filepath"
	...
```

添加 `"encoding/json"`：
```go
import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	...
```

**Step 3: 在 `app.go` 末尾追加 3 个绑定方法**

```go
// CopyToSystemClipboard 写入系统剪贴板（复制模式）
func (a *App) CopyToSystemClipboard(path string) string {
	err := a.fileOpSvc.CopyToSystemClipboard([]string{path})
	if err != nil {
		println("Error:", err.Error())
		return "错误: " + err.Error()
	}
	return ""
}

// CutToSystemClipboard 写入系统剪贴板（剪切模式）
func (a *App) CutToSystemClipboard(path string) string {
	err := a.fileOpSvc.CutToSystemClipboard([]string{path})
	if err != nil {
		println("Error:", err.Error())
		return "错误: " + err.Error()
	}
	return ""
}

// ReadFromSystemClipboard 读取系统剪贴板文件列表
func (a *App) ReadFromSystemClipboard() string {
	paths, isCut, err := a.fileOpSvc.ReadFromSystemClipboard()
	if err != nil {
		println("Error:", err.Error())
		return ""
	}
	if len(paths) == 0 {
		return ""
	}
	data, _ := json.Marshal(map[string]interface{}{
		"paths": paths,
		"isCut": isCut,
	})
	return string(data)
}
```

**Step 4: 确认编译通过**

Run: `cd workbench && go build ./...`
Expected: 无错误

**Step 5: 运行全部测试**

Run: `cd workbench && go test ./... -count=1`
Expected: 全部 PASS

**Step 6: 提交后端变更**

```bash
git add util/clipboard_windows.go service/clipboard.go app.go
git commit -m "feat: 新增 Win32 剪贴板 API 封装，支持 CF_HDROP 格式读写"
```

---

### Task 3: 前端集成 — 系统剪贴板 + 快捷键

**Files:**
- Modify: `frontend/src/views/Home.vue`

**Step 1: 导入新增 Wails 绑定**

当前 import：
```js
import {
  GetDirectories,
  ScanAndPullRepos,
  DeleteFile,
  CopyItem,
  MoveItem
} from '../../wailsjs/go/main/App'
```

改为：
```js
import {
  GetDirectories,
  ScanAndPullRepos,
  DeleteFile,
  CopyItem,
  MoveItem,
  CopyToSystemClipboard,
  CutToSystemClipboard,
  ReadFromSystemClipboard
} from '../../wailsjs/go/main/App'
```

**Step 2: 添加 onBeforeUnmount 到 import**

当前：
```js
import { ref, reactive, onMounted, watch } from 'vue'
```

改为：
```js
import { ref, reactive, onMounted, onBeforeUnmount, watch } from 'vue'
```

**Step 3: 修改 handleCopy — 追加系统剪贴板写入**

当前：
```js
const handleCopy = (data) => {
  clipboard.mode = 'copy'
  clipboard.sourcePath = data.path
  clipboard.sourceName = data.name
  clipboard.sourceType = data.type
  ElMessage.success(`${data.path.replaceAll('\\', '/')} 复制成功`)
}
```

改为：
```js
const handleCopy = async (data) => {
  clipboard.mode = 'copy'
  clipboard.sourcePath = data.path
  clipboard.sourceName = data.name
  clipboard.sourceType = data.type
  ElMessage.success(`${data.path.replaceAll('\\', '/')} 复制成功`)
  CopyToSystemClipboard(data.path).catch(() => {})
}
```

**Step 4: 修改 handleCut — 追加系统剪贴板写入**

当前：
```js
const handleCut = (data) => {
  clipboard.mode = 'cut'
  clipboard.sourcePath = data.path
  clipboard.sourceName = data.name
  clipboard.sourceType = data.type
  ElMessage.success(`${data.path.replaceAll('\\', '/')} 剪切成功`)
}
```

改为：
```js
const handleCut = async (data) => {
  clipboard.mode = 'cut'
  clipboard.sourcePath = data.path
  clipboard.sourceName = data.name
  clipboard.sourceType = data.type
  ElMessage.success(`${data.path.replaceAll('\\', '/')} 剪切成功`)
  CutToSystemClipboard(data.path).catch(() => {})
}
```

**Step 5: 修改 handlePaste — 增加系统剪贴板回退**

当前：
```js
const handlePaste = async (targetData) => {
  if (!clipboard.mode || !clipboard.sourcePath) return
```

改为：
```js
const handlePaste = async (targetData) => {
  // 应用内剪贴板优先
  if (clipboard.mode && clipboard.sourcePath) {
    await handleAppClipboardPaste(targetData)
    return
  }

  // 回退到系统剪贴板
  await handleSystemClipboardPaste(targetData)
}

const handleAppClipboardPaste = async (targetData) => {
  const targetDir = resolveTargetDir(targetData)
  if (!targetDir) return

  try {
    let result
    if (clipboard.mode === 'copy') {
      result = await CopyItem(clipboard.sourcePath, targetDir)
    } else {
      result = await MoveItem(clipboard.sourcePath, targetDir)
    }

    if (result && !result.startsWith('错误')) {
      ElMessage.success(`粘贴成功：${result.replaceAll('\\', '/')}`)
      fileTreePanelRef.value?.refreshNode(targetDir)

      if (clipboard.mode === 'cut') {
        const srcLastSep = Math.max(clipboard.sourcePath.lastIndexOf('\\'), clipboard.sourcePath.lastIndexOf('/'))
        const srcParent = srcLastSep > 0 ? clipboard.sourcePath.substring(0, srcLastSep) : ''
        if (srcParent && srcParent !== targetDir) {
          fileTreePanelRef.value?.refreshNode(srcParent)
        }
        clearClipboard()
      }
    } else {
      ElMessage.error(result || '粘贴失败')
    }
  } catch (error) {
    ElMessage.error('粘贴失败: ' + (error.message || String(error)))
  }
}

const handleSystemClipboardPaste = async (targetData) => {
  const targetDir = resolveTargetDir(targetData)
  if (!targetDir) return

  try {
    const result = await ReadFromSystemClipboard()
    if (!result) {
      ElMessage.info('剪贴板中没有可粘贴的内容')
      return
    }

    const clipData = JSON.parse(result)
    const paths = clipData.paths || []
    const isCut = clipData.isCut || false

    if (paths.length === 0) {
      ElMessage.info('剪贴板中没有可粘贴的内容')
      return
    }

    let successCount = 0
    for (const srcPath of paths) {
      let res
      if (isCut) {
        res = await MoveItem(srcPath, targetDir)
      } else {
        res = await CopyItem(srcPath, targetDir)
      }
      if (res && !res.startsWith('错误')) {
        successCount++
      }
    }

    if (successCount > 0) {
      ElMessage.success(`粘贴成功：${successCount} 个项目`)
      fileTreePanelRef.value?.refreshNode(targetDir)
    } else {
      ElMessage.error('粘贴失败')
    }
  } catch (error) {
    // 静默降级
  }
}
```

注意：需要删除原来的 `handlePaste` 整个函数体，替换为上述新的 `handlePaste` + `handleAppClipboardPaste` + `handleSystemClipboardPaste` 三个函数。

**Step 6: 添加键盘快捷键监听**

在 `clearClipboard` 函数之前添加：

```js
// ---- 键盘快捷键 ----
const handleGlobalKeydown = (e) => {
  if (!selectedNode.value) return
  if (!(e.ctrlKey || e.metaKey)) return

  if (e.key === 'c') {
    e.preventDefault()
    handleCopy(selectedNode.value)
  } else if (e.key === 'x') {
    e.preventDefault()
    handleCut(selectedNode.value)
  } else if (e.key === 'v') {
    e.preventDefault()
    handlePaste(selectedNode.value)
  }
}
```

修改 `onMounted`，添加键盘事件监听：

```js
onMounted(() => {
  loadDirectories()
  document.addEventListener('keydown', handleGlobalKeydown)
})
```

在 `onBeforeUnmount` 生命周期（在 watch 之后、`</script>` 之前）添加清理：

```js
onBeforeUnmount(() => {
  document.removeEventListener('keydown', handleGlobalKeydown)
})
```

**Step 7: 提交前端变更**

```bash
git add frontend/src/views/Home.vue
git commit -m "feat: 前端集成系统剪贴板读写 + Ctrl+C/X/V 快捷键支持"
```

---

### Task 4: 生成 Wails 绑定 + 集成验证

**Step 1: 重新生成 Wails 前端绑定**

Run: `cd workbench && wails generate module`

确认 `frontend/wailsjs/go/main/App.js` 中包含 `CopyToSystemClipboard`、`CutToSystemClipboard`、`ReadFromSystemClipboard`。
确认 `frontend/wailsjs/go/main/App.d.ts` 中包含对应类型声明。

**Step 2: 启动开发环境验证**

Run: `cd workbench && wails dev`

验证清单：
- [ ] 应用内复制文件 → 打开资源管理器 → Ctrl+V 粘贴，文件被复制
- [ ] 应用内剪切文件 → 资源管理器 Ctrl+V 粘贴，文件被移动（源文件消失）
- [ ] 资源管理器复制文件 → 应用内右键粘贴，文件出现在目标目录
- [ ] 资源管理器剪切文件 → 应用内粘贴，文件被移动
- [ ] 应用内剪贴板有内容时粘贴，优先使用应用内剪贴板
- [ ] 应用内剪贴板为空、系统剪贴板有文件时粘贴，使用系统剪贴板
- [ ] 两者都为空时粘贴，提示"剪贴板中没有可粘贴的内容"
- [ ] Ctrl+C 复制选中节点
- [ ] Ctrl+X 剪切选中节点
- [ ] Ctrl+V 粘贴到选中节点

**Step 3: 最终提交**

```bash
git add docs/plans/2026-05-14-clipboard-integration-design.md docs/plans/2026-05-14-clipboard-integration-plan.md
git commit -m "docs: 新增系统剪贴板互通设计文档和实现计划"
```
