<template>
  <div class="pg-root">
    <div class="card shrink">
      <h3><FlaskConical :size="15" :stroke-width="1.75" /> Playground
        <span v-if="!app.status?.proxyRunning" class="badge err" style="margin-left:8px">{{ t('playground.proxyStopped') }}</span>
      </h3>
      <div class="row" style="flex-wrap:wrap;gap:10px">
        <select v-model="model" class="input" style="max-width:340px">
          <option value="" disabled>{{ t('playground.pickModel') }}</option>
          <optgroup v-for="g in grouped" :key="g.provider" :label="g.provider">
            <option v-for="m in g.models" :key="m.id" :value="m.id">{{ m.id }}</option>
          </optgroup>
        </select>
        <label class="switch-row">
          <input type="checkbox" v-model="stream" /> <span>{{ t('playground.stream') }}</span>
        </label>
        <label class="switch-row">max_tokens <input type="number" v-model.number="maxTokens" style="width:90px;padding:5px 8px;background:var(--bg3);border:1px solid var(--border);border-radius:4px;color:var(--text);font-size:12px" /></label>
        <button class="btn btn-danger" v-if="msgs.length" @click="msgs = []" style="margin-left:auto">{{ t('playground.clear') }}</button>
      </div>
      <div class="field" style="margin-top:10px;margin-bottom:0">
        <input type="text" v-model="system" :placeholder="t('playground.systemPh')" style="max-width:100%" />
      </div>
    </div>

    <!-- 对话区（独立滚动） -->
    <div class="card grow">
      <div class="chat-scroll" ref="chatEl">
        <div v-for="(m, i) in msgs" :key="i" class="bubble" :class="m.role">
          <div class="bubble-role">{{ m.role === 'user' ? t('playground.you') : (m.model || t('playground.assistant')) }}</div>
          <div class="bubble-content">{{ m.content }}<span v-if="m.streaming" class="cursor">▌</span></div>
          <div v-if="m.meta" class="bubble-meta">{{ m.meta }}</div>
        </div>
        <div v-if="!msgs.length" class="desc" style="text-align:center;margin-top:40px">
          {{ t('playground.emptyL1') }}<br/>{{ t('playground.emptyL2') }}
        </div>
      </div>
      <div class="input-bar">
        <textarea v-model="input" rows="2" :placeholder="t('playground.inputPh')"
          @keydown="onKey" :disabled="sending"></textarea>
        <button class="btn btn-primary" style="align-self:flex-end" :disabled="sending || !model || !input.trim()" @click="send">
          {{ sending ? t('playground.sending') : t('playground.send') }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { FlaskConical } from 'lucide-vue-next'
import { ListModels, PlaygroundChat } from '../../wailsjs/go/main/App'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import { useAppStore } from '../stores/app'

interface ModelItem { id: string; display_name: string; provider: string; provider_name: string; type: string }
interface Bubble { role: string; content: string; streaming?: boolean; model?: string; meta?: string; failed?: boolean }

const app = useAppStore()
const { t } = useI18n({ useScope: 'global' })
const models = ref<ModelItem[]>([])
const model = ref('')
const stream = ref(true)
const maxTokens = ref(1024)
const system = ref('')
const input = ref('')
const sending = ref(false)
const msgs = ref<Bubble[]>([])
const chatEl = ref<HTMLElement | null>(null)

// ---- 会话保活：切页/重开不丢对话（localStorage，最近 100 条） ----
const SESSION_KEY = 'playground-session-v1'

interface SavedSession { msgs: Bubble[]; system: string; model: string; stream: boolean; maxTokens: number }

function loadSession() {
  try {
    const raw = localStorage.getItem(SESSION_KEY)
    if (!raw) return
    const s = JSON.parse(raw) as SavedSession
    msgs.value = (s.msgs || []).map(m => (m.streaming ? { ...m, streaming: false } : m))
    if (typeof s.system === 'string') system.value = s.system
    if (s.model) model.value = s.model
    if (typeof s.stream === 'boolean') stream.value = s.stream
    if (Number.isFinite(s.maxTokens) && s.maxTokens > 0) maxTokens.value = s.maxTokens
  } catch { /* 损坏的会话数据：忽略，从空开始 */ }
}

let saveTimer: number | undefined
function scheduleSave() {
  if (saveTimer) clearTimeout(saveTimer)
  saveTimer = window.setTimeout(() => {
    try {
      const s: SavedSession = {
        msgs: msgs.value.slice(-100), // 上限防 localStorage 撑爆
        system: system.value, model: model.value, stream: stream.value, maxTokens: maxTokens.value,
      }
      localStorage.setItem(SESSION_KEY, JSON.stringify(s))
    } catch { /* 存储满等异常忽略 */ }
  }, 400)
}

async function scrollBottom() {
  await nextTick()
  if (chatEl.value) chatEl.value.scrollTop = chatEl.value.scrollHeight
}

const grouped = computed(() => {
  const m = new Map<string, ModelItem[]>()
  for (const it of models.value) {
    if (!m.has(it.provider_name)) m.set(it.provider_name, [])
    m.get(it.provider_name)!.push(it)
  }
  return [...m.entries()].map(([provider, items]) => ({ provider, models: items }))
})

function onKey(e: KeyboardEvent) {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    send()
  }
}

async function loadModels() {
  try {
    const res = JSON.parse(await ListModels())
    if (res.success) models.value = res.data
    if (!model.value && models.value.length) model.value = models.value[0].id
  } catch { /* ignore */ }
}

async function send() {
  const text = input.value.trim()
  if (!text || !model.value || sending.value) return
  input.value = ''
  // 多轮上下文：带上此前全部成功回合（排除失败气泡与空内容）
  const history = msgs.value
    .filter(m => !m.failed && m.content && (m.role === 'user' || m.role === 'assistant'))
    .map(m => ({ role: m.role, content: m.content }))
  msgs.value.push({ role: 'user', content: text })
  const bubble: Bubble = { role: 'assistant', content: '', streaming: true, model: model.value }
  msgs.value.push(bubble)
  sending.value = true

  try {
    const req = JSON.stringify({
      model: model.value, stream: stream.value, max_tokens: maxTokens.value,
      system: system.value,
      messages: [...history, { role: 'user', content: text }]
    })
    const promise = PlaygroundChat(req)
    const res = JSON.parse(await promise)
    if (res.success) {
      const d = res.data
      bubble.meta = `${d.model} · ${d.durationMs}ms · ↑${d.promptTokens} ↓${d.completionTokens}${d.stream ? t('playground.streamTag') : ''}`
      if (!stream.value) bubble.content = d.content // 流式时 delta 已实时写入
    } else {
      bubble.content = res.error
      bubble.failed = true
    }
  } catch (e) {
    bubble.content = t('playground.callFailed', { e })
    bubble.failed = true
  } finally {
    bubble.streaming = false
    sending.value = false
  }
}

let cancelDelta: (() => void) | null = null
onMounted(() => {
  loadSession() // 先恢复会话（含上次选的模型），再拉模型列表
  loadModels()
  cancelDelta = EventsOn('playground:delta', (delta: string) => {
    const last = msgs.value[msgs.value.length - 1]
    if (last && last.role === 'assistant') last.content += delta
  })
})
onUnmounted(() => {
  if (cancelDelta) cancelDelta()
  if (saveTimer) clearTimeout(saveTimer)
})
// 会话/设置变化 → 防抖持久化；消息更新（含流式 delta）→ 跟随滚动到底
watch([msgs, system, model, stream, maxTokens], () => { scheduleSave(); scrollBottom() }, { deep: true })
</script>

<style scoped>
.pg-root { display: flex; flex-direction: column; height: calc(100vh - var(--titlebar-h) - 2 * var(--main-pad)); }
.card.shrink { flex-shrink: 0; }
.card.grow { flex: 1; min-height: 200px; display: flex; flex-direction: column; padding-bottom: 10px; }

.chat-scroll { flex: 1; min-height: 0; overflow-y: auto; display: flex; flex-direction: column; gap: 12px; padding: 4px 2px; }
.bubble { max-width: 86%; border-radius: var(--radius-lg); padding: 10px 14px; white-space: pre-wrap; word-break: break-word; font-size: 13.5px; line-height: 1.7; }
.bubble.user { align-self: flex-end; background: var(--accent-solid); color: #fff; }
.bubble.assistant { align-self: flex-start; background: var(--bg); border: 1px solid var(--border); }
.bubble.assistant.failed { border-color: rgba(248,113,113,.5); color: var(--red); }
.bubble-role { font-size: 11px; color: var(--text-dim); margin-bottom: 4px; }
.bubble.user .bubble-role { color: rgba(255,255,255,0.7); }
.bubble-meta { font-size: 11px; color: var(--text-dim); margin-top: 6px; border-top: 1px solid var(--border); padding-top: 5px; font-family: var(--mono); }
.cursor { animation: blink 0.9s infinite; color: var(--accent); }
@keyframes blink { 50% { opacity: 0; } }

.input-bar { display: flex; gap: 10px; margin-top: 10px; }
.input-bar textarea {
  flex: 1; padding: 10px 12px; background: var(--bg3); border: 1px solid var(--border);
  border-radius: 8px; color: var(--text); font-size: 13px; outline: none; resize: none; font-family: inherit;
}
.input-bar textarea:focus { border-color: var(--accent); }
</style>
