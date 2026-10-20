// frontend/src/utils/assetFolder.js
// 素材文件夹树共享单点（PRD「素材管理：文件夹树与拖拽整理」PR1）：路径前缀匹配
// 与徽标递归计数的唯一实现，禁在消费组件内各自手拼（前缀边界口径漂移 = 「a」误
// 匹配「ab/」类静默过滤错误）。
//
// 路径口径：relPath / folderPath 均为相对库根目录的正斜杠路径（后端 files.path /
// AssetFolderNode.path 同一口径）；folderPath 空串 = 库根「全部素材」语义。

/**
 * 目录前缀匹配（递归过滤口径）：relPath 落在 folderPath 目录（含自身与任意深度
 * 子目录）内返回 true。folderPath 为空串 = 库根，恒匹配。
 *
 * 边界契约：按「完整段」匹配，folderPath='a' 不得命中 'ab/x.pptx'（前缀后必须
 * 恰为分隔符，防裸 startsWith 的跨目录误匹配）。
 */
export function folderPrefixMatch(relPath, folderPath) {
  if (!folderPath) return true
  return relPath === folderPath || relPath.startsWith(folderPath + '/')
}

/**
 * 徽标递归计数聚合（PRD R2 a）：按索引底册（fileEntries，ListAssetFiles 全量
 * 清单）把每个文件计入其全部祖先目录（含库根 ''）。返回 plain object（键 =
 * folderPath，值 = 递归文件数），FolderTree 按 node.path 取值（根 '' 命中内置键）。
 *
 * 口径说明：以索引底册为准（展示与画廊数据源一致），与后端 AssetFolderNode.
 * FileCount 的磁盘口径可能短暂不一致（索引滞后于磁盘，下次扫描收敛）。
 */
export function buildFolderCountMap(fileEntries) {
  const map = { '': 0 }
  for (const entry of fileEntries || []) {
    const path = entry?.path || ''
    if (!path) continue
    map[''] += 1
    const segments = path.split('/')
    segments.pop() // 去文件名，剩目录链
    let prefix = ''
    for (const seg of segments) {
      prefix = prefix ? `${prefix}/${seg}` : seg
      map[prefix] = (map[prefix] || 0) + 1
    }
  }
  return map
}

// ---- 文件拖拽移动（PRD R4，PR3）----

// 拖拽数据 MIME（自定义类型，禁 text/plain——防拖入外部应用误读与外部拖入误判）。
// 产出与解析两端唯一实现（dragstart / drop 两处消费），禁第二处手拼。
export const ASSET_FILE_DRAG_MIME = 'application/x-workbench-asset-files'

/**
 * 构造拖拽数据载荷（dragstart 消费）：fileIDs = 参与移动的文件记录 ID 列表
 * （files.id，后端 MoveAssetFiles 入参口径）。
 */
export function buildAssetDragPayload(fileIds) {
  return JSON.stringify({ fileIds })
}

/**
 * 解析拖拽数据载荷（drop 消费）：返回 fileIds 数组；MIME 不匹配 / 非法 JSON /
 * 空列表返回空数组（调用方静默忽略——非本面板来源的拖拽不入分派）。
 */
export function parseAssetDragPayload(dataTransfer) {
  try {
    const raw = dataTransfer?.getData(ASSET_FILE_DRAG_MIME)
    if (!raw) return []
    const parsed = JSON.parse(raw)
    const ids = Array.isArray(parsed?.fileIds) ? parsed.fileIds : []
    return ids.filter(id => Number.isFinite(id) && id > 0)
  } catch {
    return []
  }
}

// 目录拖拽载荷（PRD R5/D，PR4）：与文件载荷 MIME 区分（同节点可作两类 drop 落点，
// 互不干扰）。载荷 = { folderPath }（源目录相对路径，'' = 根不可拖由组件侧拦截）。
export const ASSET_FOLDER_DRAG_MIME = 'application/x-workbench-asset-folders'

export function buildFolderDragPayload(folderPath) {
  return JSON.stringify({ folderPath })
}

export function parseFolderDragPayload(dataTransfer) {
  try {
    const raw = dataTransfer?.getData(ASSET_FOLDER_DRAG_MIME)
    if (!raw) return ''
    const parsed = JSON.parse(raw)
    return typeof parsed?.folderPath === 'string' ? parsed.folderPath : ''
  } catch {
    return ''
  }
}
