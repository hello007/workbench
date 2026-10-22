/**
 * unified diff 文本解析（纯函数，与组件解耦）。
 *
 * 原实现内嵌于 FileDiffDialog.vue，分支对比（batch 1）抽出为共享 util：
 * BranchCompareDiffViewer 复用同一套双栏解析语义，FileDiffDialog 改为引用本模块，
 * 行为逐字节保持不变（FileDiffDialog.spec.js 不回归为验收线）。
 */

/**
 * 解析 unified diff 文本为左右两栏行数组。
 * - 以 `@@` hunk 头为分隔，记录左右起始行号。
 * - 空格行：左右都加（行号递增）。
 * - `-` 行：进左栏，右栏补空占位。
 * - `+` 行：进右栏，左栏补空占位。
 * - 行号取 hunk 头中的真实值，便于对照。
 */
export const parseDiff = (text) => {
  const leftLines = []
  const rightLines = []
  if (!text) return { leftLines, rightLines }

  let leftNo = 0
  let rightNo = 0
  const lines = text.split(/\r?\n/)

  for (const raw of lines) {
    if (!raw) continue
    // hunk 头：@@ -lStart,lLen +rStart,rLen @@
    if (raw.startsWith('@@')) {
      const m = raw.match(/@@\s+-(\d+)(?:,\d+)?\s+\+(\d+)(?:,\d+)?\s+@@/)
      if (m) {
        leftNo = parseInt(m[1], 10)
        rightNo = parseInt(m[2], 10)
      }
      leftLines.push({ kind: 'hunk', no: '', text: raw })
      rightLines.push({ kind: 'hunk', no: '', text: raw })
      continue
    }
    // 普通行首字符为 diff 标记
    const tag = raw[0]
    const content = raw.slice(1)
    if (tag === '\\') {
      // "\ No newline at end of file" 提示，挂到对应侧最后一行（这里直接忽略，避免对齐复杂度）
      continue
    }
    // 跳过 diff 文件头行：diff --git / index / +++ / ---，避免污染双栏
    // （+++ 与 --- 首字符为 + / -，若不排除会被误当 add / del 行）
    if (raw.startsWith('diff --git') || raw.startsWith('index ') ||
        raw.startsWith('+++') || raw.startsWith('---')) {
      continue
    }
    if (tag === ' ') {
      leftLines.push({ kind: 'context', no: String(leftNo++), text: content })
      rightLines.push({ kind: 'context', no: String(rightNo++), text: content })
    } else if (tag === '-') {
      leftLines.push({ kind: 'del', no: String(leftNo++), text: content })
      rightLines.push({ kind: 'empty', no: '', text: '' })
    } else if (tag === '+') {
      rightLines.push({ kind: 'add', no: String(rightNo++), text: content })
      leftLines.push({ kind: 'empty', no: '', text: '' })
    } else {
      // 其他行（如 "diff --git" "index .." "+++" "---" 文件头）忽略，避免污染双栏
    }
  }

  return { leftLines, rightLines }
}

/**
 * 解析 range diff（多文件 unified diff）为按文件分组的双栏结构。
 * git diff <base> <head> 输出形如：
 *   diff --git a/path b/path
 *   index ...
 *   --- a/path
 *   +++ b/path
 *   @@ -l,l +r,r @@
 *   ...
 * 按 `diff --git` 头拆段，每段取 b/ 路径为文件名，段内行喂 parseDiff。
 * 二进制段（含 "Binary files ... differ"）单独标记，不进双栏。
 */
export const parseRangeDiff = (text) => {
  const groups = []
  if (!text) return groups

  // 按 "diff --git" 拆段，首段可能为空（文本以 diff --git 开头时）
  const segments = text.split(/^diff --git /m)
  for (const seg of segments) {
    if (!seg.trim()) continue
    // seg 形如 "a/path b/path\nindex ...\n--- a/path\n+++ b/path\n@@ ..."
    const headerMatch = seg.match(/^a\/\S+ b\/(.+?)(?:\r?\n)/)
    const file = headerMatch ? headerMatch[1] : '(未知文件)'
    const segText = 'diff --git ' + seg
    if (/^Binary files /m.test(segText)) {
      groups.push({ file, binary: true, left: [], right: [] })
      continue
    }
    const { leftLines, rightLines } = parseDiff(segText)
    if (leftLines.length === 0 && rightLines.length === 0) continue
    groups.push({ file, binary: false, left: leftLines, right: rightLines })
  }
  return groups
}
