import { describe, it, expect } from 'vitest'
import { decodeBase64Utf8 } from '../base64'

/** 按后端 ReadFileBytes 语义构造 base64：原始字节逐字节映射后编码（非裸 btoa(文本)） */
function encodeUtf8Base64(text) {
  const bytes = new TextEncoder().encode(text)
  let binary = ''
  bytes.forEach((b) => {
    binary += String.fromCharCode(b)
  })
  return btoa(binary)
}

describe('decodeBase64Utf8', () => {
  it('中文等多字节 UTF-8 内容按字节还原后解码与原文一致（裸 atob 会双重编码乱码）', () => {
    const text = '{"name":"导入新目录","path":"D:/工作区/新目录"}'
    expect(decodeBase64Utf8(encodeUtf8Base64(text))).toBe(text)
  })

  it('纯 ASCII 内容解码正确', () => {
    const text = '{"manifestVersion":1}'
    expect(decodeBase64Utf8(btoa(text))).toBe(text)
  })

  it('空字符串返回空字符串', () => {
    expect(decodeBase64Utf8('')).toBe('')
  })
})
