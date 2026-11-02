<template>
  <div>
    <!-- 列表 -->
    <div class="card">
      <h3><Server :size="15" :stroke-width="1.75" /> {{ t('providers.title') }}
        <button class="btn btn-primary" style="margin-left:auto" @click="openForm(null)"><Plus :size="13" /> {{ t('providers.add') }}</button>
      </h3>
      <div class="desc">
        <i18n-t keypath="providers.desc" tag="span">
          <template #b><b>{{ t('providers.descBold') }}</b></template>
          <template #code><code>DeepSeek/deepseek-chat</code></template>
        </i18n-t>
      </div>
      <div v-if="conflicts.length" class="warn-banner">
        <div v-for="c in conflicts" :key="c"><AlertTriangle :size="13" style="vertical-align:-2px" /> {{ c }}</div>
      </div>
      <table>
        <thead>
          <tr>
            <th>{{ t('providers.th.name') }}</th>
            <th style="width:90px">{{ t('providers.th.type') }}</th>
            <th>Base URL</th>
            <th style="width:80px">{{ t('providers.th.status') }}</th>
            <th>{{ t('providers.th.models') }}</th>
            <th style="width:70px">{{ t('providers.th.default') }}</th>
            <th style="width:230px">{{ t('providers.th.actions') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="p in providers" :key="p.id">
            <td>{{ p.name }}</td>
            <td>
              <span class="badge">{{ typeLabel(p.type) }}</span>
            </td>
            <td style="font-family:var(--mono);font-size:12px;max-width:280px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">
              {{ p.baseUrl }}
            </td>
            <td>
              <label class="switch-row">
                <input type="checkbox" :checked="p.enabled" @change="toggleEnabled(p, ($event.target as HTMLInputElement).checked)" />
              </label>
            </td>
            <td style="font-size:12px;max-width:260px">
              <template v-if="p.models?.length">
                <span v-for="m in p.models.slice(0, 4)" :key="m" class="badge model-chip">{{ m }}</span>
                <span v-if="p.models.length > 4" class="badge" :title="p.models.join(', ')">+{{ p.models.length - 4 }}</span>
              </template>
              <template v-else>-</template>
            </td>
            <td>
              <input type="radio" name="defaultProv" :checked="defaultId === p.id" @change="setDefault(p.id)" :title="t('providers.setDefaultTip')" />
            </td>
            <td>
              <button class="btn" style="padding:3px 10px;font-size:12px" @click="openForm(p)">{{ t('common.edit') }}</button>
              <button class="btn" style="padding:3px 10px;font-size:12px" :disabled="testing" @click="test(p)">{{ t('providers.test') }}</button>
              <button class="btn btn-danger" style="padding:3px 10px;font-size:12px" @click="remove(p)">{{ t('common.delete') }}</button>
            </td>
          </tr>
        </tbody>
      </table>
      <div v-if="!providers.length" class="desc">{{ t('providers.empty') }}</div>
    </div>

    <!-- 表单 -->
    <div class="card" v-if="formOpen">
      <h3>{{ form.id ? t('providers.form.edit') : t('providers.form.add') }}</h3>
      <div class="field">
        <label>{{ t('providers.form.name') }}</label>
        <input type="text" v-model="form.name" :placeholder="t('providers.form.namePh')" style="max-width:320px" />
      </div>
      <div class="field">
        <label>{{ t('providers.form.type') }}</label>
        <select v-model="form.type" class="input" style="max-width:420px">
          <option value="openai">{{ t('providers.form.typeOpenai') }}</option>
          <option value="anthropic">{{ t('providers.form.typeAnthropic') }}</option>
          <option value="gemini">{{ t('providers.form.typeGemini') }}</option>
        </select>
      </div>
      <div class="field">
        <label>{{ t('providers.form.baseUrl') }}</label>
        <input type="text" v-model="form.baseUrl" placeholder="https://api.deepseek.com" style="max-width:420px" />
        <div class="desc" v-if="form.type === 'anthropic'">{{ t('providers.form.anthropicHint') }}</div>
        <div class="desc" v-if="form.type === 'gemini'">{{ t('providers.form.geminiHint') }}</div>
      </div>
      <div class="field">
        <label>{{ t('providers.form.apiKey') }}</label>
        <input type="text" v-model="form.apiKey" placeholder="sk-..." style="max-width:420px" />
      </div>
      <div class="field">
        <label>{{ t('providers.form.models') }}</label>
        <div class="row">
          <textarea v-model="form.modelsText" rows="3" placeholder="deepseek-chat&#10;deepseek-reasoner" style="max-width:520px;flex:1;font-family:var(--mono);resize:vertical"></textarea>
          <div style="display:flex;flex-direction:column;gap:8px">
            <button class="btn" :disabled="fetching" @click="fetchModels">{{ fetching ? t('providers.form.fetching') : t('providers.form.fetch') }}</button>
          </div>
        </div>
        <div v-if="fetchedModels.length" class="desc" style="margin-top:6px">
          <a v-for="m in fetchedModels" :key="m.id" class="badge pick" @click="pickModel(m.id)">{{ m.id }}</a>
        </div>
      </div>
      <div class="field">
        <label>{{ t('providers.form.fallbacks') }}</label>
        <textarea v-model="form.fallbacksText" rows="2" placeholder="auto&#10;DeepSeek/deepseek-chat" style="max-width:520px;font-family:var(--mono);resize:vertical"></textarea>
      </div>
      <div class="field" v-if="form.type === 'anthropic'">
        <label>{{ t('providers.form.maxTokens') }}</label>
        <input type="number" v-model.number="form.maxTokens" style="max-width:160px" />
      </div>
      <div class="row" style="margin-top:16px">
        <button class="btn btn-primary" :disabled="saving" @click="save">{{ saving ? t('providers.form.saving') : t('common.save') }}</button>
        <button class="btn" @click="formOpen = false">{{ t('common.cancel') }}</button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Server, Plus, AlertTriangle } from 'lucide-vue-next'
import {
  DeleteProvider, FetchProviderModels, GetConfig, ListProviders, SaveProvider, SetDefaultProvider, TestProvider
} from '../../wailsjs/go/main/App'
import { useAppStore } from '../stores/app'

interface Prov {
  id: string; name: string; type: string; baseUrl: string; apiKey: string
  enabled: boolean; models: string[] | null; note: string; maxTokens: number
  modelMapping: Record<string, string> | null
  fallbacks?: string[] | null
}

const app = useAppStore()
const { t } = useI18n({ useScope: 'global' })
const providers = ref<Prov[]>([])
const defaultId = ref('')
const conflicts = ref<string[]>([])
const formOpen = ref(false)
const saving = ref(false)
const testing = ref(false)
const fetching = ref(false)
const fetchedModels = ref<{ id: string }[]>([])

const form = reactive({
  id: '', name: '', type: 'openai', baseUrl: '', apiKey: '',
  modelsText: '', fallbacksText: '', maxTokens: 8192
})

// 多分隔符解析：换行 / 逗号（中英文）/ 分号（中英文）/ 空白
function parseModels(text: string): string[] {
  return text.split(/[\n,;，；\t ]+/).map(s => s.trim()).filter(Boolean)
}

function typeLabel(ty: string): string {
  if (ty === 'anthropic') return 'Anthropic'
  if (ty === 'gemini') return 'Gemini'
  return 'OpenAI'
}

function handleSaveError(res: { error?: string }) {
  app.toast(res.error || '保存失败', 'err')
}

async function load() {
  const res = JSON.parse(await ListProviders())
  if (res.success) {
    providers.value = res.data.providers
    defaultId.value = res.data.defaultProviderId
    conflicts.value = res.data.conflicts || []
  }
}

// 厂商增删改后同步 store 里的配置快照，避免设置/录制页用旧快照保存时回退厂商列表
async function refreshAppConfig() {
  try {
    const res = JSON.parse(await GetConfig())
    if (res.success) app.config = res.data
  } catch { /* ignore */ }
}

function openForm(p: Prov | null) {
  fetchedModels.value = []
  if (p) {
    form.id = p.id; form.name = p.name; form.type = p.type
    form.baseUrl = p.baseUrl; form.apiKey = p.apiKey
    form.modelsText = (p.models || []).join('\n')
    form.fallbacksText = (p.fallbacks || []).join('\n')
    form.maxTokens = p.maxTokens || 8192
  } else {
    form.id = ''; form.name = ''; form.type = 'openai'
    form.baseUrl = ''; form.apiKey = ''; form.modelsText = ''; form.fallbacksText = ''; form.maxTokens = 8192
  }
  formOpen.value = true
}

async function save() {
  saving.value = true
  try {
    const cfg = {
      id: form.id, name: form.name, type: form.type, baseUrl: form.baseUrl,
      apiKey: form.apiKey, enabled: true,
      models: parseModels(form.modelsText),
      fallbacks: parseModels(form.fallbacksText),
      maxTokens: form.maxTokens
    }
    const res = JSON.parse(await SaveProvider(JSON.stringify(cfg)))
    if (res.success) {
      app.toast(t('providers.toast.saved', { name: cfg.name, n: cfg.models.length }))
      const warns = res.data.warnings || []
      if (warns.length) {
        app.toast(warns[0], 'err')
      }
      formOpen.value = false
      load()
      refreshAppConfig()
    } else {
      handleSaveError(res)
    }
  } finally {
    saving.value = false
  }
}

async function remove(p: Prov) {
  if (!(await app.confirm({ title: t('providers.confirmTitle'), body: t('providers.confirmBody', { name: p.name }), danger: true }))) return
  const res = JSON.parse(await DeleteProvider(p.id))
  if (res.success) { app.toast(t('providers.toast.deleted')); load(); refreshAppConfig() } else app.toast(res.error, 'err')
}

async function toggleEnabled(p: Prov, on: boolean) {
  const res = JSON.parse(await SaveProvider(JSON.stringify({ ...p, enabled: on })))
  if (res.success) { app.toast(on ? t('providers.toast.enabled') : t('providers.toast.disabled')); load(); refreshAppConfig() } else handleSaveError(res)
}

async function setDefault(id: string) {
  const res = JSON.parse(await SetDefaultProvider(id))
  if (res.success) { defaultId.value = id; app.toast(t('providers.toast.defaultSwitched')) } else app.toast(res.error, 'err')
}

async function test(p: Prov) {
  testing.value = true
  try {
    const res = JSON.parse(await TestProvider(JSON.stringify(p)))
    if (res.success && res.data.ok) {
      app.toast(t('providers.toast.testOk', { status: res.data.httpStatus }))
    } else {
      app.toast(t('providers.toast.testFail', { status: res.data?.httpStatus ?? '', msg: res.data?.sample || res.error || '' }), 'err')
    }
  } finally {
    testing.value = false
  }
}

async function fetchModels() {
  fetching.value = true
  try {
    const res = JSON.parse(await FetchProviderModels(JSON.stringify({
      type: form.type, baseUrl: form.baseUrl, apiKey: form.apiKey
    })))
    if (res.success) {
      fetchedModels.value = res.data
      app.toast(t('providers.toast.fetched', { n: res.data.length }))
    } else {
      app.toast(res.error, 'err')
    }
  } finally {
    fetching.value = false
  }
}

function pickModel(id: string) {
  const cur = parseModels(form.modelsText)
  if (!cur.includes(id)) cur.push(id)
  form.modelsText = cur.join('\n')
}

onMounted(load)
</script>

<style scoped>
.badge.pick { cursor: pointer; margin: 2px; }
.badge.pick:hover { border-color: var(--accent); color: var(--accent); }
.model-chip { margin: 1px 2px; display: inline-block; }
.warn-banner {
  margin: 10px 0; padding: 10px 14px; border: 1px solid rgba(251,191,36,.4);
  border-radius: var(--radius); background: rgba(251,191,36,.07); color: var(--yellow);
  font-size: 12.5px; line-height: 1.9;
}
</style>
