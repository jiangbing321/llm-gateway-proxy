<template>
  <div class="logs-root">
    <!-- 请求日志（独立滚动区域） -->
    <div class="card grow">
      <h3><ScrollText :size="15" :stroke-width="1.75" /> {{ t('logs.title') }}
        <span style="margin-left:auto" class="row">
          <button class="btn" :disabled="loadingHistory" @click="loadAll">
            <History :size="13" /> {{ loadingHistory ? t('logs.loading') : t('logs.loadHistory') }}
          </button>
          <button class="btn btn-danger" @click="clear"><Trash2 :size="13" /> {{ t('logs.clear') }}</button>
          <span class="badge">{{ t('logs.count', { n: app.logs.length }) }}</span>
        </span>
      </h3>
      <div class="log-scroll">
      <table v-if="app.logs.length">
        <thead>
          <tr>
            <th style="width:150px">{{ t('logs.th.time') }}</th>
            <th style="width:120px">{{ t('logs.th.provider') }}</th>
            <th>{{ t('logs.th.pathModel') }}</th>
            <th style="width:70px">{{ t('logs.th.status') }}</th>
            <th style="width:80px">{{ t('logs.th.duration') }}</th>
            <th style="width:70px">{{ t('logs.th.stream') }}</th>
            <th style="width:150px">Tokens</th>
            <th>{{ t('logs.th.error') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="e in reversed" :key="e.id">
            <td style="font-family:var(--mono);font-size:12px">{{ fmtTime(e.time) }}</td>
            <td>
              <div style="font-size:12px" :title="e.transform ? t('logs.transformTip', { tag: e.transform }) : ''">
                {{ e.provider || '-' }}
                <span v-if="e.transform" style="color:var(--yellow)" :title="t('logs.transformBadge')">⇄</span>
              </div>
            </td>
            <td>
              <div style="font-family:var(--mono);font-size:12px">{{ e.path }}</div>
              <div v-if="e.model" style="color:var(--accent);font-size:12px">{{ e.model }}</div>
            </td>
            <td>
              <span class="badge" :class="e.status >= 200 && e.status < 300 ? 'ok' : 'err'">{{ e.status || '-' }}</span>
            </td>
            <td>{{ e.durationMs }}ms</td>
            <td>
              <Check v-if="e.stream" :size="13" style="color:var(--green)" />
              <span v-else style="color:var(--text-dim)">-</span>
            </td>
            <td v-if="e.promptTokens || e.completionTokens" style="font-size:12px">
              ↑{{ e.promptTokens }} ↓{{ e.completionTokens }}
            </td>
            <td v-else>-</td>
            <td style="color:var(--red);font-size:12px">{{ e.error || '' }}</td>
          </tr>
        </tbody>
      </table>
      <div v-else class="desc">{{ t('logs.empty') }}</div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ScrollText, History, Trash2, Check } from 'lucide-vue-next'
import { ClearLogs, GetLogHistory } from '../../wailsjs/go/main/App'
import { useAppStore } from '../stores/app'
import { fmtTimeHM } from '../utils/format'

const app = useAppStore()
const { t } = useI18n({ useScope: 'global' })
const reversed = computed(() => [...app.logs].reverse())
const loadingHistory = ref(false)

// 今天的记录只显示时间，非今天的加日期前缀（MM-DD），避免跨天历史混淆
// fmtTimeHM 内部读 locale → 渲染期调用即建立响应依赖，切语言自动重渲染
function fmtTime(t: string): string {
  const d = new Date(t)
  const now = new Date()
  const sameDay = d.getFullYear() === now.getFullYear() &&
    d.getMonth() === now.getMonth() && d.getDate() === now.getDate()
  const hm = fmtTimeHM(t)
  if (sameDay) return hm
  const mm = String(d.getMonth() + 1).padStart(2, '0')
  const dd = String(d.getDate()).padStart(2, '0')
  return `${mm}-${dd} ${hm}`
}

// 加载落盘历史（proxy-access.log 最近 200 条，含跨重启记录），
// 替换内存列表；历史 ID 为负数，与后续实时推送（正数 ID）的 v-for key 不冲突
async function loadAll() {
  loadingHistory.value = true
  try {
    const res = JSON.parse(await GetLogHistory(200))
    if (res.success) app.logs = res.data
    else app.toast(res.error, 'err')
  } finally {
    loadingHistory.value = false
  }
}

async function clear() {
  if (!(await app.confirm({ title: t('logs.confirmTitle'), body: t('logs.confirmBody'), danger: true }))) return
  await ClearLogs()
  app.logs = []
}
</script>

<style scoped>
/* 页面根容器撑满可视高度：日志表独立滚动 */
.logs-root {
  display: flex;
  flex-direction: column;
  height: calc(100vh - var(--titlebar-h) - 2 * var(--main-pad));
}
.card.grow {
  flex: 1;
  min-height: 220px;
  display: flex;
  flex-direction: column;
}
.log-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;   /* 日志表独立滚动 */
  border-radius: 6px;
}
</style>
