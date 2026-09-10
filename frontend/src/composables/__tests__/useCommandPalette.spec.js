import { describe, it, expect, vi, beforeEach } from 'vitest'
import { useCommandPalette } from '../useCommandPalette'

// wailsjs 在 frontend/wailsjs，从 src/composables/__tests__/ 解析为 ../../../wailsjs
vi.mock('../../../wailsjs/go/main/App', () => ({
  SearchFiles: vi.fn(() => Promise.resolve([{ name: 'a.go', path: 'src/a.go', type: 'file' }])),
  ContentSearch: vi.fn(() => Promise.resolve([{ repo: 'r', results: [] }]))
}))

import { SearchFiles, ContentSearch } from '../../../wailsjs/go/main/App'

describe('useCommandPalette', () => {
  let palette

  beforeEach(() => {
    vi.clearAllMocks()
    SearchFiles.mockResolvedValue([{ name: 'a.go', path: 'src/a.go', type: 'file' }])
    ContentSearch.mockResolvedValue([{ repo: 'r', results: [] }])
    palette = useCommandPalette()
  })

  describe('mode 计算', () => {
    it(':: 前缀为 content-global', () => {
      palette.input.value = '::kw'
      expect(palette.mode.value).toBe('content-global')
    })
    it(': 前缀为 content', () => {
      palette.input.value = ':kw'
      expect(palette.mode.value).toBe('content')
    })
    it('# 前缀为 workdir', () => {
      palette.input.value = '#'
      expect(palette.mode.value).toBe('workdir')
    })
    it('@ 前缀为 favorites', () => {
      palette.input.value = '@'
      expect(palette.mode.value).toBe('favorites')
    })
    it('> 前缀为 command', () => {
      palette.input.value = '>cmd'
      expect(palette.mode.value).toBe('command')
    })
    it('无前缀为 general', () => {
      palette.input.value = 'kw'
      expect(palette.mode.value).toBe('general')
    })
  })

  describe('query 计算', () => {
    it('general 返回 trim 输入', () => {
      palette.input.value = '  kw  '
      expect(palette.query.value).toBe('kw')
    })
    it('workdir 去掉 # 前缀', () => {
      palette.input.value = '#proj'
      expect(palette.query.value).toBe('proj')
    })
    it('content 去掉 : 前缀', () => {
      palette.input.value = ':kw'
      expect(palette.query.value).toBe('kw')
    })
    it('content-global 去掉 :: 前缀', () => {
      palette.input.value = '::kw'
      expect(palette.query.value).toBe('kw')
    })
  })

  describe('contentQuery 解析', () => {
    it('空查询返回空字段', () => {
      palette.input.value = ':'
      expect(palette.contentQuery.value).toEqual({ keyword: '', fileExt: '', subDir: '' })
    })
    it('提取文件扩展名', () => {
      palette.input.value = ':.go keyword'
      expect(palette.contentQuery.value.fileExt).toBe('.go')
      expect(palette.contentQuery.value.keyword).toBe('keyword')
    })
    it('提取子目录', () => {
      palette.input.value = ':src/ keyword'
      expect(palette.contentQuery.value.subDir).toBe('src')
      expect(palette.contentQuery.value.keyword).toBe('keyword')
    })
    it('仅 keyword', () => {
      palette.input.value = ':keyword'
      expect(palette.contentQuery.value.keyword).toBe('keyword')
    })
  })

  describe('open/close', () => {
    it('open 重置状态并显示', () => {
      palette.input.value = 'old'
      palette.fileResults.value = [{ name: 'x' }]
      palette.open()
      expect(palette.visible.value).toBe(true)
      expect(palette.input.value).toBe('')
      expect(palette.fileResults.value).toEqual([])
      expect(palette.contentSearchExecuted.value).toBe(false)
    })
    it('close 重置状态并隐藏', () => {
      palette.visible.value = true
      palette.input.value = 'old'
      palette.close()
      expect(palette.visible.value).toBe(false)
      expect(palette.input.value).toBe('')
      expect(palette.fileResults.value).toEqual([])
    })
    it('openWithContentSearch 设置 : 前缀', () => {
      palette.openWithContentSearch('src')
      expect(palette.visible.value).toBe(true)
      expect(palette.input.value).toBe(':src/ ')
    })
  })

  describe('searchFiles', () => {
    it('general 模式查询返回结果', async () => {
      palette.input.value = 'kw'
      await palette.searchFiles('/root')
      expect(palette.searchLoading.value).toBe(false)
      expect(palette.fileResults.value.length).toBe(1)
    })
    it('非 general 模式不查询', async () => {
      palette.input.value = '#prefix'
      await palette.searchFiles('/root')
      expect(palette.fileResults.value).toEqual([])
    })
    it('空 query 不查询', async () => {
      palette.input.value = ''
      await palette.searchFiles('/root')
      expect(palette.fileResults.value).toEqual([])
    })
    it('查询异常时置空结果', async () => {
      SearchFiles.mockRejectedValueOnce(new Error('fail'))
      palette.input.value = 'kw'
      await palette.searchFiles('/root')
      expect(palette.fileResults.value).toEqual([])
      expect(palette.searchLoading.value).toBe(false)
    })
  })

  describe('executeContentSearch', () => {
    it('空 keyword 不执行', async () => {
      palette.input.value = ':'
      await palette.executeContentSearch()
      expect(palette.contentSearching.value).toBe(false)
      expect(palette.contentSearchExecuted.value).toBe(false)
    })
    it('有 keyword 执行并返回分组', async () => {
      palette.input.value = ':kw'
      await palette.executeContentSearch()
      expect(palette.contentSearchExecuted.value).toBe(true)
      expect(palette.contentGroups.value.length).toBe(1)
    })
    it('异常时 contentGroups 置空但 executed 为 true', async () => {
      ContentSearch.mockRejectedValueOnce(new Error('fail'))
      palette.input.value = ':kw'
      await palette.executeContentSearch()
      expect(palette.contentGroups.value).toEqual([])
      expect(palette.contentSearchExecuted.value).toBe(true)
    })
  })

  describe('moveSelection / resetSelection', () => {
    it('下移选中索引', () => {
      palette.fileResults.value = [{}, {}, {}]
      palette.moveSelection(1)
      expect(palette.selectedIndex.value).toBe(1)
    })
    it('下移越界保持最大索引', () => {
      palette.fileResults.value = [{}, {}, {}]
      palette.selectedIndex.value = 2
      palette.moveSelection(1)
      expect(palette.selectedIndex.value).toBe(2)
    })
    it('上移下界为 0', () => {
      palette.fileResults.value = [{}, {}, {}]
      palette.selectedIndex.value = 1
      palette.moveSelection(-5)
      expect(palette.selectedIndex.value).toBe(0)
    })
    it('resetSelection 重置为 0', () => {
      palette.selectedIndex.value = 5
      palette.resetSelection()
      expect(palette.selectedIndex.value).toBe(0)
    })
  })
})
