<template>
  <div>
    <!-- 厂商健康 -->
    <div class="card" v-if="health.length">
      <h3><Activity :size="15" :stroke-width="1.75" /> {{ t('dashboard.health.title') }}
        <button class="btn" style="margin-left:auto;padding:4px 12px;font-size:12px" :disabled="healthLoading" @click="refreshHealth">
          <RefreshCw :size="12" :class="{ spin: healthLoading }" /> {{ healthLoading ? t('dashboard.health.checking') : t('dashboard.health.checkNow') }}
        </button>
      </h3>
      <div class="row" style="flex-wrap:wrap;gap:10px">
        <div v-for="h in health" :key="h.providerId" class="health-chip" :title="h.lastError || ''">
          <span class="dot" :class="h.state === 'up' ? 'ok' : h.state === 'down' ? 'err' : 'off'"></span>
          <span>{{ h.providerName }}</span>
          <span class="health-latency">{{ h.state === 'up' ? h.latencyMs + 'ms' : (h.state === 'down' ? t('dashboard.health.down') : '…') }}</span>
        </div>
      </div>
    </div>

    <div class="card">
      <h3><Network :size="15" :stroke-width="1.75" /> {{ t('dashboard.proxy.title') }}</h3>
      <i18n-t keypath="dashboard.proxy.desc" tag="div" class="desc">
        <template #bOpenAI><b>{{ t('dashboard.proxy.bOpenAI') }}</b></template>
        <template #cCC><code>/v1/chat/completions</code></template>
        <template #bAnthropic><b>{{ t('dashboard.proxy.bAnthropic') }}</b></template>
        <template #cMsg><code>/v1/messages</code></template>
        <template #bResp><b>{{ t('dashboard.proxy.bResp') }}</b></template>
        <template #cRes><code>/v1/responses</code></template>
        <template #cEnv><code>ANTHROPIC_BASE_URL</code></template>
      </i18n-t>
      <div class="row" style="margin-bottom:6px">
        <span class="badge" :class="st?.proxyRunning ? 'ok' : 'err'">
          {{ st?.proxyRunning ? t('dashboard.proxy.running') : t('dashboard.proxy.stopped') }}
        </span>
        <span v-if="st?.proxyURL" class="badge accent">{{ st.proxyURL }}</span>
      </div>
      <CopyField v-if="st?.proxyURL" :text="st.proxyURL + '/v1'" @copied="app.toast(t('common.copied'))" />
      <div class="row">
        <button class="btn btn-primary" @click="toggleProxy">
          {{ st?.proxyRunning ? t('dashboard.proxy.stop') : t('dashboard.proxy.start') }}
        </button>
      </div>
    </div>

    <div class="card" v-if="app.config?.bind === '0.0.0.0' && wslIp">
      <h3><Terminal :size="15" :stroke-width="1.75" /> {{ t('dashboard.wsl.title') }}</h3>
      <div class="desc">{{ t('dashboard.wsl.desc') }}</div>
      <CopyField :text="`http://${wslIp}:${app.config?.port ?? 8717}/v1`" @copied="app.toast(t('common.copied'))" />
      <i18n-t keypath="dashboard.wsl.tip" tag="div" class="desc" style="margin-top:6px">
        <template #code><code style="color:var(--accent)">ip route show default</code></template>
      </i18n-t>
    </div>

    <div class="card">
      <h3><SquareTerminal :size="15" :stroke-width="1.75" /> {{ t('dashboard.curl.title') }}</h3>
      <div class="desc">{{ t('dashboard.curl.desc') }}</div>
      <pre class="code-block">{{ curlExample }}</pre>
      <CopyField :text="curlExample" @copied="app.toast(t('common.copied'))" />
      <div class="desc" style="margin-top:8px">{{ t('dashboard.curl.python') }}</div>
      <pre class="code-block">{{ pythonExample }}</pre>
    </div>

    <div class="card">
      <h3><Info :size="15" :stroke-width="1.75" /> {{ t('dashboard.info.title') }}</h3>
      <div class="desc">
        <p>{{ t('dashboard.info.l1') }}</p>
        <p>{{ t('dashboard.info.l2') }}</p>
        <p>{{ t('dashboard.info.l3') }}</p>
        <p>{{ t('dashboard.info.l4') }}</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Activity, Network, Terminal, SquareTerminal, Info, RefreshCw } from 'lucide-vue-next'
import { CheckHealthNow, GetHealth, GetHostIPs, SetProxyRunning } from '../../wailsjs/go/main/App'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import { useAppStore } from '../stores/app'
import CopyField from '../components/CopyField.vue'

const app = useAppStore()
const { t } = useI18n({ useScope: 'global' })
const st = computed(() => app.status)
const wslIp = ref('')
const health = ref<{ providerId: string; providerName: string; state: string; latencyMs: number; lastError?: string }[]>([])
const healthLoading = ref(false)

async function loadHealth() {
  try {
    const res = JSON.parse(await GetHealth())
    if (res.success) health.value = res.data
  } catch { /* ignore */ }
}
async function refreshHealth() {
  healthLoading.value = true
  try {
    const res = JSON.parse(await CheckHealthNow())
    if (res.success) health.value = res.data
  } finally { healthLoading.value = false }
}

// 页面级订阅用 EventsOn 返回的 cancel 解绑（EventsOff 会误删其它页面的同事件订阅）
let cancelHealth: (() => void) | null = null
onMounted(async () => {
  loadHealth()
  cancelHealth = EventsOn('health:change', () => loadHealth())
  try {
    const res = JSON.parse(await GetHostIPs())
    if (res.success) {
      const wsl = (res.data as { name: string; ip: string; wsl: boolean }[]).find(n => n.wsl)
      if (wsl) wslIp.value = wsl.ip
    }
  } catch { /* ignore */ }
})
onUnmounted(() => { if (cancelHealth) cancelHealth() })

const base = computed(() => (st.value?.proxyURL || 'http://127.0.0.1:8717') + '/v1')
const apiKey = computed(() => app.config?.localApiKey || '')

const curlExample = computed(() => {
  const auth = apiKey.value ? ` \\\n  -H "x-api-key: ${apiKey.value}"` : ''
  return `curl -N ${base.value}/chat/completions${auth} \\
  -H "Content-Type: application/json" \\
  -d '{"model":"MODEL-ID","messages":[{"role":"user","content":"${t('dashboard.curl.sampleMsg')}"}],"stream":true}'`
})

const pythonExample = computed(() => {
  const keyLine = apiKey.value
    ? `client = OpenAI(base_url="${base.value}", api_key="${apiKey.value}")`
    : `client = OpenAI(base_url="${base.value}", api_key="none")  # ${t('dashboard.curl.noKeyNote')}`
  return `from openai import OpenAI

${keyLine}

resp = client.chat.completions.create(
    model="MODEL-ID",
    messages=[{"role": "user", "content": "${t('dashboard.curl.sampleMsg')}"}],
)
print(resp.choices[0].message.content)`
})

async function toggleProxy() {
  const on = !st.value?.proxyRunning
  const res = JSON.parse(await SetProxyRunning(on))
  if (res.success) {
    app.toast(on ? t('dashboard.proxy.startedToast') : t('dashboard.proxy.stoppedToast'))
  } else {
    app.toast(res.error, 'err')
  }
}
</script>

<style scoped>
.health-chip {
  display: inline-flex; align-items: center; gap: 8px; padding: 6px 14px;
  background: var(--bg); border: 1px solid var(--border); border-radius: 999px; font-size: 12.5px;
  transition: border-color .15s ease;
}
.health-chip:hover { border-color: var(--border-bright); }
.health-latency { color: var(--text-dim); font-size: 11.5px; font-family: var(--mono); }
.spin { animation: rot 1s linear infinite; }
@keyframes rot { to { transform: rotate(360deg); } }
</style>
