/**
 * IP 封禁可信网段白名单工具（连接管控七期，纯前端校验/提示面）。
 *
 * 后端权威口径在 service 层（isWhitelistedIP 单点 + ApplyConfig 预解析，裸 IP
 * 自动补 /32 //128）；本文件为前端侧同口径复刻，三处消费：
 *  - BanIPDialog：封禁目标 IP 的白名单命中即时提示（后端 Ban 仍兜底拒绝，
 *    E_IP_BAN_WHITELISTED——前端提示为引导，非正确性依赖）；
 *  - BanIPDialog 网段档（连接管控八期）：封禁网段输入解析（parseCidrInput）
 *    与白名单交集/覆盖预检（cidrOverlapWhitelist，后端 E_IP_BAN_CIDR_INVALID /
 *    E_IP_BAN_CIDR_WHITELISTED 权威兜底）；
 *  - SettingsPanel：白名单条目行级格式校验（后端 normalize 剔除兜底）。
 * 两口径不一致的表现仅为提示偏差（后端权威判定兜底），但维护上仍须保持同源
 * 语义：CIDR 串或裸 IP 均合法、IPv4/IPv6 双栈。
 */

/**
 * 将 IP 字符串解析为字节数组（IPv4 → 4 字节；IPv6 → 16 字节，含 IPv4 映射
 * 形态 ::ffff:a.b.c.d 展开）；非法返回 null。
 * @param {string} ip
 * @returns {number[]|null}
 */
export function ipToBytes(ip) {
  if (typeof ip !== 'string') return null
  const trimmed = ip.trim()
  if (!trimmed) return null
  if (trimmed.includes(':')) {
    return parseIPv6(trimmed)
  }
  return parseIPv4(trimmed)
}

/** 解析 IPv4 点分十进制（每段 0-255，恰 4 段）。 */
function parseIPv4(s) {
  const parts = s.split('.')
  if (parts.length !== 4) return null
  const bytes = []
  for (const p of parts) {
    if (!/^\d{1,3}$/.test(p)) return null
    const n = Number(p)
    if (n > 255) return null
    bytes.push(n)
  }
  return bytes
}

/**
 * 解析 IPv6（RFC 4291 文本形态：8 组 16 进制、"::" 压缩至多一次、末段可嵌
 * IPv4 点分十进制）。展开后恰 16 字节，否则 null。
 */
function parseIPv6(s) {
  let head = s
  let tail = []
  const dbl = s.indexOf('::')
  if (dbl !== -1) {
    // "::" 至多出现一次
    if (s.indexOf('::', dbl + 1) !== -1) return null
    const left = s.slice(0, dbl)
    const right = s.slice(dbl + 2)
    head = left ? splitIPv6Groups(left) : []
    tail = right ? splitIPv6Groups(right) : []
    if (head === null || tail === null) return null
  } else {
    head = splitIPv6Groups(s)
    if (head === null) return null
  }
  // 每组展开为 2 字节；IPv4 嵌入末段已在 splitIPv6Groups 展开为 2 组。
  // 空 groups（纯 '::'，全零未指定地址）合法——后端 net.ParseIP 同样接受，
  // 该形态是 ::/0 全网段封禁（连接管控八期）的解析前提
  const groups = [...head, ...tail]
  if (groups.length > 8) return null
  if (dbl === -1 && groups.length !== 8) return null
  const bytes = []
  for (const g of groups) {
    const v = parseInt(g, 16)
    if (Number.isNaN(v) || v < 0 || v > 0xffff) return null
    bytes.push(v >> 8, v & 0xff)
  }
  while (bytes.length < 16) bytes.push(0)
  // "::" 压缩段补零：head 组字节位于 bytes 头部、tail 组字节紧随其后
  //（[headBytes, headBytes+tailBytes) 区间），中间以 fill 个零字节填充
  if (dbl !== -1) {
    const headBytes = head.length * 2
    const tailBytes = tail.length * 2
    const fill = 16 - headBytes - tailBytes
    if (fill < 0) return null
    const out = bytes.slice(0, headBytes)
    for (let i = 0; i < fill; i++) out.push(0)
    out.push(...bytes.slice(headBytes, headBytes + tailBytes))
    return out
  }
  return bytes
}

/**
 * 按 ":" 切组：末段若为 IPv4 点分十进制则展开为 2 组 16 进制（::ffff:192.168.1.1
 * 形态）；非法返回 null。
 */
function splitIPv6Groups(s) {
  const raw = s.split(':')
  if (raw.length === 0) return null
  const groups = []
  for (let i = 0; i < raw.length; i++) {
    const seg = raw[i]
    if (seg.includes('.')) {
      // IPv4 嵌入仅允许出现在末段
      if (i !== raw.length - 1) return null
      const v4 = parseIPv4(seg)
      if (!v4) return null
      groups.push(((v4[0] << 8) | v4[1]).toString(16), ((v4[2] << 8) | v4[3]).toString(16))
      continue
    }
    if (!/^[0-9a-fA-F]{1,4}$/.test(seg)) return null
    groups.push(seg)
  }
  return groups
}

/**
 * 解析白名单条目为 { bytes, prefixLen }（CIDR 串或裸 IP；裸 IP 前缀取全位
 * /32 //128，与后端 parseIPBanWhitelistEntry 同口径）；非法返回 null。
 * @param {string} entry
 * @returns {{bytes: number[], prefixLen: number}|null}
 */
export function parseWhitelistEntry(entry) {
  if (typeof entry !== 'string') return null
  const trimmed = entry.trim()
  if (!trimmed) return null
  const slash = trimmed.lastIndexOf('/')
  let ipPart = trimmed
  let prefixLen = null
  if (slash !== -1) {
    ipPart = trimmed.slice(0, slash)
    const lenPart = trimmed.slice(slash + 1)
    if (!/^\d{1,3}$/.test(lenPart)) return null
    prefixLen = Number(lenPart)
  }
  const bytes = ipToBytes(ipPart)
  if (!bytes) return null
  const max = bytes.length * 8
  if (prefixLen === null) {
    prefixLen = max
  }
  if (prefixLen < 0 || prefixLen > max) return null
  return { bytes, prefixLen }
}

/**
 * 判定 ip 是否命中白名单（回环豁免为后端判定面收编，前端只做 CIDR 命中提示
 * ——回环提示由 BanIPDialog 入口校验承担，不走本函数）。家族不匹配（IPv4 条目
 * vs IPv6 IP）不命中。空白名单恒 false。
 * @param {string} ip 待判定 IP
 * @param {string[]} whitelist CIDR/裸 IP 条目列表
 * @returns {boolean}
 */
export function isIPWhitelisted(ip, whitelist) {
  if (!Array.isArray(whitelist) || whitelist.length === 0) return false
  const ipBytes = ipToBytes(ip)
  if (!ipBytes) return false
  for (const entry of whitelist) {
    const parsed = parseWhitelistEntry(entry)
    if (!parsed) continue
    if (parsed.bytes.length !== ipBytes.length) continue
    if (cidrMatch(parsed.bytes, parsed.prefixLen, ipBytes)) return true
  }
  return false
}

/** 前缀位逐字节比较（prefixLen 之前必须全等）。 */
function cidrMatch(netBytes, prefixLen, ipBytes) {
  const full = Math.floor(prefixLen / 8)
  for (let i = 0; i < full; i++) {
    if (netBytes[i] !== ipBytes[i]) return false
  }
  const rem = prefixLen % 8
  if (rem === 0) return true
  const mask = (0xff << (8 - rem)) & 0xff
  return (netBytes[full] & mask) === (ipBytes[full] & mask)
}

/**
 * 白名单条目格式校验（SettingsPanel 行级提示用）：合法返回空串，非法返回
 * 中文错误说明（与后端 normalize 剔除口径一致——CIDR 串或裸 IP、掩码长度
 * 不越界）。
 * @param {string} entry
 * @returns {string}
 */
export function validateWhitelistEntry(entry) {
  const trimmed = typeof entry === 'string' ? entry.trim() : ''
  if (!trimmed) return '条目不能为空'
  if (!parseWhitelistEntry(trimmed)) {
    return '格式无效：须为 CIDR（如 10.0.0.0/8、2001:db8::/32）或裸 IP'
  }
  return ''
}

// ---------------------------------------------------------------------------
// 封禁网段输入与白名单交集预检（连接管控八期，BanIPDialog 网段档消费面）。
// 性质同上：仅提示面非正确性依赖（后端 BanCIDR 权威校验 + E_IP_BAN_CIDR_INVALID /
// E_IP_BAN_CIDR_WHITELISTED 兜底拒绝），但口径漂移 = 误导性提示，仍须同源语义。
// ---------------------------------------------------------------------------

/**
 * 解析封禁网段输入（连接管控八期）：CIDR 串或裸 IP 均合法，裸 IP 视为
 * /32 //128 等价形态——与后端 parseIPBanWhitelistEntry 同口径。解析逻辑
 * 委托 parseWhitelistEntry 单点（禁第二套口径），本导出为封禁域语义别名。
 * @param {string} input
 * @returns {{bytes: number[], prefixLen: number}|null} 非法返回 null
 */
export function parseCidrInput(input) {
  return parseWhitelistEntry(input)
}

/**
 * 由网段字节与前缀长度构造网络地址（前缀位保留、主机位清零），作为 CIDR
 * 包含检测的代表点——任意两个 CIDR 网段要么不相交要么一个包含另一个（前缀
 * 代数性质），故「网段 A 首址落入 B」⇔ A ⊆ B，双向首址包含即可判定相交。
 * IPv4/IPv6 同构（4/16 字节统一处理，复用七期 isIPWhitelisted 的字节原语，
 * 非为 IPv6 另造实现）。
 */
function networkAddress(bytes, prefixLen) {
  const out = bytes.slice()
  const full = Math.floor(prefixLen / 8)
  const rem = prefixLen % 8
  if (rem !== 0) {
    const mask = (0xff << (8 - rem)) & 0xff
    out[full] = bytes[full] & mask
  }
  for (let i = rem !== 0 ? full + 1 : full; i < out.length; i++) {
    out[i] = 0
  }
  return out
}

/**
 * 封禁网段与可信网段白名单的相交/覆盖预检（BanIPDialog 网段档提示面用）。
 *
 * 口径边界（与后端 BanCIDR 校验同源，注释即契约）：
 *  - overlapCount：网段与白名单条目相交的条目数，双栈统一字节判定，与后端
 *    相交语义一致（家族不匹配不命中、非法条目跳过）；
 *  - fullyCovered：网段被**单条**白名单条目完全包含（前缀不长于网段且网段
 *    首址落入条目），与后端 cidrFullyCoveredByWhitelist 同为单条目口径——
 *    多条拼接覆盖（如 /24 被 /25+/25 两半合计覆盖）两端口径一致放行（剩余
 *    半段仍有封禁效果）；后端 E_IP_BAN_CIDR_WHITELISTED 权威兜底。
 *
 * @param {string} cidr 封禁网段输入（CIDR 串或裸 IP）
 * @param {string[]} whitelist 可信网段白名单条目列表
 * @returns {{overlapCount: number, fullyCovered: boolean}} 网段非法或空白名单时两值恒零值
 */
export function cidrOverlapWhitelist(cidr, whitelist) {
  const result = { overlapCount: 0, fullyCovered: false }
  const parsed = parseCidrInput(cidr)
  if (!parsed || !Array.isArray(whitelist)) return result
  const firstSelf = networkAddress(parsed.bytes, parsed.prefixLen)
  for (const entry of whitelist) {
    const w = parseWhitelistEntry(entry)
    if (!w || w.bytes.length !== parsed.bytes.length) continue
    const selfInW = cidrMatch(w.bytes, w.prefixLen, firstSelf)
    const wInSelf = cidrMatch(parsed.bytes, parsed.prefixLen, networkAddress(w.bytes, w.prefixLen))
    if (!selfInW && !wInSelf) continue
    result.overlapCount++
    // 单条完全覆盖：网段首址在条目内且条目前缀不长于网段（CIDR 两两非交即含，
    // 两条件合取 ⇔ 网段 ⊆ 该条目）
    if (selfInW && w.prefixLen <= parsed.prefixLen) {
      result.fullyCovered = true
    }
  }
  return result
}
