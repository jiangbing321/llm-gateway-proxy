import { createI18n } from 'vue-i18n'
import { zh } from './locales/zh'
import { en } from './locales/en'

export type Lang = 'zh' | 'en'

// 跟随系统：navigator.language（WebView2 跟随系统 UI 语言）zh* → zh，否则 en
export function resolveAutoLang(): Lang {
  return (navigator.language || 'zh').toLowerCase().startsWith('zh') ? 'zh' : 'en'
}

function isLang(v: string | null | undefined): v is Lang {
  return v === 'zh' || v === 'en'
}

// 启动时同步定 locale（mount 前，无闪烁）：localStorage 缓存优先，兜底系统语言
const saved = localStorage.getItem('lang')

export const i18n = createI18n({
  legacy: false,
  locale: isLang(saved) ? saved : resolveAutoLang(),
  fallbackLocale: 'zh',
  messages: { zh, en },
})

export function setLocale(l: Lang) {
  i18n.global.locale.value = l
  localStorage.setItem('lang', l)
  document.documentElement.lang = l === 'zh' ? 'zh-CN' : 'en'
}

// GetConfig 返回后校准：config.language 显式 zh/en 用之；''（跟随系统）用 navigator 解析。
// 与后端 i18n.SetLang 的系统检测语义一致（两端同源于同一 config.json，无分歧窗口）。
export function applyConfigLanguage(cfgLang?: string) {
  const target: Lang = isLang(cfgLang) ? cfgLang : resolveAutoLang()
  if (i18n.global.locale.value !== target) setLocale(target)
}
