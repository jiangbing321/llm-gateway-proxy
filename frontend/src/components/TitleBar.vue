<template>
  <div class="titlebar">
    <div class="title">
      <span class="logo">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="url(#lgp-grad)" stroke-width="2"
             stroke-linecap="round" stroke-linejoin="round">
          <defs>
            <linearGradient id="lgp-grad" x1="0" y1="0" x2="24" y2="24" gradientUnits="userSpaceOnUse">
              <stop offset="0" stop-color="#6366F1" />
              <stop offset="1" stop-color="#22D3EE" />
            </linearGradient>
          </defs>
          <path d="M13 2 3 14h9l-1 8 10-12h-9l1-8z" />
        </svg>
      </span>
      <span>LLM Gateway Proxy</span>
      <span class="badge" :class="app.status?.proxyRunning ? 'ok' : ''"
            :style="!app.status?.proxyRunning ? 'opacity:.55' : ''">
        <span v-if="app.status?.proxyRunning" class="dot ok pulse"></span>
        {{ app.status?.proxyRunning ? t('shell.proxyRunning') : t('shell.proxyStopped') }}
      </span>
    </div>
    <div class="controls">
      <button class="win-btn" :title="t('shell.minimize')" @click="WindowMinimise()"><span class="icon-min"></span></button>
      <button class="win-btn" :title="t('shell.maximize')" @click="WindowToggleMaximise()"><span class="icon-max"></span></button>
      <button class="win-btn close" :title="t('shell.toTray')" @click="WindowClose()"><span class="icon-close"></span></button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { WindowClose, WindowMinimise, WindowToggleMaximise } from '../../wailsjs/go/main/App'
import { useAppStore } from '../stores/app'

const app = useAppStore()
const { t } = useI18n({ useScope: 'global' })
</script>

<style scoped>
/* 悬浮标题栏：内容滚动时从其下方穿过（见 style.css 的 .layout/.sidebar/.main）。
   性能关键：不用 backdrop-filter——全宽悬浮层叠在大内容上时，每次窗口呈现都要对
   下方整块内容做高斯模糊合成，录制浏览大文本时曾导致窗口激活/切换卡顿数秒。 */
.titlebar {
  position: absolute; top: 0; left: 0; right: 0; z-index: 100;
  height: var(--titlebar-h); display: flex; align-items: center; justify-content: space-between;
  background: var(--bg);
  border-bottom: 1px solid var(--border);
  padding-left: 14px; user-select: none;
  --wails-draggable: drag;
}
.title { display: flex; align-items: center; gap: 8px; font-size: 12.5px; font-weight: 600; letter-spacing: .01em; }
.title .logo { display: inline-flex; align-items: center; }
.badge { font-weight: 400; }
.controls {
  display: flex; height: 100%;
  --wails-draggable: no-drag; /* 豁免拖拽区：否则第一次点击被拖拽层吞掉，需按两次 */
}
.win-btn {
  width: 44px; height: 100%; background: transparent; border: none; color: var(--text-dim);
  cursor: pointer;
  display: inline-flex; align-items: center; justify-content: center;
  transition: background-color .12s ease, color .12s ease;
}
.win-btn:hover { background: var(--bg-elevated); color: var(--text); }
.win-btn.close:hover { background: var(--red); color: #fff; }

/* 纯 CSS 矢量图标（替代字体字符，DPI 缩放下保持锐利） */
.icon-min, .icon-max, .icon-close {
  display: inline-block; position: relative; width: 10px; height: 10px;
}
.icon-min::before { /* 底部横线 */
  content: ''; position: absolute; left: 0; right: 0; bottom: 0.5px;
  height: 1.5px; background: currentColor; border-radius: 1px;
}
.icon-max::before { /* 空心方框 */
  content: ''; position: absolute; inset: 0.5px;
  border: 1.5px solid currentColor; border-radius: 1px;
}
.icon-close::before, .icon-close::after { /* X = 两条 45° 线 */
  content: ''; position: absolute; top: 50%; left: 50%;
  width: 13px; height: 1.5px; background: currentColor; border-radius: 1px;
}
.icon-close::before { transform: translate(-50%, -50%) rotate(45deg); }
.icon-close::after { transform: translate(-50%, -50%) rotate(-45deg); }
</style>
