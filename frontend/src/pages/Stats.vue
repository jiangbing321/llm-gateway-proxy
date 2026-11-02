<template>
  <div>
    <!-- 汇总卡片 -->
    <div class="card">
      <h3><BarChart3 :size="15" :stroke-width="1.75" /> {{ t('stats.title') }}
        <span style="margin-left:auto" class="row">
          <button class="btn" :class="{ 'btn-primary': range === 1 }" @click="setRange(1)">{{ t('stats.range1') }}</button>
          <button class="btn" :class="{ 'btn-primary': range === 7 }" @click="setRange(7)">{{ t('stats.range7') }}</button>
          <button class="btn" :class="{ 'btn-primary': range === 30 }" @click="setRange(30)">{{ t('stats.range30') }}</button>
        </span>
      </h3>
      <div class="cards-row">
        <div class="stat-box"><div class="num">{{ s.total.requests }}</div><div class="lbl">{{ t('stats.stat.requests') }}</div></div>
        <div class="stat-box"><div class="num">{{ fmtTok(s.total.promptTokens + s.total.completionTokens) }}</div><div class="lbl">{{ t('stats.stat.tokens') }}</div></div>
        <div class="stat-box"><div class="num">{{ fmtTok(s.total.promptTokens) }} / {{ fmtTok(s.total.completionTokens) }}</div><div class="lbl">{{ t('stats.stat.inOut') }}</div></div>
        <div class="stat-box"><div class="num">¥{{ s.total.cost.toFixed(3) }}</div><div class="lbl">{{ t('stats.stat.cost') }}</div></div>
        <div class="stat-box"><div class="num">{{ s.total.requests ? ((s.total.errors / s.total.requests) * 100).toFixed(1) : '0' }}%</div><div class="lbl">{{ t('stats.stat.errRate') }}</div></div>
      </div>
    </div>

    <!-- 按天用量（图表 / 表格双视图） -->
    <div class="card">
      <h3>{{ t('stats.daily.title') }}
        <span style="margin-left:auto" class="row">
          <button class="btn" :class="{ 'btn-primary': view === 'chart' }" @click="view = 'chart'">{{ t('stats.daily.viewChart') }}</button>
          <button class="btn" :class="{ 'btn-primary': view === 'table' }" @click="view = 'table'">{{ t('stats.daily.viewTable') }}</button>
        </span>
      </h3>
      <!-- 图表视图（纯 CSS，双色堆叠） -->
      <div v-if="s.days.length && view === 'chart'" class="chart">
        <div v-for="d in s.days" :key="d.date" class="chart-col" :class="{ today: isToday(d.date) }"
             :title="t('stats.chartTip', { date: d.date, today: isToday(d.date) ? t('stats.todayTag') : '', req: d.requests, in: d.promptTokens, out: d.completionTokens, cost: d.cost.toFixed(3) })">
          <div class="bars">
            <div class="bar in" :style="{ height: barH(d.promptTokens) }"></div>
            <div class="bar out" :style="{ height: barH(d.completionTokens) }"></div>
          </div>
          <div class="chart-date">{{ d.date.slice(5) }}</div>
        </div>
      </div>
      <!-- 表格视图（最新在前，今日行高亮） -->
      <table v-if="s.days.length && view === 'table'">
        <thead>
          <tr>
            <th>{{ t('stats.th.date') }}</th>
            <th style="width:90px">{{ t('stats.th.requests') }}</th>
            <th style="width:90px">{{ t('stats.th.errors') }}</th>
            <th style="width:120px">{{ t('stats.th.input') }}</th>
            <th style="width:120px">{{ t('stats.th.output') }}</th>
            <th style="width:110px">{{ t('stats.th.cost') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="d in reversedDays" :key="d.date" :class="{ 'day-today': isToday(d.date) }">
            <td style="font-family:var(--mono);font-size:12px">{{ d.date }}{{ isToday(d.date) ? t('stats.todayTag') : '' }}</td>
            <td>{{ d.requests }}</td>
            <td :style="d.errors ? 'color:var(--red)' : ''">{{ d.errors }}</td>
            <td>{{ fmtTok(d.promptTokens) }}</td>
            <td>{{ fmtTok(d.completionTokens) }}</td>
            <td>¥{{ d.cost.toFixed(3) }}</td>
          </tr>
        </tbody>
      </table>
      <div v-if="!s.days.length" class="desc">{{ t('stats.emptyDays') }}</div>
      <div v-if="view === 'chart'" class="legend">
        <span><i class="dot in"></i>{{ t('stats.legendIn') }}</span>
        <span><i class="dot out"></i>{{ t('stats.legendOut') }}</span>
      </div>
    </div>

    <!-- 厂商×模型明细 -->
    <div class="card">
      <h3>{{ t('stats.detailTitle') }}</h3>
      <table v-if="s.rows.length">
        <thead>
          <tr>
            <th>{{ t('stats.th.provider') }}</th><th>{{ t('stats.th.model') }}</th>
            <th style="width:80px">{{ t('stats.th.requests') }}</th>
            <th style="width:80px">{{ t('stats.th.errors') }}</th>
            <th style="width:110px">{{ t('stats.th.input') }}</th>
            <th style="width:110px">{{ t('stats.th.output') }}</th>
            <th style="width:100px">{{ t('stats.th.cost') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="r in s.rows" :key="r.provider + r.model">
            <td><span class="badge">{{ r.provider }}</span></td>
            <td><code style="color:var(--accent);font-size:12px">{{ r.model }}</code></td>
            <td>{{ r.requests }}</td>
            <td :style="r.errors ? 'color:var(--red)' : ''">{{ r.errors }}</td>
            <td>{{ fmtTok(r.promptTokens) }}</td>
            <td>{{ fmtTok(r.completionTokens) }}</td>
            <td>¥{{ r.cost.toFixed(3) }}</td>
          </tr>
        </tbody>
      </table>
      <div v-else class="desc">{{ t('stats.emptyRows') }}</div>
    </div>

    <!-- 计价规则 -->
    <div class="card">
      <h3><Coins :size="15" :stroke-width="1.75" /> {{ t('stats.pricing.title') }}
        <button class="btn" style="margin-left:auto" @click="addRule"><Plus :size="13" /> {{ t('stats.pricing.add') }}</button>
        <button class="btn btn-primary" style="margin-left:8px" :disabled="savingPricing" @click="savePricing">{{ t('common.save') }}</button>
      </h3>
      <div class="desc">{{ t('stats.pricing.desc') }}</div>
      <table v-if="pricing.length">
        <thead>
          <tr>
            <th>{{ t('stats.pricing.thProvider') }}</th><th>{{ t('stats.pricing.thModel') }}</th>
            <th style="width:110px">{{ t('stats.pricing.thInput') }}</th>
            <th style="width:110px">{{ t('stats.pricing.thOutput') }}</th>
            <th>{{ t('stats.pricing.thNote') }}</th>
            <th style="width:60px"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(p, i) in pricing" :key="i">
            <td><input class="input" style="width:100%" v-model="p.providerPattern" placeholder="*" /></td>
            <td><input class="input" style="width:100%" v-model="p.modelPattern" placeholder="*" /></td>
            <td><input class="input" type="number" step="0.1" style="width:100%" v-model.number="p.inputPer1M" /></td>
            <td><input class="input" type="number" step="0.1" style="width:100%" v-model.number="p.outputPer1M" /></td>
            <td><input class="input" style="width:100%" v-model="p.note" /></td>
            <td><button class="btn btn-danger" style="padding:2px 8px;font-size:12px" @click="pricing.splice(i, 1)"><Trash2 :size="12" /></button></td>
          </tr>
        </tbody>
      </table>
      <div v-else class="desc">{{ t('stats.pricing.empty') }}</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { BarChart3, Coins, Plus, Trash2 } from 'lucide-vue-next'
import { GetPricing, GetStats, SavePricing } from '../../wailsjs/go/main/App'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import { useAppStore } from '../stores/app'

interface Daily { date: string; requests: number; errors: number; promptTokens: number; completionTokens: number; cost: number }
interface Row { provider: string; model: string; requests: number; errors: number; promptTokens: number; completionTokens: number; cost: number }
interface Pricing { providerPattern: string; modelPattern: string; inputPer1M: number; outputPer1M: number; note: string }

const app = useAppStore()
const { t } = useI18n({ useScope: 'global' })
const range = ref(7)
const view = ref<'chart' | 'table'>('chart')
const days = ref<Daily[]>([])
const rows = ref<Row[]>([])
const total = ref<Daily>({ date: '', requests: 0, errors: 0, promptTokens: 0, completionTokens: 0, cost: 0 })
const pricing = ref<Pricing[]>([])
const savingPricing = ref(false)

const s = computed(() => ({ days: days.value, rows: rows.value, total: total.value }))
const maxTok = computed(() => Math.max(1, ...days.value.map(d => Math.max(d.promptTokens, d.completionTokens))))
// 表格视图最新在前
const reversedDays = computed(() => [...days.value].reverse())

function barH(v: number): string {
  return Math.max(2, Math.round((v / maxTok.value) * 130)) + 'px'
}
function fmtTok(v: number): string {
  if (v >= 1e6) return (v / 1e6).toFixed(1) + 'M'
  if (v >= 1e3) return (v / 1e3).toFixed(1) + 'K'
  return String(v)
}

async function load() {
  const res = JSON.parse(await GetStats(range.value))
  if (res.success) {
    days.value = res.data.days || []
    rows.value = res.data.rows || []
    total.value = res.data.total
  }
  const pr = JSON.parse(await GetPricing())
  if (pr.success) pricing.value = pr.data || []
}

function isToday(date: string): boolean {
  const now = new Date()
  const mm = String(now.getMonth() + 1).padStart(2, '0')
  const dd = String(now.getDate()).padStart(2, '0')
  return date === `${now.getFullYear()}-${mm}-${dd}`
}

function setRange(n: number) { range.value = n; load() }
function addRule() {
  pricing.value.push({ providerPattern: '*', modelPattern: '*', inputPer1M: 0, outputPer1M: 0, note: '' })
}

async function savePricing() {
  savingPricing.value = true
  try {
    const res = JSON.parse(await SavePricing(JSON.stringify(pricing.value)))
    if (res.success) {
      app.toast(t('stats.pricing.saved'))
      load() // 重新计算成本
    } else app.toast(res.error, 'err')
  } finally {
    savingPricing.value = false
  }
}

// 自动刷新：新请求到达（log:new）即刷新统计；另加 30s 兜底定时器。
// 注意：wails 的 EventsOff(事件名) 会移除该事件【全部】监听器（包括 App.vue
// 里请求日志页依赖的全局订阅）——页面级解绑必须用 EventsOn 返回的 cancel 函数。
let cancelLog: (() => void) | null = null
let timer: ReturnType<typeof setInterval> | null = null
onMounted(() => {
  load()
  cancelLog = EventsOn('log:new', () => load())
  timer = setInterval(load, 30000)
})
onUnmounted(() => {
  if (cancelLog) cancelLog()
  if (timer) clearInterval(timer)
})
</script>

<style scoped>
.cards-row { display: flex; gap: 12px; flex-wrap: wrap; }
.stat-box {
  flex: 1; min-width: 130px; background: var(--bg); border: 1px solid var(--border);
  border-radius: var(--radius); padding: 12px 14px; text-align: center;
}
.stat-box .num {
  font-size: 18px; font-weight: 600; font-family: var(--mono);
  background: var(--grad-accent); -webkit-background-clip: text; background-clip: text;
  color: transparent;
}
.stat-box .lbl { font-size: 11.5px; color: var(--text-dim); margin-top: 4px; }

.chart { display: flex; align-items: flex-end; gap: 6px; height: 180px; padding: 8px 4px; overflow-x: auto; }
.chart-col { display: flex; flex-direction: column; align-items: center; min-width: 34px; }
.bars { display: flex; align-items: flex-end; gap: 3px; height: 140px; }
.bar { width: 12px; border-radius: 2px 2px 0 0; min-height: 2px; }
.bar.in { background: var(--grad-accent); }
.bar.out { background: linear-gradient(180deg, #4ADE80, #22C55E); }
.chart-date { font-size: 10.5px; color: var(--text-dim); margin-top: 6px; font-family: var(--mono); }
/* 表格视图今日行强调 */
tr.day-today td:first-child { color: var(--accent); font-weight: 600; }
/* 今日列高亮 */
.chart-col.today .chart-date { color: var(--accent); font-weight: 600; }
.chart-col.today .bar.in  { box-shadow: 0 0 0 1px rgba(124,140,248,.8), 0 0 12px rgba(99,102,241,.5); }
.chart-col.today .bar.out { box-shadow: 0 0 0 1px rgba(124,140,248,.8), 0 0 12px rgba(99,102,241,.5); }
.legend { display: flex; gap: 18px; margin-top: 8px; font-size: 12px; color: var(--text-dim); }
.legend .dot { display: inline-block; width: 10px; height: 10px; border-radius: 2px; margin-right: 5px; }
.legend .dot.in { background: var(--grad-accent); }
.legend .dot.out { background: var(--green); }
</style>
