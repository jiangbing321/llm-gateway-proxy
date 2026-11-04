<template>
  <div class="layout">
    <TitleBar />
    <div class="layout-body" ref="bodyEl" :class="{ 'nav-dragging': navDragging }">
      <aside v-show="!navHidden" class="sidebar" :style="{ width: navW + 'px' }">
        <template v-for="group in navGroups" :key="group.title">
          <div class="nav-group-title">{{ group.title }}</div>
          <div v-for="item in group.items" :key="item.key" class="nav-item"
               :class="{ active: app.page === item.key }" @click="app.page = item.key">
            <component :is="item.icon" :size="15" :stroke-width="1.75" />
            <span>{{ item.label }}</span>
          </div>
        </template>
        <div class="sidebar-footer">
          <div style="margin-top:6px">{{ t('shell.closeToTrayHint') }}</div>
        </div>
      </aside>

      <!-- 侧边栏分隔条：拖动调宽 / 中间按钮折叠 -->
      <div class="nav-splitter" :title="navHidden ? '' : t('shell.dragResize')" @mousedown="onNavSplitterDown">
        <button class="nav-toggle" :title="navHidden ? t('shell.expandNav') : t('shell.collapseNav')"
                @mousedown.stop @click="toggleNav">
          <ChevronLeft v-if="!navHidden" :size="16" :stroke-width="2.5" />
          <ChevronRight v-else :size="16" :stroke-width="2.5" />
        </button>
      </div>

      <main class="main">
        <section v-if="updateInfo" class="update-banner" role="status">
          <div class="update-message">
            <strong>{{ t('shell.updateAvailable', { version: updateInfo.version }) }}</strong>
            <p v-if="updateInfo.releaseNotes">{{ updateInfo.releaseNotes }}</p>
          </div>
          <div class="update-actions">
            <button type="button" class="update-link" @click="openUpdateLink(updateInfo.downloadUrl)">
              {{ t('shell.downloadUpdate') }} <ExternalLink :size="13" />
            </button>
            <button type="button" class="update-link secondary" @click="openUpdateLink(updateInfo.releaseNotesUrl)">
              {{ t('shell.releaseNotes') }}
            </button>
            <button type="button" class="update-dismiss" :aria-label="t('shell.dismissUpdate')" @click="updateInfo = null">
              <X :size="15" />
            </button>
          </div>
        </section>
        <Transition name="page" mode="out-in">
          <Dashboard v-if="app.page === 'dashboard'" />
          <ProvidersPage v-else-if="app.page === 'providers'" />
          <ProxySettings v-else-if="app.page === 'settings'" />
          <Models v-else-if="app.page === 'models'" />
          <Stats v-else-if="app.page === 'stats'" />
          <Playground v-else-if="app.page === 'playground'" />
          <Recordings v-else-if="app.page === 'recordings'" />
          <Logs v-else-if="app.page === 'logs'" />
        </Transition>
      </main>
    </div>
    <div class="toast-wrap">
      <div v-for="t in app.toasts" :key="t.id" class="toast" :class="t.kind">{{ t.text }}</div>
    </div>
    <ConfirmDialog />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import type { Component } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Home, Server, Boxes, BarChart3, FlaskConical,
  FolderOpen, ScrollText, Settings2, ChevronLeft, ChevronRight, ExternalLink, X
} from 'lucide-vue-next'
import { GetAvailableUpdate, GetConfig, GetStatus } from '../wailsjs/go/main/App'
import { BrowserOpenURL, EventsOn } from '../wailsjs/runtime/runtime'
import { useAppStore, LogEntry } from './stores/app'
import { applyConfigLanguage } from './i18n'
import TitleBar from './components/TitleBar.vue'
import ConfirmDialog from './components/ConfirmDialog.vue'
import Dashboard from './pages/Dashboard.vue'
import ProvidersPage from './pages/Providers.vue'
import ProxySettings from './pages/ProxySettings.vue'
import Models from './pages/Models.vue'
import Stats from './pages/Stats.vue'
import Playground from './pages/Playground.vue'
import Recordings from './pages/Recordings.vue'
import Logs from './pages/Logs.vue'

const app = useAppStore()
const { t } = useI18n({ useScope: 'global' })
interface UpdateInfo {
  version: string
  downloadUrl: string
  releaseNotesUrl: string
  releaseNotes?: string
}
const updateInfo = ref<UpdateInfo | null>(null)

function openUpdateLink(url: string) {
  BrowserOpenURL(url)
}

// ---- 侧边栏宽度拖拽 / 折叠（偏好持久化） ----
const bodyEl = ref<HTMLElement | null>(null)
const navW = ref(Math.min(420, Math.max(170, Number(localStorage.getItem('app-nav-w')) || 208)))
const navHidden = ref(localStorage.getItem('app-nav-hidden') === '1')
const navDragging = ref(false)

function toggleNav() {
  navHidden.value = !navHidden.value
  localStorage.setItem('app-nav-hidden', navHidden.value ? '1' : '0')
}

function onNavSplitterDown(e: MouseEvent) {
  if (navHidden.value || e.button !== 0) return
  navDragging.value = true
  e.preventDefault()
  const move = (ev: MouseEvent) => {
    const rect = bodyEl.value?.getBoundingClientRect()
    if (!rect) return
    navW.value = Math.max(170, Math.min(420, ev.clientX - rect.left))
  }
  const up = () => {
    navDragging.value = false
    document.removeEventListener('mousemove', move)
    document.removeEventListener('mouseup', up)
    localStorage.setItem('app-nav-w', String(navW.value))
  }
  document.addEventListener('mousemove', move)
  document.addEventListener('mouseup', up)
}

// 导航：computed 使 label 随语言即时切换（显式 PageKey 类型保住 app.page 赋值的字面量联合）
type PageKey = 'dashboard' | 'providers' | 'settings' | 'models' | 'stats' | 'playground' | 'recordings' | 'logs'
interface NavItem { key: PageKey; icon: Component; label: string }
const navGroups = computed<{ title: string; items: NavItem[] }[]>(() => [
  {
    title: t('nav.group.gateway'),
    items: [
      { key: 'dashboard', icon: Home, label: t('nav.dashboard') },
      { key: 'providers', icon: Server, label: t('nav.providers') },
      { key: 'models', icon: Boxes, label: t('nav.models') }
    ]
  },
  {
    title: t('nav.group.tools'),
    items: [
      { key: 'stats', icon: BarChart3, label: t('nav.stats') },
      { key: 'playground', icon: FlaskConical, label: t('nav.playground') },
      { key: 'recordings', icon: FolderOpen, label: t('nav.recordings') },
      { key: 'logs', icon: ScrollText, label: t('nav.logs') },
      { key: 'settings', icon: Settings2, label: t('nav.settings') }
    ]
  }
])

async function refreshStatus() {
  try {
    const raw = await GetStatus()
    const res = JSON.parse(raw)
    if (res.success) app.status = res.data
  } catch (e) {
    console.warn('GetStatus failed', e)
  }
}

async function refreshConfig() {
  try {
    const res = JSON.parse(await GetConfig())
    if (res.success) {
      app.config = res.data
      // 启动时以 config.language 校准（显式值优先；'' 保持系统语言解析，与后端检测一致）
      applyConfigLanguage(app.config?.language)
    }
  } catch (e) {
    console.warn('GetConfig failed', e)
  }
}

async function refreshAvailableUpdate() {
  try {
    const res = JSON.parse(await GetAvailableUpdate())
    if (res.success && res.data) updateInfo.value = res.data
  } catch (e) {
    console.warn('GetAvailableUpdate failed', e)
  }
}

onMounted(() => {
  EventsOn('version:update-available', (info: UpdateInfo) => {
    updateInfo.value = info
  })
  refreshAvailableUpdate()
  refreshStatus()
  refreshConfig()
  setInterval(refreshStatus, 5000)

  EventsOn('log:new', (e: LogEntry) => {
    app.pushLog(e)
  })
  EventsOn('proxy:status', (st: { running: boolean; addr: string; error: string }) => {
    if (app.status) {
      app.status.proxyRunning = st.running
      app.status.proxyURL = st.running ? 'http://' + st.addr : ''
    }
    if (st.error) app.toast(t('shell.proxyStatusChanged') + st.error, 'err')
    refreshStatus()
  })
})
</script>

<style scoped>
.update-banner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 18px;
  margin: 0 0 18px;
  padding: 12px 14px;
  border: 1px solid rgba(124, 140, 248, .35);
  border-radius: var(--radius);
  background: linear-gradient(110deg, rgba(94, 106, 210, .16), rgba(34, 211, 238, .07));
}
.update-message { min-width: 0; }
.update-message strong { color: var(--text); font-size: 12px; font-weight: 600; }
.update-message p { margin: 4px 0 0; color: var(--text-dim); font-size: 11px; line-height: 1.5; }
.update-actions { display: flex; flex: 0 0 auto; align-items: center; gap: 12px; }
.update-link, .update-dismiss {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 5px;
  padding: 5px 8px;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--cyan);
  font: inherit;
  font-size: 11px;
  cursor: pointer;
}
.update-link:hover, .update-dismiss:hover { background: rgba(124, 140, 248, .12); }
.update-link.secondary { color: var(--text-dim); }
.update-dismiss { color: var(--text-dim); }
@media (max-width: 650px) {
  .update-banner { align-items: flex-start; flex-direction: column; gap: 8px; }
  .update-actions { width: 100%; justify-content: flex-start; }
  .update-dismiss { margin-left: auto; }
}
</style>
