// locale 感知的日期格式化：在 computed/模板内调用即可随语言切换自动重渲染
import { i18n } from '../i18n'

function tag(): string {
  return i18n.global.locale.value === 'en' ? 'en-US' : 'zh-CN'
}

export function fmtDateTime(ts: string | number | Date): string {
  return new Date(ts).toLocaleString(tag())
}

export function fmtTime(t: string | number | Date): string {
  return new Date(t).toLocaleTimeString(tag())
}

// HH:mm:ss（24 小时制，两种语言一致）
export function fmtTimeHM(t: string | number | Date): string {
  return new Date(t).toLocaleTimeString(tag(), { hour12: false })
}
