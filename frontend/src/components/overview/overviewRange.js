/**
 * 概览「AI 任务」域时间档位的文案单点真相源。
 *
 * 档位 key 与窗口起点由后端 docs/spec/dashboard-metrics.md §4 定义；本模块只负责
 * 中文标签与口径句，供 AiCostCard / AiSuccessCard 共用，避免两张卡片各写一份漂移。
 */
export const OVERVIEW_RANGE_LABELS = {
  today: '今日',
  week: '本周',
  '7d': '近 7 天',
  '30d': '近 30 天',
  '90d': '近 90 天'
}

/** 未知/缺失档位按默认档 week 展示（与后端 normalizeOverviewRange 同口径）。 */
export function overviewRangeLabel(key) {
  return OVERVIEW_RANGE_LABELS[key] || OVERVIEW_RANGE_LABELS.week
}

/**
 * AI 卡片底部常显口径句：范围随档位联动，保留上限为恒定事实。
 * 「近 90 天 / 最多 2000 条」是历史保留上限（超出不可见），不是当前所选范围。
 */
export function overviewRangeText(key) {
  return `范围：${overviewRangeLabel(key)} · 历史保留上限：近 90 天 / 最多 2000 条`
}
