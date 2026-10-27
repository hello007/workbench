import { describe, it, expect } from 'vitest'
import { ipToBytes, parseWhitelistEntry, isIPWhitelisted, validateWhitelistEntry } from '../ipWhitelist'

// 可信网段白名单工具单测（连接管控七期）：与后端 parseIPBanWhitelistEntry /
// isWhitelistedIP 同口径的前端复刻，非法形态拒绝与 CIDR 边界为主要断言面
// （提示面非正确性依赖，但口径漂移 = 误导性提示，仍须守护）。

describe('ipWhitelist.ipToBytes', () => {
  it('IPv4 合法解析为 4 字节', () => {
    expect(ipToBytes('192.168.1.20')).toEqual([192, 168, 1, 20])
    expect(ipToBytes(' 10.0.0.1 ')).toEqual([10, 0, 0, 1])
  })

  it('IPv4 非法形态拒绝（段数/越界/非数字/空串）', () => {
    expect(ipToBytes('10.0.0')).toBeNull()
    expect(ipToBytes('10.0.0.1.2')).toBeNull()
    expect(ipToBytes('256.0.0.1')).toBeNull()
    expect(ipToBytes('a.b.c.d')).toBeNull()
    expect(ipToBytes('')).toBeNull()
    expect(ipToBytes(null)).toBeNull()
  })

  it('IPv6 全形与压缩形态解析为 16 字节', () => {
    // 2001:db8::1 展开后首 2 字节 0x20 0x01、末字节 1
    const v6 = ipToBytes('2001:db8::1')
    expect(v6).toHaveLength(16)
    expect(v6[0]).toBe(0x20)
    expect(v6[1]).toBe(0x01)
    expect(v6[15]).toBe(1)
    // 全形 8 组
    expect(ipToBytes('2001:0db8:0000:0000:0000:0000:0000:0001')).toEqual(v6)
  })

  it('IPv6 IPv4 映射嵌入形态展开', () => {
    const mapped = ipToBytes('::ffff:192.168.1.20')
    expect(mapped).toHaveLength(16)
    expect(mapped.slice(12)).toEqual([192, 168, 1, 20])
  })

  it('IPv6 非法形态拒绝（双压缩/组越界/组数不足且无压缩）', () => {
    expect(ipToBytes('1::2::3')).toBeNull()
    expect(ipToBytes('12345::')).toBeNull()
    expect(ipToBytes('2001:db8')).toBeNull()
  })
})

describe('ipWhitelist.parseWhitelistEntry', () => {
  it('CIDR 串解析出字节数组与前缀长度', () => {
    expect(parseWhitelistEntry('10.0.0.0/8')).toEqual({ bytes: [10, 0, 0, 0], prefixLen: 8 })
    expect(parseWhitelistEntry('2001:db8::/32')).toEqual({ bytes: ipToBytes('2001:db8::'), prefixLen: 32 })
  })

  it('裸 IP 自动补全前缀（IPv4 /32、IPv6 /128，与后端同口径）', () => {
    expect(parseWhitelistEntry('192.168.1.20')).toEqual({ bytes: [192, 168, 1, 20], prefixLen: 32 })
    expect(parseWhitelistEntry('2001:db8::1')).toEqual({ bytes: ipToBytes('2001:db8::1'), prefixLen: 128 })
  })

  it('非法条目拒绝（垃圾串/掩码越界/掩码非数字/空串）', () => {
    expect(parseWhitelistEntry('not-a-cidr')).toBeNull()
    expect(parseWhitelistEntry('10.0.0.0/33')).toBeNull()
    expect(parseWhitelistEntry('2001:db8::/129')).toBeNull()
    expect(parseWhitelistEntry('10.0.0.0/abc')).toBeNull()
    expect(parseWhitelistEntry('   ')).toBeNull()
  })
})

describe('ipWhitelist.isIPWhitelisted', () => {
  it('CIDR 网段边界命中：/8 覆盖 10.x 全段，11.x 不命中', () => {
    const wl = ['10.0.0.0/8']
    expect(isIPWhitelisted('10.1.2.3', wl)).toBe(true)
    expect(isIPWhitelisted('10.255.255.255', wl)).toBe(true)
    expect(isIPWhitelisted('11.0.0.1', wl)).toBe(false)
  })

  it('前缀部分字节边界：/24 第三段精确比较', () => {
    const wl = ['192.168.1.0/24']
    expect(isIPWhitelisted('192.168.1.255', wl)).toBe(true)
    expect(isIPWhitelisted('192.168.2.1', wl)).toBe(false)
  })

  it('裸 IP 条目仅命中同址单机（/32 语义）', () => {
    const wl = ['192.168.1.20']
    expect(isIPWhitelisted('192.168.1.20', wl)).toBe(true)
    expect(isIPWhitelisted('192.168.1.21', wl)).toBe(false)
  })

  it('IPv6 网段命中与家族不匹配不命中', () => {
    const wl = ['2001:db8::/32']
    expect(isIPWhitelisted('2001:db8::1', wl)).toBe(true)
    expect(isIPWhitelisted('2001:db9::1', wl)).toBe(false)
    // IPv4 IP 对 IPv6 条目（家族不同）不命中
    expect(isIPWhitelisted('10.0.0.1', wl)).toBe(false)
  })

  it('空白名单 / 非法 IP / 非法条目跳过后不命中', () => {
    expect(isIPWhitelisted('10.0.0.1', [])).toBe(false)
    expect(isIPWhitelisted('10.0.0.1', undefined)).toBe(false)
    expect(isIPWhitelisted('garbage', ['10.0.0.0/8'])).toBe(false)
    // 非法条目跳过不抛错，合法条目仍参与判定
    expect(isIPWhitelisted('10.0.0.1', ['not-a-cidr', '10.0.0.0/8'])).toBe(true)
  })
})

describe('ipWhitelist.validateWhitelistEntry', () => {
  it('合法条目（CIDR/裸 IP）返回空串', () => {
    expect(validateWhitelistEntry('10.0.0.0/8')).toBe('')
    expect(validateWhitelistEntry('192.168.1.20')).toBe('')
    expect(validateWhitelistEntry('2001:db8::/32')).toBe('')
  })

  it('非法条目返回中文错误说明', () => {
    expect(validateWhitelistEntry('not-a-cidr')).toContain('格式无效')
    expect(validateWhitelistEntry('10.0.0.0/33')).toContain('格式无效')
  })

  it('空串返回非空提示（必填语义）', () => {
    expect(validateWhitelistEntry('')).not.toBe('')
    expect(validateWhitelistEntry('   ')).not.toBe('')
  })
})
