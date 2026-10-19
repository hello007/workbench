import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import AssetBackupDialog from '../AssetBackupDialog.vue'

// 覆盖面：打开即拉备份清单（List 恒权威本地不缓存）、刷新按钮重拉、空态文案、
// 大小格式化、还原确认链（明示覆盖当前库全部索引与标注 → 确认后按条目绝对路径
// executeRestore → success toast + restored 事件 + 关弹窗）、确认取消不执行、
// 还原失败 handleError 分流弹窗保持可另选、还原在途行级防重。
// 宿主职责（ListAssetLibraryBackups / RestoreAssetLibrary 绑定与刷新链）不经本
// spec——宿主接线由 AssetsPanel.spec 打桩消费。

vi.mock('element-plus', async () => {
  const actual = await vi.importActual('element-plus')
  return {
    ...actual,
    ElMessage: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() },
    ElMessageBox: { confirm: vi.fn(() => Promise.resolve(true)) }
  }
})

import { ElMessage, ElMessageBox } from 'element-plus'

const stubs = {
  'el-dialog': {
    template: '<div v-if="modelValue" class="stub-dialog"><slot /><template v-if="$slots.footer"><div class="dialog-footer"><slot name="footer" /></div></template></div>',
    props: ['modelValue', 'title', 'width', 'closeOnClickModal', 'appendToBody'],
    emits: ['update:modelValue']
  },
  'el-button': {
    template: '<button class="el-button" :disabled="disabled || loading" @click="$emit(\'click\')"><slot /></button>',
    props: ['disabled', 'type', 'size', 'loading', 'text', 'plain'],
    emits: ['click']
  },
  'el-table': {
    template: '<div class="el-table"><slot /></div>',
    props: ['data', 'size', 'maxHeight']
  },
  // 行 stub：遍历 el-table 的 data 逐行渲染 scoped slot（EnvVarPanel.spec 同款
  // $parent.data 形态），prop 列直渲行值、模板列走 :row 作用域插槽
  'el-table-column': {
    props: ['prop', 'label', 'width', 'minWidth', 'fixed', 'showOverflowTooltip'],
    template: '<div class="el-table-col"><span class="col-label">{{ label }}</span><div v-for="(row, i) in $parent.data" :key="i" class="table-row"><template v-if="prop">{{ row[prop] }}</template><slot :row="row" /></div></div>'
  }
}

const mockBackups = [
  { libraryId: 'lib-1', fileName: '20261018-100000.db', path: 'D:/data/assets/backups/lib-1/20261018-100000.db', sizeBytes: 1024 * 1024, createdAt: '2026-10-18 10:00:00' },
  { libraryId: 'lib-1', fileName: '20261017-090000.db', path: 'D:/data/assets/backups/lib-1/20261017-090000.db', sizeBytes: 512 * 1024, createdAt: '2026-10-17 09:00:00' }
]

const createWrapper = async (props = {}) => {
  const wrapper = mount(AssetBackupDialog, {
    props: {
      modelValue: true,
      libraryId: 'lib-1',
      libraryName: '素材库',
      listBackups: vi.fn(() => Promise.resolve(mockBackups)),
      executeRestore: vi.fn(() => Promise.resolve(null)),
      ...props
    },
    global: { stubs }
  })
  await flushPromises()
  return wrapper
}

const findButton = (wrapper, text) =>
  wrapper.findAll('button').find((b) => b.text().trim() === text)

const restoreButtons = (wrapper) =>
  wrapper.findAll('.table-row button.el-button').filter((b) => b.text().trim() === '还原')

describe('AssetBackupDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('打开即拉取清单并渲染时间/文件名/大小行（倒序数据透传）', async () => {
    const listBackups = vi.fn(() => Promise.resolve(mockBackups))
    const wrapper = await createWrapper({ listBackups })
    expect(listBackups).toHaveBeenCalledWith('lib-1')
    // 四列（时间/文件名/大小/操作）× 2 行数据 = 8；操作列按钮由 restoreButtons 单独取
    expect(wrapper.findAll('.table-row')).toHaveLength(8)
    expect(wrapper.text()).toContain('2026-10-18 10:00:00')
    expect(wrapper.text()).toContain('20261018-100000.db')
    // 大小格式化（formatAssetSize 共享单点口径：整数值不带小数位）
    expect(wrapper.text()).toContain('1 MB')
  })

  it('刷新按钮重拉清单', async () => {
    const listBackups = vi.fn(() => Promise.resolve(mockBackups))
    const wrapper = await createWrapper({ listBackups })
    await findButton(wrapper, '刷新').trigger('click')
    await flushPromises()
    expect(listBackups).toHaveBeenCalledTimes(2)
  })

  it('空清单显示备份引导空态', async () => {
    const wrapper = await createWrapper({ listBackups: vi.fn(() => Promise.resolve([])) })
    expect(wrapper.find('.backup-state').text()).toContain('暂无备份')
  })

  it('清单拉取失败 handleError 分流，loading 复位可重拉', async () => {
    const listBackups = vi.fn(() => Promise.reject({ code: 'E_ASSETS_LIB_NOT_FOUND', message: '素材库不存在' }))
    const wrapper = await createWrapper({ listBackups })
    expect(ElMessage.error).toHaveBeenCalledWith('加载备份清单失败: 素材库不存在')
    // loading 复位（加载态文案消失），呈现空态可点刷新重拉
    expect(wrapper.text()).not.toContain('加载备份清单')
    expect(wrapper.find('.backup-state').exists()).toBe(true)
    await findButton(wrapper, '刷新').trigger('click')
    await flushPromises()
    expect(listBackups).toHaveBeenCalledTimes(2)
  })

  it('还原确认链：明示覆盖后果 → 确认后按条目绝对路径执行 → toast + restored + 关弹窗', async () => {
    const executeRestore = vi.fn(() => Promise.resolve(null))
    const wrapper = await createWrapper({ executeRestore })
    await restoreButtons(wrapper)[0].trigger('click')
    await flushPromises()
    expect(ElMessageBox.confirm).toHaveBeenCalledWith(
      expect.stringContaining('覆盖当前库全部索引与标注'),
      '还原备份',
      expect.objectContaining({ type: 'warning' })
    )
    expect(ElMessageBox.confirm).toHaveBeenCalledWith(
      expect.stringContaining('2026-10-18 10:00:00'),
      '还原备份',
      expect.anything()
    )
    expect(executeRestore).toHaveBeenCalledTimes(1)
    // 条目绝对路径原样回传（后端校验必须位于该库备份目录内）
    expect(executeRestore).toHaveBeenCalledWith('D:/data/assets/backups/lib-1/20261018-100000.db')
    expect(ElMessage.success).toHaveBeenCalledWith(expect.stringContaining('20261018-100000.db'))
    expect(wrapper.emitted('restored')).toHaveLength(1)
    expect(wrapper.emitted('restored')[0][0]).toEqual(mockBackups[0])
    expect(wrapper.emitted('update:modelValue')).toContainEqual([false])
  })

  it('确认取消不执行还原', async () => {
    ElMessageBox.confirm.mockRejectedValueOnce('cancel')
    const executeRestore = vi.fn(() => Promise.resolve(null))
    const wrapper = await createWrapper({ executeRestore })
    await restoreButtons(wrapper)[0].trigger('click')
    await flushPromises()
    expect(executeRestore).not.toHaveBeenCalled()
    expect(wrapper.emitted('restored')).toBeUndefined()
  })

  it('还原失败 handleError 分流：弹窗保持、restored 不上报、防重占位复位可重试', async () => {
    const executeRestore = vi.fn(() => Promise.reject({ message: '备份文件完整性校验未通过' }))
    const wrapper = await createWrapper({ executeRestore })
    await restoreButtons(wrapper)[0].trigger('click')
    await flushPromises()
    expect(ElMessage.error).toHaveBeenCalledWith('还原备份失败: 备份文件完整性校验未通过')
    expect(wrapper.emitted('restored')).toBeUndefined()
    expect(wrapper.emitted('update:modelValue') ?? []).not.toContainEqual([false])
    // 防重占位复位：按钮恢复可点（再次触发到确认框即证明未卡死）
    ElMessageBox.confirm.mockClear()
    await restoreButtons(wrapper)[1].trigger('click')
    await flushPromises()
    expect(ElMessageBox.confirm).toHaveBeenCalledTimes(1)
  })

  it('还原在途行级防重：在途期间其余行按钮禁用', async () => {
    let resolveRestore
    const executeRestore = vi.fn(() => new Promise((resolve) => { resolveRestore = resolve }))
    const wrapper = await createWrapper({ executeRestore })
    await restoreButtons(wrapper)[0].trigger('click')
    // 在途：其余行按钮 disabled（行级防重）
    expect(restoreButtons(wrapper)[1].attributes('disabled')).toBeDefined()
    resolveRestore(null)
    await flushPromises()
    // 落定后复位
    expect(restoreButtons(wrapper)[1].attributes('disabled')).toBeUndefined()
  })
})
