<template>
  <div>
    <div class="card">
      <h3><Settings2 :size="15" :stroke-width="1.75" /> {{ t('settings.title') }}</h3>
      <div class="field">
        <label>{{ t('settings.language.label') }}</label>
        <div class="row">
          <select v-model="languageSel" class="input" style="max-width:420px">
            <option value="">{{ t('settings.language.auto') }}</option>
            <option value="zh">{{ t('settings.language.zh') }}</option>
            <option value="en">{{ t('settings.language.en') }}</option>
          </select>
        </div>
        <div class="desc">{{ t('settings.language.hint') }}</div>
      </div>
      <div class="field">
        <label>{{ t('settings.autoStart') }}</label>
        <label class="switch-row">
          <input type="checkbox" :checked="autoStart" @change="toggleAutoStart" />
          <span>{{ autoStart ? t('settings.autoStartOn') : t('settings.autoStartOff') }}</span>
        </label>
      </div>
      <div class="field">
        <label>{{ t('settings.autoProxy') }}</label>
        <label class="switch-row">
          <input type="checkbox" v-model="form.proxyEnabled" />
          <span>{{ form.proxyEnabled ? t('common.on') : t('common.off') }}</span>
        </label>
      </div>
      <div class="field">
        <label>{{ t('settings.bind') }}</label>
        <div class="row">
          <select v-model="form.bind" class="input" style="max-width:420px">
            <option value="127.0.0.1">{{ t('settings.bindLoopback') }}</option>
            <option value="0.0.0.0">{{ t('settings.bindAll') }}</option>
          </select>
        </div>
        <div v-if="form.bind === '0.0.0.0' && !form.localApiKey" class="warn-tip">
          {{ t('settings.bindWarn') }}
        </div>
      </div>
      <div class="field">
        <label>{{ t('settings.port') }}</label>
        <input type="number" v-model.number="form.port" min="1024" max="65535" style="max-width:160px" />
      </div>
      <div class="field">
        <label>{{ t('settings.apiKey') }}</label>
        <div class="row">
          <input type="text" v-model="form.localApiKey" :placeholder="t('settings.apiKeyPh')" />
          <button class="btn" @click="genKey"><KeyRound :size="13" /> {{ t('settings.genKey') }}</button>
        </div>
      </div>
      <div class="row" style="margin-top:16px">
        <button class="btn btn-primary" :disabled="saving" @click="save">
          {{ saving ? t('settings.saving') : t('settings.saveApply') }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Settings2, KeyRound } from 'lucide-vue-next'
import { GetAutoStart, SaveConfig, SetAutoStart } from '../../wailsjs/go/main/App'
import { useAppStore, AppConfig } from '../stores/app'
import { setLocale, resolveAutoLang } from '../i18n'

const app = useAppStore()
const { t } = useI18n({ useScope: 'global' })
const saving = ref(false)

const form = reactive<AppConfig>({
  proxyEnabled: true,
  port: 8717,
  bind: '127.0.0.1',
  localApiKey: '',
  language: ''
})

watch(
  () => app.config,
  (c) => {
    if (c) Object.assign(form, c)
  },
  { immediate: true }
)

// 语言下拉：'' = 跟随系统 | zh | en。切换即改前端 locale 并单独保存（不带半编辑表单）。
const languageSel = computed({
  get: () => form.language ?? '',
  set: (v: string) => {
    form.language = v
    setLocale(v === '' ? resolveAutoLang() : (v as 'zh' | 'en'))
    void saveLanguageOnly(v)
  },
})

async function saveLanguageOnly(v: string) {
  // 快照基底必须是 app.config（含 providers 之外的最新持久字段），绝不混入半编辑的 form
  const res = JSON.parse(await SaveConfig(JSON.stringify({ ...app.config, language: v })))
  if (res.success) {
    app.config = res.data
  } else {
    app.toast(res.error, 'err')
  }
}

const autoStart = ref(false)
onMounted(async () => {
  try {
    const res = JSON.parse(await GetAutoStart())
    if (res.success) autoStart.value = res.data.enabled
  } catch { /* ignore */ }
})

async function toggleAutoStart(e: Event) {
  const on = (e.target as HTMLInputElement).checked
  const res = JSON.parse(await SetAutoStart(on))
  if (res.success) {
    autoStart.value = on
    app.toast(on ? t('settings.autoStartOn') : t('settings.autoStartOff'))
  } else {
    app.toast(res.error, 'err')
    ;(e.target as HTMLInputElement).checked = !on
  }
}

function genKey() {
  const buf = new Uint8Array(16)
  crypto.getRandomValues(buf)
  form.localApiKey =
    'sk-local-' + Array.from(buf, b => b.toString(16).padStart(2, '0')).join('')
}

async function save() {
  saving.value = true
  try {
    const res = JSON.parse(await SaveConfig(JSON.stringify(form)))
    if (res.success) {
      app.config = res.data
      app.toast(t('settings.saved'))
    } else {
      app.toast(res.error, 'err')
    }
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
.warn-tip {
  margin-top: 8px; padding: 8px 12px; border: 1px solid rgba(251,191,36,.4);
  border-radius: var(--radius); background: rgba(251,191,36,.07); color: var(--yellow);
  font-size: 12.5px; line-height: 1.6; max-width: 560px;
}
</style>
