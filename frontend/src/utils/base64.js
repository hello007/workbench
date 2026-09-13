/**
 * base64 → UTF-8 文本解码。
 *
 * 桥接背景：后端 ReadFileBytes 返回文件原始字节的 base64；atob 按 Latin-1
 * 逐字节还原，中文等多字节字符直接 string 化会双重编码乱码。须先还原字节
 * 序列再经 TextDecoder 按 UTF-8 解码。
 *
 * @param {string} base64 后端返回的 base64 文本
 * @returns {string} UTF-8 解码后的原文
 */
export function decodeBase64Utf8(base64) {
  const bytes = Uint8Array.from(atob(base64), (c) => c.charCodeAt(0))
  return new TextDecoder('utf-8').decode(bytes)
}
