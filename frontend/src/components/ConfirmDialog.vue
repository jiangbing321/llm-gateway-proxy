<template>
  <Transition name="confirm">
    <div v-if="app.confirmDialog.open" class="confirm-mask" tabindex="-1" :ref="onMaskMounted" @keydown.esc="cancel">
      <div class="confirm-card" role="dialog" aria-modal="true">
        <div class="confirm-title">{{ app.confirmDialog.title }}</div>
        <div v-if="app.confirmDialog.body" class="confirm-body">{{ app.confirmDialog.body }}</div>
        <div class="confirm-actions">
          <button class="btn" @click="cancel">{{ t('common.cancel') }}</button>
          <button :class="app.confirmDialog.danger ? 'btn btn-danger' : 'btn btn-primary'" @click="ok">
            {{ app.confirmDialog.confirmText || t('common.confirm') }}
          </button>
        </div>
      </div>
    </div>
  </Transition>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { useAppStore } from '../stores/app'

const app = useAppStore()
const { t } = useI18n({ useScope: 'global' })

function ok() { app.resolveConfirm(true) }
function cancel() { app.resolveConfirm(false) }

// ref 函数：元素插入 DOM 时同步回调（比 Transition 的 @enter 钩子时机更可靠）。
// 危险操作默认焦点给取消（回车=取消，防误删）；普通操作给确认。
// 焦点在卡内按钮上：Esc 冒泡到 mask 触发取消，Enter 走按钮原生 click。
function onMaskMounted(el: Element | null) {
  if (!el) return
  const btns = (el as HTMLElement).querySelectorAll('button')
  const target = app.confirmDialog.danger ? btns[0] : btns[btns.length - 1]
  ;(target as HTMLButtonElement | undefined)?.focus()
}
</script>

<style scoped>
.confirm-mask {
  position: fixed; inset: 0; z-index: 500;   /* 标题栏(100)之上、toast(1000)之下 */
  background: rgba(4, 7, 12, .62);
  display: flex; align-items: center; justify-content: center;
  outline: none;
}
.confirm-card {
  width: 360px; max-width: calc(100vw - 48px);
  background: var(--bg-card); border: 1px solid var(--border-bright);
  border-radius: 12px; padding: 18px;
  box-shadow: 0 16px 48px rgba(0, 0, 0, .5);
}
.confirm-title { font-size: 14px; font-weight: 600; color: var(--text); }
.confirm-body { margin-top: 8px; font-size: 12.5px; line-height: 1.7; color: var(--text-dim); white-space: pre-line; }
.confirm-actions { display: flex; justify-content: flex-end; gap: 10px; margin-top: 18px; }

.confirm-enter-active { transition: opacity .15s ease; }
.confirm-enter-active .confirm-card { transition: transform .15s ease, opacity .15s ease; }
.confirm-leave-active { transition: opacity .12s ease; }
.confirm-enter-from, .confirm-leave-to { opacity: 0; }
.confirm-enter-from .confirm-card { transform: translateY(8px) scale(.98); }
</style>
