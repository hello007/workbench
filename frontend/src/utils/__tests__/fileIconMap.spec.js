import { describe, it, expect } from 'vitest'
import { getIconForFile, DEFAULT_ICON_MAP, FILENAME_ICON_MAP } from '../fileIconMap'

describe('getIconForFile', () => {
  it('空名/null 返回 null', () => {
    expect(getIconForFile('')).toBeNull()
    expect(getIconForFile(null)).toBeNull()
  })

  it('无后缀特殊文件 LICENSE/COPYING 返回 license 图标', () => {
    expect(getIconForFile('LICENSE')).toBeTruthy()
    expect(getIconForFile('licence')).toBeTruthy()
    expect(getIconForFile('copying')).toBeTruthy()
    expect(getIconForFile('notice')).toBeTruthy()
  })

  it('常见后缀返回对应图标', () => {
    expect(getIconForFile('main.go')).toBeTruthy()
    expect(getIconForFile('App.vue')).toBeTruthy()
    expect(getIconForFile('data.json')).toBeTruthy()
    expect(getIconForFile('style.css')).toBeTruthy()
    expect(getIconForFile('readme.md')).toBeTruthy()
  })

  it('后缀大小写不敏感', () => {
    expect(getIconForFile('A.GO')).toBeTruthy()
    expect(getIconForFile('B.Vue')).toBeTruthy()
    expect(getIconForFile('C.JSON')).toBeTruthy()
  })

  it('未知后缀返回 null', () => {
    expect(getIconForFile('file.unknownext')).toBeNull()
  })

  it('无后缀非特殊文件返回 null', () => {
    expect(getIconForFile('README')).toBeNull()
    expect(getIconForFile('Makefile')).toBeNull()
  })

  it('压缩包后缀返回 zip 图标', () => {
    expect(getIconForFile('a.zip')).toBeTruthy()
    expect(getIconForFile('b.tar')).toBeTruthy()
    expect(getIconForFile('c.gz')).toBeTruthy()
    expect(getIconForFile('d.7z')).toBeTruthy()
  })

  it('配置文件后缀返回 properties 图标', () => {
    expect(getIconForFile('app.ini')).toBeTruthy()
    expect(getIconForFile('config.toml')).toBeTruthy()
    expect(getIconForFile('.env')).toBeTruthy() // .env 的 getExtension 返回 'env'，匹配 properties 图标
    expect(getIconForFile('app.env')).toBeTruthy()
  })

  it('DEFAULT_ICON_MAP 包含常见后缀', () => {
    expect(DEFAULT_ICON_MAP.go).toBeTruthy()
    expect(DEFAULT_ICON_MAP.vue).toBeTruthy()
    expect(DEFAULT_ICON_MAP.json).toBeTruthy()
    expect(DEFAULT_ICON_MAP['7z']).toBeTruthy()
  })

  it('FILENAME_ICON_MAP 包含 license 系列', () => {
    expect(FILENAME_ICON_MAP.license).toBeTruthy()
    expect(FILENAME_ICON_MAP.copying).toBeTruthy()
  })
})
