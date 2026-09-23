/**
 * chatMarkdown.js 测试
 * 覆盖：plaintext 语言注册守卫（highlight 回调对未识别语言固定回退
 *       language:'plaintext'，漏注册会在渲染无语言标记围栏时稳定触发
 *       hljs「Could not find the language 'plaintext'」console 报错）/
 *       无语言标记围栏渲染 / 基础转义行为
 */
import { describe, it, expect, vi } from 'vitest'
import hljs from 'highlight.js/lib/core'
import { renderChatMarkdown } from '../chatMarkdown'

describe('renderChatMarkdown - plaintext 回归守卫', () => {
  it('hljs 已注册 plaintext（hljs 为模块单例，同一断言实际锁定 chatMarkdown.js 与 FilePreviewRenderer.vue 两处渲染链的注册）', () => {
    expect(hljs.getLanguage('plaintext')).toBeTruthy()
  })

  it('渲染无语言标记围栏：输出 pre.hljs 且 console 无 hljs 找不到语言的报错', () => {
    const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
    try {
      const html = renderChatMarkdown('```\nplain text\n```')
      expect(html).toContain('<pre class="hljs">')
      expect(html).toContain('plain text')
      // plaintext 漏注册时 hljs 降级前会 console.error「Could not find the language '...'」
      const hljsLangErrors = errorSpy.mock.calls.filter((args) =>
        args.some((a) => String(a).includes("Could not find the language"))
      )
      expect(hljsLangErrors).toHaveLength(0)
    } finally {
      errorSpy.mockRestore()
    }
  })
})

describe('renderChatMarkdown - 基础行为', () => {
  it('空输入返回空串', () => {
    expect(renderChatMarkdown('')).toBe('')
    expect(renderChatMarkdown(null)).toBe('')
  })

  it('已知语言围栏走 hljs 高亮结构（对照无语言标记回退分支）', () => {
    const html = renderChatMarkdown('```go\nfmt.Println("hi")\n```')
    expect(html).toContain('<pre class="hljs">')
  })

  it('原始 HTML 被转义不注入（html:false 防 XSS）', () => {
    const html = renderChatMarkdown('<img src=x onerror=alert(1)>')
    expect(html).not.toContain('<img')
    expect(html).toContain('&lt;img')
  })
})
