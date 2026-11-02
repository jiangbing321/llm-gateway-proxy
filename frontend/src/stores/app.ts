import { defineStore } from 'pinia'

export interface AppConfig {
  proxyEnabled: boolean
  port: number
  bind: string
  localApiKey: string
  recordEnabled?: boolean
  recordDir?: string
  recordRetentionDays?: number
  /** '' = 跟随系统 | zh | en（保存快照必须带上，否则 SaveConfig 全量覆盖会清掉） */
  language?: string
}

export interface AppStatus {
  proxyRunning: boolean
  proxyURL: string
  proxyError?: string
  port: number
}

export interface LogEntry {
  id: number
  time: string
  path: string
  provider?: string
  model: string
  status: number
  durationMs: number
  stream: boolean
  promptTokens: number
  completionTokens: number
  error?: string
  clientIp: string
  transform?: string
}

export interface Toast {
  id: number
  text: string
  kind: 'ok' | 'err'
}

/** 确认对话框选项（app.confirm() 全局弹窗，替代原生 confirm） */
export interface ConfirmOptions {
  title: string
  body?: string
  /** 危险操作：确认按钮红色，默认焦点给取消按钮（回车=取消，防误触） */
  danger?: boolean
  confirmText?: string
}

export const useAppStore = defineStore('app', {
  state: () => ({
    status: null as AppStatus | null,
    config: null as AppConfig | null,
    logs: [] as LogEntry[],
    toasts: [] as Toast[],
    confirmDialog: { open: false, title: '', danger: false, resolve: null } as (ConfirmOptions & { open: boolean; resolve: ((v: boolean) => void) | null }),
    page: 'dashboard' as 'dashboard' | 'providers' | 'settings' | 'models' | 'stats' | 'playground' | 'recordings' | 'logs',
  }),
  actions: {
    toast(text: string, kind: 'ok' | 'err' = 'ok') {
      const id = Date.now() + Math.random()
      this.toasts.push({ id, text, kind })
      setTimeout(() => {
        this.toasts = this.toasts.filter(t => t.id !== id)
      }, 3200)
    },
    /** 全局确认对话框（ConfirmDialog.vue 渲染）；await 返回用户选择。
     *  注意 state 字段名不能与 action 同名（同名会互相覆盖，首次调用后 action 失效）。 */
    confirm(opts: ConfirmOptions): Promise<boolean> {
      // 已有对话框打开时先拒绝旧 Promise（避免悬挂）
      this.confirmDialog.resolve?.(false)
      return new Promise(resolve => {
        this.confirmDialog = { ...opts, open: true, resolve }
      })
    },
    resolveConfirm(v: boolean) {
      this.confirmDialog.resolve?.(v)
      this.confirmDialog.open = false
    },
    pushLog(e: LogEntry) {
      this.logs.push(e)
      if (this.logs.length > 200) this.logs.splice(0, this.logs.length - 200)
    }
  }
})
