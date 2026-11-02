<template>
  <div class="rec-root">
    <!-- 单行工具栏：录制开关 / 设置 / 打开目录 / 搜索 / 刷新 -->
    <div class="card shrink toolbar">
      <label class="switch-row">
        <input type="checkbox" v-model="recEnabled" @change="saveRecord" />
        <span class="row" style="gap:6px">
          <span v-if="recEnabled" class="dot ok pulse"></span>
          <span v-else class="dot off"></span>
          {{ recEnabled ? t('recordings.toolbar.recording') : t('recordings.toolbar.stopped') }}
        </span>
      </label>
      <button class="btn sm" :class="{ on: settingsOpen }" @click="settingsOpen = !settingsOpen">
        <Settings2 :size="13" /> {{ t('recordings.toolbar.settings') }}
      </button>
      <button class="btn sm" @click="openDir"><FolderOpen :size="13" /> {{ t('recordings.toolbar.openDir') }}</button>
      <span class="toolbar-search">
        <input type="text" v-model="keyword" class="input" :placeholder="t('recordings.toolbar.searchPh')" @keyup.enter="reloadEntries" />
        <button class="btn sm" @click="reloadEntries" :title="t('recordings.toolbar.search')"><Search :size="13" /></button>
        <button class="btn sm" @click="loadFiles" :title="t('recordings.toolbar.refreshFiles')"><RefreshCw :size="13" /></button>
      </span>
    </div>

    <!-- 设置面板（默认收起） -->
    <div v-show="settingsOpen" class="card shrink settings-panel">
      <div class="sp-row">
        <span class="sp-label">{{ t('recordings.sp.currentFile') }}</span>
        <CopyField v-if="recInfo?.file" :text="recInfo.file" @copied="app.toast(t('common.copied'))" />
        <span v-else class="desc">{{ t('recordings.sp.noFile') }}</span>
        <span v-if="recInfo?.size" class="desc">{{ t('recordings.sp.jsonl', { kb: (recInfo.size / 1024).toFixed(1) }) }}</span>
        <button class="btn sm" style="margin-left:auto" @click="pickDir">{{ t('recordings.sp.changeDir') }}</button>
      </div>
      <div class="sp-row">
        <span class="sp-label">{{ t('recordings.sp.retention') }}</span>
        <input type="number" v-model.number="retentionDays" min="1" max="3650" style="max-width:110px" :disabled="!app.config" @change="saveRetention" />
        <span class="desc">{{ t('recordings.sp.retentionUnit') }}</span>
      </div>
      <i18n-t keypath="recordings.sp.desc" tag="div" class="desc" style="margin:0">
        <template #b><b>{{ t('recordings.sp.descBold') }}</b></template>
      </i18n-t>
    </div>

    <!-- 内容浏览器：左列表（简要信息）+ 右详情（原始/格式化） -->
    <div class="card grow browser-card">
      <div class="browser" ref="browserEl" :class="{ dragging }">
        <div v-show="!listHidden" class="left-col" :style="{ width: listW + 'px' }">
          <!-- 文件头：日期下拉 + 删除当天 -->
          <div class="file-header">
            <select class="file-select" :value="selected" :title="t('recordings.list.pickDate')"
                    :disabled="!files.length" @change="onDateChange">
              <option value="" disabled v-if="!files.length">{{ t('recordings.list.noFiles') }}</option>
              <option v-for="f in files" :key="f.date" :value="f.date"
                      :title="(f.sizeBytes / 1024).toFixed(0) + ' KB'">{{ f.date }} · {{ t('recordings.list.entries', { n: f.lines }) }}</option>
            </select>
            <button class="file-del" :title="t('recordings.list.delDay')" :disabled="!selected" @click="del(selected)">
              <Trash2 :size="13" />
            </button>
          </div>
          <div class="entry-list" ref="listEl" tabindex="0" @keydown="onListKey">
            <div v-for="(e, i) in entries" :key="i" class="entry-item" :class="{ active: i === selIdx }" @click="selIdx = i">
              <div class="entry-item-top">
                <span class="badge sm" :class="e.dir === 'req' ? 'accent' : e.status >= 200 && e.status < 300 ? 'ok' : 'err'">
                  {{ e.dir === 'req' ? '↑' : '↓' + (e.status || '') }}
                </span>
                <span class="entry-item-model" :title="e.model">{{ e.model || '—' }}</span>
                <span class="entry-item-size" :title="e.size ? e.size + ' B' : ''">{{ fmtSize(e.size) }}</span>
                <span class="entry-item-time">{{ fmtShort(e.ts) }}</span>
              </div>
              <div v-if="e.transform || e.stream" class="entry-item-sub">
                <span v-if="e.transform">⇄ {{ e.transform }}</span>
                <span v-if="e.stream">{{ t('recordings.list.stream') }}</span>
              </div>
            </div>
            <div v-if="!entries.length" class="desc" style="text-align:center;padding:30px 10px">
              {{ selected ? (keyword ? t('recordings.list.emptyNoMatch') : t('recordings.list.emptyFile')) : t('recordings.list.emptyNoFile') }}
            </div>
          </div>
        </div>

        <!-- 可拖动分隔条：拖动调宽 / 中间按钮折叠列表 -->
        <div class="splitter" :title="listHidden ? '' : t('shell.dragResize')" @mousedown="onSplitterDown">
          <button class="split-toggle" :title="listHidden ? t('recordings.list.expandList') : t('recordings.list.collapseList')"
                  @mousedown.stop @click="toggleList">
            <ChevronLeft v-if="!listHidden" :size="16" :stroke-width="2.5" />
            <ChevronRight v-else :size="16" :stroke-width="2.5" />
          </button>
        </div>

        <div class="detail">
          <template v-if="sel">
            <div class="detail-head">
              <span class="badge" :class="sel.dir === 'req' ? 'accent' : sel.status >= 200 && sel.status < 300 ? 'ok' : 'err'">
                {{ sel.dir === 'req' ? t('recordings.detail.req') : t('recordings.detail.resp', { status: sel.status || '' }) }}
              </span>
              <span class="badge" v-if="sel.model">{{ sel.model }}</span>
              <span class="badge" v-if="sel.size" :title="sel.size + ' B'">{{ fmtSize(sel.size) }}</span>
              <span class="badge warn" v-if="sel.transform" :title="t('recordings.detail.transformBadge', { tag: sel.transform })">⇄ {{ sel.transform }}</span>
              <span class="badge" v-if="sel.stream">{{ t('recordings.detail.stream') }}</span>
              <span class="detail-time">{{ fmtTime(sel.ts) }}</span>
              <span class="detail-actions">
                <button class="btn sm" :class="{ on: viewMode === 'pretty' }" @click="viewMode = 'pretty'">{{ t('recordings.detail.pretty') }}</button>
                <button class="btn sm" :class="{ on: viewMode === 'raw' }" @click="viewMode = 'raw'">{{ t('recordings.detail.raw') }}</button>
                <template v-if="viewMode === 'raw' && isSSEBody">
                  <span class="v-sep"></span>
                  <button class="btn sm" :class="{ on: !sseCompact }" @click="setSseCompact(false)" :title="t('recordings.detail.framePrettyTip')">{{ t('recordings.detail.framePretty') }}</button>
                  <button class="btn sm" :class="{ on: sseCompact }" @click="setSseCompact(true)" :title="t('recordings.detail.frameCompactTip')">{{ t('recordings.detail.frameCompact') }}</button>
                </template>
                <button class="btn sm" :class="{ on: searchOpen }" @click="toggleSearch" :title="t('recordings.detail.searchInPage')"><Search :size="13" /></button>
                <button class="btn sm" @click="copyBody"><Copy :size="13" /> {{ t('common.copy') }}</button>
              </span>
            </div>
            <!-- 页内搜索行：Highlight API 高亮，命中仅限已渲染内容 -->
            <div v-if="searchOpen" class="search-bar">
              <input ref="searchInputEl" v-model="searchQ" type="text" class="input search-input"
                     :placeholder="t('recordings.detail.searchPh')"
                     @keydown.enter.prevent="stepMatch($event.shiftKey ? -1 : 1)"
                     @keydown.esc="closeSearch" />
              <span class="search-count" :title="matchRanges.length >= MAX_SEARCH_HITS ? t('recordings.detail.hitsCap', { n: MAX_SEARCH_HITS }) : ''">
                {{ matchRanges.length ? matchIdx + 1 : 0 }}/{{ matchRanges.length }}
              </span>
              <button class="btn sm" :disabled="!matchRanges.length" @click="stepMatch(-1)" :title="t('recordings.detail.prevMatch')"><ChevronUp :size="13" /></button>
              <button class="btn sm" :disabled="!matchRanges.length" @click="stepMatch(1)" :title="t('recordings.detail.nextMatch')"><ChevronDown :size="13" /></button>
              <button class="btn sm" @click="closeSearch" :title="t('recordings.detail.closeSearch')"><X :size="13" /></button>
            </div>
            <div class="detail-body" ref="detailBodyEl">
              <div v-if="detailLoading" class="detail-empty">{{ t('recordings.detail.loadingBody') }}</div>
              <template v-else>
                <!-- 原始视图：JSON 美化 + 语法高亮 -->
                <template v-if="viewMode === 'raw'">
                  <div v-if="rawView.note" class="fmt-note">{{ rawView.note }}</div>
                  <!-- a11y-fix: 巨型 v-html DOM（数万节点）会导致 WebView2 浏览器进程在 UIA
                       无障碍树构建/序列化时阻塞 UI 线程（输入路由瘫痪、系统级卡顿），
                       aria-hidden 将纯展示性的机器转储内容从无障碍树剔除，是标准缓解手段。 -->
                  <pre class="raw-pre" aria-hidden="true" v-html="rawView.html"></pre>
                </template>
                <template v-else-if="formatted && formatted.ok">
                  <div v-if="formatted.note" class="fmt-note">{{ formatted.note }}</div>
                  <div v-if="formatted.meta.length" class="fmt-meta">
                    <span v-for="m in formatted.meta" :key="m[0]" class="meta-chip">{{ m[0] }}: {{ m[1] }}</span>
                  </div>
                  <div v-for="(m, i) in formatted.msgs" :key="i" class="fmt-msg">
                    <span class="fmt-role" :class="m.role">{{ m.role }}</span>
                    <div class="fmt-text" :class="m.role">{{ m.text }}</div>
                  </div>
                  <div v-if="formatted.usage" class="fmt-usage">
                    <span class="badge">↑ {{ formatted.usage.p }} tokens</span>
                    <span class="badge">↓ {{ formatted.usage.c }} tokens</span>
                  </div>
                  <pre v-if="formatted.pretty" class="raw-pre" aria-hidden="true" v-html="prettyHtml"></pre>
                </template>
                <template v-else>
                  <div class="fmt-note">{{ formatted?.note || t('recordings.detail.cannotFormat') }}</div>
                  <pre class="raw-pre" aria-hidden="true" v-html="rawView.html"></pre>
                </template>
              </template>
            </div>
          </template>
          <div v-else class="detail-empty">{{ t('recordings.detail.selectEntry') }}</div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ChevronDown, ChevronLeft, ChevronRight, ChevronUp, Copy, FolderOpen, Search, Settings2, RefreshCw, Trash2, X } from 'lucide-vue-next'
import {
  CopyText, DeleteRecording, GetRecordInfo, ListRecordings, OpenRecordDir, PickRecordDir,
  ReadRecordingEntry, ReadRecordings, SaveConfig
} from '../../wailsjs/go/main/App'
import { useAppStore } from '../stores/app'
import { fmtTimeHM } from '../utils/format'
import CopyField from '../components/CopyField.vue'

interface RecFile { date: string; sizeBytes: number; lines: number }
interface RecEntry { ts: string; dir: string; model: string; status: number; stream: boolean; transform?: string; size?: number; line: number; body?: string }
interface RecInfo { enabled: boolean; file: string; size: number }

const app = useAppStore()
const { t } = useI18n({ useScope: 'global' })
const files = ref<RecFile[]>([])
const selected = ref('')
const keyword = ref('')
const entries = ref<RecEntry[]>([])
const selIdx = ref(-1)
const viewMode = ref<'pretty' | 'raw'>('pretty')
const listEl = ref<HTMLElement | null>(null)

// 详情正文：列表只回元信息 + 行号，选中后按需拉取完整内容（不截断）
const detailBody = ref('')
const detailLoading = ref(false)

const sel = computed<RecEntry | null>(() =>
  selIdx.value >= 0 && selIdx.value < entries.value.length ? entries.value[selIdx.value] : null)

// ---- 列表宽度拖拽 / 折叠（偏好持久化） ----
const browserEl = ref<HTMLElement | null>(null)
const listW = ref(Math.min(560, Math.max(200, Number(localStorage.getItem('rec-list-w')) || 300)))
const listHidden = ref(localStorage.getItem('rec-list-hidden') === '1')
const dragging = ref(false)

function toggleList() {
  listHidden.value = !listHidden.value
  localStorage.setItem('rec-list-hidden', listHidden.value ? '1' : '0')
}

function onSplitterDown(e: MouseEvent) {
  if (listHidden.value || e.button !== 0) return
  dragging.value = true
  e.preventDefault()
  const move = (ev: MouseEvent) => {
    const rect = browserEl.value?.getBoundingClientRect()
    if (!rect) return
    const max = Math.min(560, rect.width - 360) // 右侧详情至少留 360px
    listW.value = Math.max(200, Math.min(max, ev.clientX - rect.left))
  }
  const up = () => {
    dragging.value = false
    document.removeEventListener('mousemove', move)
    document.removeEventListener('mouseup', up)
    localStorage.setItem('rec-list-w', String(listW.value))
  }
  document.addEventListener('mousemove', move)
  document.addEventListener('mouseup', up)
}

// ---- 消息录制（开关与设置） ----
const recInfo = ref<RecInfo | null>(null)
const recEnabled = ref(false)
const recDir = ref('')
const retentionDays = ref(30)
const settingsOpen = ref(false)

// 配置在 App.vue 中异步加载；监听它以避免页面先挂载时停留在默认值。
watch(() => app.config?.recordRetentionDays, days => {
  if (days !== undefined) retentionDays.value = days
}, { immediate: true })

function fmtTime(ts: string): string {
  return fmtTimeHM(ts)
}

function fmtShort(t: string): string {
  const d = new Date(t)
  return isNaN(d.getTime()) ? '' : d.toTimeString().slice(0, 8)
}

// 原始消息字节数的自适应单位（B / KB / MB；10KB 内保留 1 位小数）
function fmtSize(v?: number): string {
  if (!v || v <= 0) return ''
  if (v < 1024) return `${v} B`
  if (v < 10 * 1024) return `${(v / 1024).toFixed(1)} KB`
  if (v < 1024 * 1024) return `${Math.round(v / 1024)} KB`
  return `${(v / 1048576).toFixed(1)} MB`
}

// ---- 详情格式化：把各协议的请求/响应体归一化为 role + 文本 ----

interface FmtMsg { role: string; text: string }
interface Formatted {
  ok: boolean
  note?: string
  msgs: FmtMsg[]
  meta: [string, string][]
  usage?: { p: number; c: number }
  pretty?: string
}

const formatted = computed<Formatted | null>(() => (sel.value ? formatBody(sel.value, detailBody.value) : null))

// 选中条目变化 → 按行号拉取完整正文（并发点击时丢弃过期结果）
let detailSeq = 0
watch(sel, e => { void loadDetail(e) })

async function loadDetail(e: RecEntry | null) {
  const seq = ++detailSeq
  if (!e || !selected.value) {
    detailBody.value = ''
    detailLoading.value = false
    return
  }
  detailLoading.value = true
  try {
    const res = JSON.parse(await ReadRecordingEntry(selected.value, e.line))
    if (seq !== detailSeq) return
    if (res.success) {
      detailBody.value = res.data.body || ''
    } else {
      detailBody.value = ''
      app.toast(res.error, 'err')
    }
  } catch {
    if (seq === detailSeq) detailBody.value = ''
  } finally {
    if (seq === detailSeq) detailLoading.value = false
  }
}

// ---- 原始视图：JSON 美化 + 轻量语法高亮（超大内容降级为纯文本） ----

function escapeHtml(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

function highlightJSON(s: string): string {
  return escapeHtml(s).replace(
    /("(?:\\.|[^"\\])*")(\s*:)?|\b(true|false|null)\b|(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)/g,
    (m, str: string, colon: string, kw: string, num: string) => {
      if (str) return colon ? `<span class="j-key">${str}</span>${colon}` : `<span class="j-str">${str}</span>`
      if (kw) return `<span class="j-kw">${kw}</span>`
      if (num) return `<span class="j-num">${num}</span>`
      return m
    })
}

// highlightSSEData 在通用 JSON 高亮之上，给关键语义字段的值叠加专属配色（单趟正则完成）
const SSE_KEY_CLASS: Record<string, string> = {
  reasoning_content: 'sse-think', reasoning: 'sse-think', thinking: 'sse-think', // 推理增量
  partial_json: 'sse-tool',                                                     // 工具参数增量
  content: 'sse-reply', text: 'sse-reply',                                      // 正文增量
  finish_reason: 'sse-finish', stop_reason: 'sse-finish',                       // 结束原因
}
function highlightSSEData(jsonText: string): string {
  return highlightJSON(jsonText).replace(
    /<span class="j-key">"(reasoning_content|reasoning|thinking|partial_json|content|text|finish_reason|stop_reason)"<\/span>(\s*:\s*)<span class="j-str">/g,
    (m, key: string, colon: string) => `<span class="j-key">"${key}"</span>${colon}<span class="j-str ${SSE_KEY_CLASS[key]}">`
  )
}

// SSE 事件类别 → 颜色 class（开始/结束/块边界）
function sseEventClass(evt: string): string {
  if (evt === 'message_start' || evt === 'content_block_start') return 'sse-start'
  if (evt === 'message_stop' || evt === 'content_block_stop') return 'sse-stop'
  if (evt === 'message_delta' || evt === 'ping') return 'sse-block'
  return ''
}

// 性能护栏：SSE 只渲染前 MAX_SSE_FRAMES 帧、单帧 data 超长截断、总渲染量封顶，纯文本降级最多 ESCAPE_CAP 字符。
// 此前整个 body（可达数 MB、数千帧）一次性同步正则 + v-html 插入单个 <pre>，
// 海量 <span> 的 DOM 布局/销毁会卡死主线程；且巨型帧（超大工具参数等）在「单行」
// 模式下形成数 MB 宽的单行，文本排版（shaping）耗时数秒，「帧格式化」模式下膨胀出
// 几十万内联节点，后续滚动/点击的命中测试与重排长时间占满 CPU——切换模式时尤为明显，
// 内存压力还会拖慢整个系统。故除帧数外还需限制单帧载荷与总输出量。
const MAX_SSE_FRAMES = 300
const MAX_FRAME_DATA = 16384      // 单帧 data 载荷上限（字符），超出截断显示（完整内容可复制）
const MAX_SSE_RENDER = 1 << 20    // 渲染总输出上限（字符），限制 DOM 节点总量
const ESCAPE_CAP = 1 << 20
// 非 SSE 的美化 JSON 路径：单行截断阈值（字符），防超长字符串值拖垮排版
const COMPACT_LINE_MAX = 2000

// highlightSSEBody 把 SSE 原文渲染为逐帧结构：事件行着色 + 帧内 JSON 高亮。
// compact=false 每帧 JSON 美化多行；compact=true 每帧压成一行（紧凑原文直接着色，不解析）。
// 超过 MAX_SSE_FRAMES 帧时保留头尾各半（开始事件在头、结束事件在尾），中段折叠提示；
// 单帧 data 超过 MAX_FRAME_DATA 截断显示；累计输出超过 MAX_SSE_RENDER 后省略剩余帧。
function highlightSSEBody(body: string, compact: boolean): string {
  const parts = body.split(/\n\n+/)
  const half = Math.floor(MAX_SSE_FRAMES / 2)
  let shown = parts
  let omitted = 0
  if (parts.length > MAX_SSE_FRAMES) {
    shown = [...parts.slice(0, half), ...parts.slice(-half)]
    omitted = parts.length - shown.length
  }
  const out: string[] = []
  let budget = MAX_SSE_RENDER // 剩余渲染配额（按生成 HTML 的字符数计）
  let overflow = false
  const renderFrame = (part: string) => {
    if (budget <= 0) { overflow = true; return }
    let evt = ''
    const dataLines: string[] = []
    const extras: string[] = []
    for (const ln of part.split('\n')) {
      if (ln.startsWith('event:')) evt = ln.slice(6).trim()
      else if (ln.startsWith('data:')) dataLines.push(ln.slice(5).trim())
      else if (ln.trim()) extras.push(escapeHtml(ln))
    }
    const segs: string[] = [...extras]
    if (evt) segs.push(`<span class="sse-evt ${sseEventClass(evt)}">▌ ${escapeHtml(evt)}</span>`)
    for (const d of dataLines) {
      if (d === '[DONE]') {
        segs.push('<span class="sse-evt sse-stop">▌ [DONE]</span>')
        continue
      }
      if (d.length > MAX_FRAME_DATA) {
        // 巨型帧：截断后按紧凑高亮，避免超长单行拖垮布局（不再 JSON.parse 数 MB 字符串）
        segs.push(highlightSSEData(d.slice(0, MAX_FRAME_DATA))
          + ` <span class="sse-evt">${escapeHtml(t('recordings.view.frameTruncated', { n: d.length }))}</span>`)
        continue
      }
      if (compact) {
        segs.push(highlightSSEData(d)) // 紧凑原文直接高亮（≤MAX_FRAME_DATA，已由上方护栏保证）
      } else {
        try {
          segs.push(highlightSSEData(JSON.stringify(JSON.parse(d), null, 2)))
        } catch {
          segs.push(escapeHtml(d))
        }
      }
    }
    const html = compact ? segs.join(' ') : segs.join('\n')
    out.push(html)
    budget -= html.length
  }
  const overflowNote = `<span class="sse-evt">${escapeHtml(t('recordings.view.renderOverflow', { n: MAX_FRAME_DATA }))}</span>`
  if (omitted > 0) {
    for (const p of parts.slice(0, half)) { renderFrame(p); if (overflow) break }
    if (!overflow) out.push(`<span class="sse-evt">${escapeHtml(t('recordings.view.middleFolded', { omitted, total: parts.length, half }))}</span>`)
    if (!overflow) for (const p of parts.slice(-half)) { renderFrame(p); if (overflow) break }
  } else {
    for (const p of shown) { renderFrame(p); if (overflow) break }
  }
  if (overflow) out.push(overflowNote)
  return out.join('\n')
}

// capText 截断超长纯文本渲染（避免百万级字符的巨型文本节点拖垮布局）
function capText(s: string, max: number): string {
  return s.length <= max ? s : s.slice(0, max) +
    '\n' + t('recordings.view.capText', { shown: (max / 1048576).toFixed(1), total: (s.length / 1048576).toFixed(1) })
}

// capLine 截断"单行过长"：美化 JSON 的超长字符串值是一整行，数万字符的单行
// 文本排版（shaping）与命中测试开销极大（非 SSE 路径的通用护栏）
function capLine(line: string): string {
  return line.length > COMPACT_LINE_MAX
    ? line.slice(0, COMPACT_LINE_MAX) + ' ' + t('recordings.view.capLine', { n: (line.length - COMPACT_LINE_MAX).toLocaleString() })
    : line
}

const rawView = computed<{ html: string; note?: string }>(() => {
  const b = detailBody.value
  if (!b) return { html: '' }
  if (b.length > 4194304) {
    return { html: escapeHtml(capText(b, ESCAPE_CAP)), note: t('recordings.view.over4MB', { mb: (b.length / 1048576).toFixed(1) }) }
  }
  if (isSSEBody.value) {
    return {
      html: highlightSSEBody(b, sseCompact.value),
      note: sseCompact.value
        ? t('recordings.view.sseCompact')
        : t('recordings.view.ssePretty'),
    }
  }
  try {
    const pretty = JSON.stringify(JSON.parse(b), null, 2)
    // 逐行截断：超长字符串值是单行数万字符，排版（shaping）开销极大
    const capped = pretty.split('\n').map(l => capLine(l)).join('\n')
    if (capped.length > 1048576) {
      return { html: escapeHtml(capText(capped, ESCAPE_CAP)), note: t('recordings.view.json1MB') }
    }
    return { html: highlightJSON(capped) }
  } catch {
    return { html: escapeHtml(capText(b, ESCAPE_CAP)), note: t('recordings.view.notJson') }
  }
})

const prettyHtml = computed(() => {
  const p = formatted.value?.pretty
  if (!p) return ''
  // 与 rawView 非 SSE 路径同款护栏：逐行截断后再按总量决定是否着色
  const capped = p.split('\n').map(l => capLine(l)).join('\n')
  return capped.length > 1048576 ? escapeHtml(capped) : highlightJSON(capped)
})

// SSE 原始视图的子模式：帧格式化（默认）/ 单行紧凑（均保留高亮）
const sseCompact = ref(localStorage.getItem('rec-sse-compact') === '1')
function setSseCompact(v: boolean) {
  sseCompact.value = v
  localStorage.setItem('rec-sse-compact', v ? '1' : '0')
}
const isSSEBody = computed(() => {
  const b = detailBody.value
  return !!b && (b.startsWith('data:') || b.includes('\ndata:'))
})

// ---- 详情页内搜索（CSS Custom Highlight API，零 DOM 改写） ----
// 高亮用 CSS.highlights 注册 Range 实现，避免向巨型 v-html DOM 插入 <mark>
// （Vue 重渲染会打乱手改的 DOM，且数 MB 内容的 wrap/unwrap 是已知性能雷区）。
// 注意：SSE 折叠/截断的帧不在 DOM 中，只能命中已渲染内容。

const searchOpen = ref(false)
const searchQ = ref('')
const matchRanges = ref<Range[]>([])
const matchIdx = ref(-1)
const detailBodyEl = ref<HTMLElement | null>(null)
const searchInputEl = ref<HTMLInputElement | null>(null)
// 性能护栏：封顶命中 Range 数，与既有渲染护栏（MAX_SSE_FRAMES 等）同思路
const MAX_SEARCH_HITS = 2000

const HL_ALL = 'rec-search-hit'
const HL_CUR = 'rec-search-hit-cur'

function clearHighlights() {
  if (typeof CSS !== 'undefined' && CSS.highlights) {
    CSS.highlights.delete(HL_ALL)
    CSS.highlights.delete(HL_CUR)
  }
}

// applySearch 遍历详情区文本节点收集命中 Range（大小写不敏感，单节点内匹配），
// 全部命中注册为 rec-search-hit，当前条单独注册为 rec-search-hit-cur。
function applySearch() {
  clearHighlights()
  const q = searchQ.value.trim().toLowerCase()
  if (!q || !detailBodyEl.value) {
    matchRanges.value = []
    matchIdx.value = -1
    return
  }
  const ranges: Range[] = []
  const walker = document.createTreeWalker(detailBodyEl.value, NodeFilter.SHOW_TEXT)
  for (let node = walker.nextNode() as Text | null; node; node = walker.nextNode() as Text | null) {
    const text = node.data.toLowerCase()
    let from = 0
    let at = text.indexOf(q, from)
    while (at !== -1) {
      const r = document.createRange()
      r.setStart(node, at)
      r.setEnd(node, at + q.length)
      ranges.push(r)
      if (ranges.length >= MAX_SEARCH_HITS) break
      from = at + q.length
      at = text.indexOf(q, from)
    }
    if (ranges.length >= MAX_SEARCH_HITS) break
  }
  matchRanges.value = ranges
  matchIdx.value = ranges.length ? 0 : -1
  registerCurrent()
  if (ranges.length) registerAll()
}

function registerAll() {
  if (typeof CSS === 'undefined' || !CSS.highlights) return
  CSS.highlights.set(HL_ALL, new Highlight(...matchRanges.value))
}

function registerCurrent() {
  if (typeof CSS === 'undefined' || !CSS.highlights) return
  const cur = matchRanges.value[matchIdx.value]
  if (cur) CSS.highlights.set(HL_CUR, new Highlight(cur))
  else CSS.highlights.delete(HL_CUR)
}

// scrollMatchIntoView 手动滚动容器使当前命中居中（Range 无 scrollIntoView）
function scrollMatchIntoView() {
  const r = matchRanges.value[matchIdx.value]
  const box = detailBodyEl.value
  if (!r || !box) return
  const rect = r.getBoundingClientRect()
  const boxRect = box.getBoundingClientRect()
  if (rect.top < boxRect.top || rect.bottom > boxRect.bottom) {
    box.scrollTop += rect.top - boxRect.top - box.clientHeight / 2 + rect.height / 2
  }
}

function stepMatch(d: 1 | -1) {
  const n = matchRanges.value.length
  if (!n) return
  matchIdx.value = (matchIdx.value + d + n) % n
  registerCurrent()
  scrollMatchIntoView()
}

function toggleSearch() {
  searchOpen.value ? closeSearch() : openSearch()
}

function openSearch() {
  searchOpen.value = true
  void nextTick(() => searchInputEl.value?.focus())
}

function closeSearch() {
  searchOpen.value = false
  searchQ.value = ''
  clearHighlights()
  matchRanges.value = []
  matchIdx.value = -1
}

// Ctrl+F 开关页内搜索（Wails WebView2 无原生查找栏，preventDefault 保持行为一致）
function onGlobalKey(e: KeyboardEvent) {
  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'f') {
    e.preventDefault()
    toggleSearch()
  }
}

// 内容（视图切换 / SSE 子模式 / 换条目）或关键字变化 → 重算命中。
// rawView.html / prettyHtml 已覆盖 sel / viewMode / sseCompact / detailBody 的依赖。
watch([() => rawView.value.html, prettyHtml, searchQ], () => { void nextTick(applySearch) })

// textOfContent 归一化消息内容为纯文本（字符串 / Anthropic blocks / Gemini parts / 其它对象）
function textOfContent(c: unknown): string {
  if (c == null) return ''
  if (typeof c === 'string') return c
  if (Array.isArray(c)) {
    return c.map(b => {
      if (typeof b === 'string') return b
      if (b && typeof b === 'object') {
        const o = b as Record<string, unknown>
        if (typeof o.text === 'string') return o.text
        if (typeof o.content === 'string') return o.content
        if (o.type === 'tool_use' || o.type === 'tool_call') {
          const input = o.input ?? o.arguments
          return t('recordings.view.toolCall', { name: String(o.name ?? '') }) + (input == null ? '' : ' ' + JSON.stringify(input, null, 2))
        }
        if (o.type) return `[${String(o.type)}]`
      }
      return JSON.stringify(b)
    }).filter(Boolean).join('\n')
  }
  return JSON.stringify(c, null, 2)
}

// toolCallsText 渲染 openai 风格 tool_calls（arguments 可能是 JSON 字符串）
function toolCallsText(tcs: unknown[]): string {
  return tcs.map(tc => {
    const fn = (tc as any)?.function
    if (!fn) return JSON.stringify(tc, null, 2)
    let args: string
    if (typeof fn.arguments === 'string') {
      try {
        args = JSON.stringify(JSON.parse(fn.arguments), null, 2)
      } catch {
        args = fn.arguments
      }
    } else {
      args = JSON.stringify(fn.arguments ?? {}, null, 2)
    }
    return `[${(tc as any).type ?? 'function'}] ${fn.name ?? ''}\n${args}`
  }).join('\n\n')
}

// msgsFromAnthropicBlocks 把 anthropic 响应 content 块展开为 thinking/assistant/tool_use 消息
function msgsFromAnthropicBlocks(blocks: unknown[]): FmtMsg[] {
  const out: FmtMsg[] = []
  for (const b of blocks) {
    const o = b as Record<string, unknown> | null
    if (!o || typeof o !== 'object') continue
    switch (o.type) {
      case 'text':
        if (o.text) out.push({ role: 'assistant', text: String(o.text) })
        break
      case 'thinking':
        if (o.thinking) out.push({ role: 'thinking', text: String(o.thinking) })
        break
      case 'tool_use':
        out.push({ role: 'tool_use', text: textOfContent([o]) })
        break
      default:
        out.push({ role: String(o.type ?? 'block'), text: JSON.stringify(o, null, 2) })
    }
  }
  return out
}

function usageOf(u: Record<string, unknown> | undefined | null): { p: number; c: number } | undefined {
  if (!u) return undefined
  const p = u.prompt_tokens ?? u.input_tokens ?? u.promptTokenCount
  const c = u.completion_tokens ?? u.output_tokens ?? u.candidatesTokenCount
  if (p == null && c == null) return undefined
  return { p: Number(p) || 0, c: Number(c) || 0 }
}

function formatBody(e: RecEntry, bodyRaw: string): Formatted {
  const body = (bodyRaw || '').trim()
  if (!body) return { ok: false, note: t('recordings.view.emptyBody'), msgs: [], meta: [] }
  // 流式响应录制的是 data: 帧序列
  if (e.dir === 'resp' && (body.startsWith('data:') || body.includes('\ndata:'))) {
    return formatSSE(body)
  }
  let j: any
  try {
    j = JSON.parse(body)
  } catch {
    return { ok: false, note: t('recordings.view.notJsonStructured'), msgs: [], meta: [] }
  }
  const meta: [string, string][] = []
  const msgs: FmtMsg[] = []

  if (e.dir === 'req') {
    if (typeof j.model === 'string') meta.push(['model', j.model])
    if (j.max_tokens != null) meta.push(['max_tokens', String(j.max_tokens)])
    if (j.temperature != null) meta.push(['temperature', String(j.temperature)])
    if (Array.isArray(j.tools)) meta.push(['tools', String(j.tools.length)])
    // system：anthropic 字符串/blocks、gemini systemInstruction（openai 的 system 在 messages 里）
    let sys = ''
    if (typeof j.system === 'string') sys = j.system
    else if (Array.isArray(j.system)) sys = textOfContent(j.system)
    else if (j.systemInstruction) sys = textOfContent((j.systemInstruction as any).content ?? j.systemInstruction)
    if (sys) msgs.push({ role: 'system', text: sys })
    if (Array.isArray(j.messages)) { // openai / anthropic
      for (const m of j.messages) {
        let text = textOfContent(m?.content)
        if (!text && Array.isArray(m?.tool_calls)) text = toolCallsText(m.tool_calls)
        msgs.push({ role: String(m?.role ?? '?'), text })
      }
    } else if (Array.isArray(j.contents)) { // gemini
      for (const m of j.contents) msgs.push({ role: String(m?.role ?? 'user'), text: textOfContent(m?.parts) })
    }
    if (msgs.length) return { ok: true, msgs, meta }
    return { ok: true, note: t('recordings.view.noMsgStruct'), msgs: [], meta, pretty: JSON.stringify(j, null, 2) }
  }

  // 非流式响应：openai choices / anthropic content blocks / gemini candidates
  if (Array.isArray(j.choices)) {
    const msg = j.choices[0]?.message
    if (msg) {
      const reasoning = msg.reasoning_content ?? msg.reasoning
      if (typeof reasoning === 'string' && reasoning) msgs.push({ role: 'thinking', text: reasoning })
      const c = textOfContent(msg.content)
      if (c) msgs.push({ role: 'assistant', text: c })
      if (Array.isArray(msg.tool_calls) && msg.tool_calls.length) {
        msgs.push({ role: 'tool_use', text: toolCallsText(msg.tool_calls) })
      }
    } else if (typeof j.choices[0]?.text === 'string') {
      msgs.push({ role: 'assistant', text: j.choices[0].text })
    }
  } else if (Array.isArray(j.content)) {
    msgs.push(...msgsFromAnthropicBlocks(j.content))
  } else if (Array.isArray(j.candidates)) {
    const t = textOfContent(j.candidates[0]?.content?.parts)
    if (t) msgs.push({ role: 'assistant', text: t })
  }
  if (typeof j.model === 'string') meta.push(['model', j.model])
  const finish = j.finish_reason ?? j.stop_reason
  if (finish) meta.push(['finish', String(finish)])
  const usage = usageOf(j.usage ?? j.usageMetadata)
  if (msgs.length || usage) return { ok: true, msgs, meta, usage }
  return { ok: true, note: t('recordings.view.noRespStruct'), msgs: [], meta, pretty: JSON.stringify(j, null, 2) }
}

// formatSSE 逐帧解析并汇总增量：正文 / 思考（reasoning）/ 工具调用
// （anthropic 工具参数走 input_json_delta 的 partial_json 增量，openai 走 delta.tool_calls）
function formatSSE(body: string): Formatted {
  let text = ''
  let reasoning = ''
  let frames = 0
  let usage: { p: number; c: number } | undefined
  const tools = new Map<number, { name: string; args: string }>() // 块 index → 工具累积

  for (const part of body.split(/\n\n+/)) {
    for (const line of part.split('\n')) {
      if (!line.startsWith('data:')) continue
      const data = line.slice(5).trim()
      if (!data || data === '[DONE]') continue
      frames++
      let j: any
      try {
        j = JSON.parse(data)
      } catch { continue }

      // anthropic content_block_start：登记 tool_use 块（拿到工具名）
      if (j?.type === 'content_block_start' && j?.content_block?.type === 'tool_use') {
        tools.set(typeof j.index === 'number' ? j.index : 0, {
          name: String(j.content_block.name ?? ''), args: '',
        })
      }

      const delta = j?.choices?.[0]?.delta
      const think = delta?.reasoning_content ?? delta?.reasoning ?? j?.delta?.thinking
      if (typeof think === 'string') reasoning += think
      const c = delta?.content ?? j?.choices?.[0]?.text
        ?? j?.delta?.text ?? j?.candidates?.[0]?.content?.parts?.[0]?.text
      if (typeof c === 'string') text += c

      // anthropic input_json_delta：工具参数 JSON 的增量分片
      const pj = j?.delta?.partial_json
      if (typeof pj === 'string') {
        const idx = typeof j?.index === 'number' ? j.index : 0
        const t = tools.get(idx) ?? { name: '', args: '' }
        t.args += pj
        tools.set(idx, t)
      }
      // openai delta.tool_calls：增量工具调用（name 分段 + arguments 分段）
      if (Array.isArray(delta?.tool_calls)) {
        for (const tc of delta.tool_calls) {
          const idx = typeof tc?.index === 'number' ? tc.index : 0
          const t = tools.get(idx) ?? { name: '', args: '' }
          if (typeof tc?.function?.name === 'string') t.name += tc.function.name
          if (typeof tc?.function?.arguments === 'string') t.args += tc.function.arguments
          tools.set(idx, t)
        }
      }
      usage = usageOf(j?.usage ?? j?.usageMetadata ?? j?.message?.usage) ?? usage
    }
  }

  const msgs: FmtMsg[] = []
  if (reasoning) msgs.push({ role: 'thinking', text: reasoning })
  if (text) msgs.push({ role: 'assistant', text })
  // 循环变量禁用 t（会遮蔽 i18n 的 t 函数，t(...) 即 TypeError——工具调用帧全走此循环）
  for (const tool of tools.values()) {
    let args = tool.args
    try {
      args = JSON.stringify(JSON.parse(tool.args), null, 2) // 增量拼回完整 JSON 后美化
    } catch { /* 拼接不完整（截断）：按原样展示 */ }
    msgs.push({ role: 'tool_use', text: t('recordings.view.toolCall', { name: tool.name || '?' }) + `\n${args}` })
  }
  const kinds = [reasoning && t('recordings.view.kindThink'), text && t('recordings.view.kindText'), tools.size && t('recordings.view.kindTool')].filter(Boolean).join(' + ')
  return {
    ok: true,
    note: t('recordings.view.sseNote', { frames, kinds: kinds || t('recordings.view.kindNone') }),
    msgs, meta: [], usage,
  }
}

// ---- 左列表交互 ----

async function copyBody() {
  if (!sel.value) return
  const res = JSON.parse(await CopyText(detailBody.value || ''))
  if (res.success) app.toast(t('recordings.toast.copiedRaw'))
}

function onListKey(e: KeyboardEvent) {
  if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return
  e.preventDefault()
  if (!entries.value.length) return
  const dir = e.key === 'ArrowDown' ? 1 : -1
  const cur = selIdx.value < 0 ? (dir > 0 ? -1 : 1) : selIdx.value
  selIdx.value = Math.min(entries.value.length - 1, Math.max(0, cur + dir))
  void nextTick(() => listEl.value?.querySelector('.entry-item.active')?.scrollIntoView({ block: 'nearest' }))
}

async function refreshRecord() {
  try {
    const res = JSON.parse(await GetRecordInfo())
    if (res.success) {
      recInfo.value = res.data
      recEnabled.value = res.data.enabled
    }
  } catch { /* ignore */ }
}

async function saveRecord() {
  if (!app.config) return
  const res = JSON.parse(await SaveConfig(JSON.stringify({
    ...app.config,
    recordEnabled: recEnabled.value
  })))
  if (res.success) {
    app.config = res.data
    app.toast(recEnabled.value ? t('recordings.toast.recOn') : t('recordings.toast.recOff'))
    refreshRecord()
  } else {
    app.toast(res.error, 'err')
  }
}

async function pickDir() {
  const res = JSON.parse(await PickRecordDir())
  if (!res.success || !res.data.dir) return
  recDir.value = res.data.dir
  if (!app.config) return
  const saved = JSON.parse(await SaveConfig(JSON.stringify({
    ...app.config, recordDir: recDir.value
  })))
  if (saved.success) {
    app.config = saved.data
    app.toast(t('recordings.toast.dirUpdated', { dir: recDir.value }))
    refreshRecord()
    selected.value = '' // 目录变了，重置选中后重载列表
    loadFiles()
  }
}

async function openDir() {
  const res = JSON.parse(await OpenRecordDir())
  if (!res.success) app.toast(res.error, 'err')
}

async function saveRetention() {
  if (!app.config) return
  const v = Math.min(3650, Math.max(1, Math.round(retentionDays.value || 30)))
  retentionDays.value = v
  const res = JSON.parse(await SaveConfig(JSON.stringify({
    ...app.config, recordRetentionDays: v
  })))
  if (res.success) {
    app.config = res.data
    app.toast(t('recordings.toast.retentionSaved', { n: v }))
  } else {
    app.toast(res.error, 'err')
  }
}

async function loadFiles() {
  const res = JSON.parse(await ListRecordings())
  if (res.success) {
    files.value = res.data
    if (!selected.value && files.value.length) select(files.value[0].date)
  }
}

async function select(date: string) {
  selected.value = date
  keyword.value = ''
  await reloadEntries()
}

// 日期下拉切换
function onDateChange(ev: Event) {
  const v = (ev.target as HTMLSelectElement).value
  if (v) void select(v)
}

async function reloadEntries() {
  if (!selected.value) return
  const res = JSON.parse(await ReadRecordings(selected.value, keyword.value, 200))
  if (res.success) {
    entries.value = res.data
    selIdx.value = entries.value.length ? 0 : -1 // 自动选中最新一条
  } else {
    app.toast(res.error, 'err')
  }
}

async function del(date: string) {
  if (!(await app.confirm({ title: t('recordings.confirmTitle'), body: t('recordings.confirmBody', { date }), danger: true }))) return
  const res = JSON.parse(await DeleteRecording(date))
  if (res.success) {
    app.toast(t('recordings.toast.deleted', { date }))
    if (selected.value === date) { selected.value = ''; entries.value = []; selIdx.value = -1 }
    loadFiles()
  } else app.toast(res.error, 'err')
}

onMounted(() => {
  refreshRecord()
  loadFiles()
  window.addEventListener('keydown', onGlobalKey)
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onGlobalKey)
  clearHighlights()
})
</script>

<style scoped>
.rec-root { display: flex; flex-direction: column; height: calc(100vh - var(--titlebar-h) - 2 * var(--main-pad)); }
.card.shrink { flex-shrink: 0; }
.card.grow { flex: 1; min-height: 200px; display: flex; flex-direction: column; }
.locked-card { text-align: center; padding: 46px 24px; }
.lock-icon {
  width: 46px; height: 46px; margin: 0 auto 12px; border-radius: 12px;
  display: flex; align-items: center; justify-content: center; color: #fff;
  background: var(--grad-accent); box-shadow: 0 4px 20px rgba(99,102,241,.45);
}

/* ---- 单行工具栏 ---- */
.toolbar {
  display: flex; align-items: center; gap: 10px; flex-wrap: wrap;
  padding: 10px 14px;
}
.toolbar-search { margin-left: auto; display: flex; align-items: center; gap: 6px; }
.toolbar-search .input { width: 200px; }

/* ---- 设置面板（默认收起） ---- */
.settings-panel { padding: 12px 14px; }
.sp-row { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; margin-bottom: 10px; }
.sp-row:last-of-type { margin-bottom: 0; }
.sp-label { color: var(--text-dim); font-size: 12px; flex-shrink: 0; }

/* ---- 两栏浏览器 ---- */
.browser-card { padding: 0; overflow: hidden; }
.browser { flex: 1; min-height: 0; display: flex; }
.browser.dragging { cursor: col-resize; user-select: none; }
.browser.dragging * { cursor: col-resize !important; }

.left-col {
  flex-shrink: 0; display: flex; flex-direction: column; min-height: 0;
  border-right: 1px solid var(--border);
}
.file-header {
  display: flex; align-items: center; gap: 6px;
  padding: 8px 10px; border-bottom: 1px solid var(--border); background: var(--bg-elevated);
}
.file-select { flex: 1; min-width: 0; padding: 5px 8px; font-size: 12px; }
.file-del {
  background: transparent; border: none; color: var(--text-dim); cursor: pointer;
  display: inline-flex; padding: 4px; border-radius: 4px; flex-shrink: 0;
}
.file-del:hover:not(:disabled) { color: var(--red); background: rgba(248,113,113,.12); }
.file-del:disabled { opacity: .35; cursor: not-allowed; }

.entry-list { flex: 1; min-height: 0; overflow-y: auto; outline: none; }
.entry-item { padding: 8px 12px; border-bottom: 1px solid rgba(30,38,51,.5); cursor: pointer; }
.entry-item:hover { background: var(--bg-elevated); }
.entry-item.active { background: rgba(124,140,248,.1); box-shadow: inset 2px 0 0 var(--accent); }
.entry-item-top { display: flex; align-items: center; gap: 8px; min-width: 0; }
.entry-item-model {
  flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
  font-family: var(--mono); font-size: 11.5px; color: var(--accent);
}
.entry-item-time { font-family: var(--mono); font-size: 10.5px; color: var(--text-dim); }
.entry-item-size { font-family: var(--mono); font-size: 10.5px; color: var(--text-dim); opacity: .75; flex-shrink: 0; }
.entry-item-sub { display: flex; gap: 6px; margin-top: 3px; font-size: 10.5px; color: var(--text-dim); }
.badge.sm { padding: 1px 6px; font-size: 10px; }

.detail { flex: 1; min-width: 0; display: flex; flex-direction: column; }
.detail-head {
  display: flex; align-items: center; gap: 8px; flex-wrap: wrap;
  padding: 10px 14px; border-bottom: 1px solid var(--border); background: var(--bg-elevated);
}
.detail-time { font-family: var(--mono); font-size: 11px; color: var(--text-dim); }
.detail-actions { margin-left: auto; display: flex; gap: 6px; align-items: center; }
/* 页内搜索行（与 detail-head 同底色/边框，形成一条头部区） */
.search-bar {
  display: flex; align-items: center; gap: 6px;
  padding: 8px 14px; border-bottom: 1px solid var(--border); background: var(--bg-elevated);
}
.search-input { flex: 1; min-width: 0; }
.search-count {
  font-family: var(--mono); font-size: 11px; color: var(--text-dim);
  min-width: 44px; text-align: center; flex-shrink: 0; white-space: nowrap;
}
.v-sep { width: 1px; height: 16px; background: var(--border-bright); margin: 0 2px; }
.btn.sm { padding: 4px 10px; font-size: 11.5px; }
.btn.sm.on { border-color: var(--accent); color: var(--accent); background: rgba(124,140,248,.1); }
/* contain 隔离详情区的布局/绘制范围（避免大内容重排波及整个窗口，含标题栏毛玻璃合成）；
   overflow-anchor 关闭滚动锚定，防内容替换时的锚点重算抖动 */
.detail-body {
  flex: 1; min-height: 0; overflow: auto; padding: 14px;
  contain: layout paint; overflow-anchor: none;
}
.detail-empty { flex: 1; display: flex; align-items: center; justify-content: center; color: var(--text-dim); font-size: 13px; }

/* 可拖动分隔条：视觉 4px，::after 向两侧各扩 3px 命中区（共 10px） */
.splitter {
  width: 4px; flex-shrink: 0; position: relative; cursor: col-resize;
  background: var(--border); transition: background-color .15s ease;
}
.splitter::after {
  content: ''; position: absolute; top: 0; bottom: 0; left: -3px; right: -3px;
}
.splitter:hover, .browser.dragging .splitter { background: var(--accent); }
.splitter.hidden-bar { cursor: default; }
.split-toggle {
  position: absolute; top: 50%; left: 0; transform: translateY(-50%);
  width: 100%; height: 44px; display: flex; align-items: center; justify-content: center;
  background: transparent; border: none; border-radius: 2px; padding: 0;
  color: var(--text); cursor: pointer; z-index: 2;
}
.split-toggle:hover { color: var(--cyan); background: rgba(34,211,238,.15); }

/* 性能关键：禁止逐字符断行。pre-wrap + break-all 会让每个字符成为换行机会点，
   混合中文 + 数千行内 span 时 WebView2 的断行布局是平方级开销，
   曾导致切换视图时整个窗口卡死。改为不换行 + 横向滚动（行布局为线性）。
   注意：pre 自身不设 overflow-x，横向滚动交给 .detail-body（滚动条常驻面板底部，
   否则 pre 的横向滚动条位于长内容最末尾，必须垂直滚到最底才可见）。 */
.raw-pre {
  font-family: var(--mono); font-size: 11.5px; line-height: 1.65;
  white-space: pre; word-break: normal; overflow-wrap: normal;
  tab-size: 2; margin: 0; color: var(--text);
}
/* JSON 语法高亮（v-html 内容需 :deep） */
.raw-pre :deep(.j-key) { color: var(--accent); }
.raw-pre :deep(.j-str) { color: var(--green); }
.raw-pre :deep(.j-num) { color: var(--yellow); }
.raw-pre :deep(.j-kw) { color: var(--red); }

/* SSE 原始视图的语义着色 */
.raw-pre :deep(.sse-evt) { color: var(--text-dim); font-weight: 600; }
.raw-pre :deep(.sse-start) { color: var(--green); }
.raw-pre :deep(.sse-stop) { color: var(--red); }
.raw-pre :deep(.sse-block) { color: var(--yellow); }
.raw-pre :deep(.sse-think) { color: var(--cyan); background: rgba(34,211,238,.08); border-radius: 2px; }
.raw-pre :deep(.sse-reply) { color: var(--green); background: rgba(74,222,128,.08); border-radius: 2px; }
.raw-pre :deep(.sse-tool) { color: var(--yellow); background: rgba(251,191,36,.08); border-radius: 2px; }
.raw-pre :deep(.sse-finish) { color: var(--red); }
.fmt-note { color: var(--text-dim); font-size: 12px; margin-bottom: 10px; line-height: 1.6; }
.fmt-meta { display: flex; gap: 6px; flex-wrap: wrap; margin-bottom: 12px; }
.meta-chip {
  font-family: var(--mono); font-size: 11px; padding: 2px 8px; border-radius: 999px;
  background: var(--bg-elevated); border: 1px solid var(--border); color: var(--text-dim);
}
.fmt-msg { margin-bottom: 12px; }
.fmt-role {
  display: inline-block; font-size: 10.5px; font-weight: 600; letter-spacing: .06em;
  text-transform: uppercase; margin-bottom: 4px; color: var(--accent);
}
.fmt-role.system { color: var(--yellow); }
.fmt-role.assistant { color: var(--green); }
.fmt-role.tool, .fmt-role.function, .fmt-role.tool_use { color: var(--red); }
.fmt-role.thinking { color: var(--text-dim); }
.fmt-text {
  white-space: pre-wrap; word-break: break-word; font-size: 12.5px; line-height: 1.7;
  background: var(--bg); border: 1px solid var(--border); border-radius: var(--radius-sm);
  padding: 10px 12px;
}
.fmt-text.thinking { color: var(--text-dim); font-style: italic; }
.fmt-text.tool_use { font-family: var(--mono); font-size: 11.5px; }
.fmt-usage { display: flex; gap: 8px; margin-top: 6px; }
</style>

<!-- 非 scoped：::highlight() 伪元素无法通过 scoped 属性选择器生效 -->
<style>
::highlight(rec-search-hit) { background: rgba(251,191,36,.35); }
::highlight(rec-search-hit-cur) { background: rgba(251,191,36,.75); outline: 1px solid #FBBF24; }
</style>
