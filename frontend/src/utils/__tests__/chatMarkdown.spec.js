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

// ===== chat-question 选择题卡片渲染 =====
// 约定：模型输出 ```chat-question JSON``` 围栏，合法渲染交互卡片，
// 非法 JSON / 结构缺失降级普通代码块（PRD B1/B6）

const validCard = JSON.stringify({
  questions: [
    {
      question: '选择部署环境',
      options: [{ label: '测试环境' }, { label: '生产环境', description: '正式对外' }],
      multiSelect: false
    }
  ]
})

function fenceOf(content) {
  return renderChatMarkdown('```chat-question\n' + content + '\n```')
}

describe('renderChatMarkdown - chat-question 卡片', () => {
  it('合法 JSON 渲染为卡片结构（容器/选项按钮/data 定位属性）', () => {
    const html = fenceOf(validCard)
    expect(html).toContain('class="chat-question"')
    expect(html).toContain('chat-question-title')
    expect(html).toContain('data-question="选择部署环境"')
    expect(html).toContain('data-q="0"')
    expect(html).toContain('data-idx="1"')
    expect(html).toContain('chat-question-submit')
    expect(html).toContain('选择部署环境')
    expect(html).toContain('测试环境')
  })

  it('multiSelect: true 的题项带 data-multi 与可多选提示', () => {
    const card = JSON.stringify({
      questions: [{ question: '选模块', options: [{ label: 'A' }, { label: 'B' }], multiSelect: true }]
    })
    const html = fenceOf(card)
    expect(html).toContain('data-multi="true"')
    expect(html).toContain('chat-question-hint')
    expect(html).toContain('可多选')
  })

  it('选项 description 渲染为 title 提示', () => {
    const html = fenceOf(validCard)
    expect(html).toContain('title="正式对外"')
  })

  it('非法 JSON 降级普通代码块（不渲染卡片、不抛错）', () => {
    const html = fenceOf('{questions: [broken')
    expect(html).not.toContain('chat-question-submit')
    expect(html).toContain('<pre')
  })

  it('结构缺失（questions 空/缺 label）降级普通代码块', () => {
    for (const broken of [
      JSON.stringify({ questions: [] }),
      JSON.stringify({ questions: [{ question: 'q', options: [{ no: 'label' }] }] }),
      JSON.stringify({ questions: [{ options: [{ label: 'A' }] }] }),
      JSON.stringify({})
    ]) {
      const html = fenceOf(broken)
      expect(html).not.toContain('chat-question-submit')
      expect(html).toContain('<pre')
    }
  })

  it('选项 label 含 HTML 时被转义（卡片属性/文本防注入）', () => {
    const card = JSON.stringify({
      questions: [{ question: '<b>q</b>', options: [{ label: '<img src=x>' }] }]
    })
    const html = fenceOf(card)
    expect(html).not.toContain('<img src=x>')
    expect(html).toContain('&lt;img src=x&gt;')
  })

  it('未闭合 chat-question 围栏（流式中）按普通代码块容错显示', () => {
    const html = renderChatMarkdown('```chat-question\n{"questions":')
    expect(html).not.toContain('chat-question-submit')
  })

  it('chat-question 之外的 info 附注不误判（如 chat-question.js 走普通代码块）', () => {
    const html = renderChatMarkdown('```chat-question.js\nconst a = 1\n```')
    expect(html).not.toContain('chat-question-submit')
    expect(html).toContain('<pre')
  })
})
