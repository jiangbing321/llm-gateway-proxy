import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import { i18n, setLocale } from './i18n'
// 打包字体（latin subset woff2；中文回退系统雅黑）
import '@fontsource/inter/400.css'
import '@fontsource/inter/500.css'
import '@fontsource/inter/600.css'
import '@fontsource/jetbrains-mono/400.css'
import '@fontsource/jetbrains-mono/500.css'
import './style.css'

// mount 前同步设定语言与 <html lang>（createI18n 已按 localStorage/系统语言定 locale）
setLocale(i18n.global.locale.value as 'zh' | 'en')

createApp(App).use(createPinia()).use(i18n).mount('#app')
