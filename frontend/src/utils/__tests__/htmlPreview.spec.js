/**
 * htmlPreview.js srcdoc 组装测试
 * 覆盖：<head> 内注入 / 无 head 前置注入 / base 指向 /preview-raw + 目录整体编码 /
 *       外链拦截脚本存在性 / 中文路径编码
 */
import { describe, it, expect } from 'vitest'
import { buildHtmlPreviewDoc } from '../htmlPreview'

describe('buildHtmlPreviewDoc', () => {
  it('有 <head> 时：base 与拦截脚本注入到 <head> 之后', () => {
    const doc = buildHtmlPreviewDoc(
      '<html><head><title>t</title></head><body></body></html>',
      'D:/site/page/index.html'
    )
    // 注入内容紧跟 </head> 开标签之后、原 head 内容之前
    const headIdx = doc.indexOf('<title>')
    const baseIdx = doc.indexOf('<base href="/preview-raw/')
    const scriptIdx = doc.indexOf('workbench-open-external')
    expect(baseIdx).toBeGreaterThan(-1)
    expect(scriptIdx).toBeGreaterThan(-1)
    expect(baseIdx).toBeLessThan(headIdx)
    // 原内容保留
    expect(doc).toContain('<title>t</title>')
  })

  it('无 <head> 时：注入内容前置（浏览器归入隐式 head）', () => {
    const doc = buildHtmlPreviewDoc('<html><body>x</body></html>', 'D:/site/a.html')
    expect(doc.startsWith('<base href="/preview-raw/')).toBe(true)
    expect(doc).toContain('<html><body>x</body></html>')
  })

  it('base href = /preview-raw + 整体 encodeURIComponent 的目录（含尾斜杠）', () => {
    const doc = buildHtmlPreviewDoc('<head></head>', 'D:/site/page/index.html')
    const expected = '/preview-raw/' + encodeURIComponent('D:/site/page/')
    expect(doc).toContain(`<base href="${expected}">`)
  })

  it('反斜杠 Windows 路径与正斜杠产出一致', () => {
    const a = buildHtmlPreviewDoc('<head></head>', 'D:\\site\\page\\index.html')
    const b = buildHtmlPreviewDoc('<head></head>', 'D:/site/page/index.html')
    expect(a).toBe(b)
  })

  it('中文路径：目录整体编码（encodeURIComponent 处理非 ASCII）', () => {
    const doc = buildHtmlPreviewDoc('<head></head>', 'D:/文档/页面/index.html')
    expect(doc).toContain(`<base href="/preview-raw/${encodeURIComponent('D:/文档/页面/')}">`)
  })

  it('无 filePath 时不注入 base，但仍注入外链拦截脚本', () => {
    const doc = buildHtmlPreviewDoc('<head></head>', '')
    expect(doc).not.toContain('<base')
    expect(doc).toContain('workbench-open-external')
  })

  it('拦截脚本：https/http 链接 postMessage 到 parent', () => {
    const doc = buildHtmlPreviewDoc('<head></head>', 'D:/a.html')
    // 关键行为片段：capture 监听 + preventDefault + parent.postMessage
    expect(doc).toContain("addEventListener('click'")
    expect(doc).toContain('e.preventDefault()')
    expect(doc).toContain("parent.postMessage({ type: 'workbench-open-external', url: href }, '*')")
    // sandbox 属性由 renderer 模板设置，srcdoc 内不重复
    expect(doc).not.toContain('sandbox=')
  })

  it('空内容 + 空 path：仅拦截脚本，不抛错', () => {
    const doc = buildHtmlPreviewDoc('', '')
    expect(doc).toContain('workbench-open-external')
    expect(doc).not.toContain('<base')
  })
})
