<template>
  <div>
    <div class="card">
      <h3><Boxes :size="15" :stroke-width="1.75" /> {{ t('models.title') }}</h3>
      <div class="desc">
        {{ t('models.desc') }}
      </div>
      <div class="row" style="margin-bottom:12px">
        <button class="btn btn-primary" :disabled="loading" @click="load">
          <RefreshCw :size="13" :class="{ spin: loading }" /> {{ loading ? t('models.loading') : t('models.refresh') }}
        </button>
        <span v-if="models.length" class="badge">{{ t('models.count', { n: models.length }) }}</span>
      </div>
      <table v-if="models.length">
        <thead>
          <tr>
            <th style="width:90px">{{ t('models.th.action') }}</th>
            <th>ID</th>
            <th>{{ t('models.th.name') }}</th>
            <th>{{ t('models.th.provider') }}</th>
            <th style="width:90px">{{ t('models.th.type') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="m in models" :key="m.provider + m.id">
            <td><button class="btn" style="padding:3px 10px;font-size:12px" @click="copyId(m.id)"><Copy :size="12" /> {{ t('common.copy') }}</button></td>
            <td><code style="color:var(--accent)">{{ m.id }}</code></td>
            <td>{{ m.display_name || m.id }}</td>
            <td><span class="badge">{{ m.provider_name }}</span></td>
            <td>
              <span class="badge" :class="m.type === 'vision' ? 'accent' : ''">{{ m.type || 'general' }}</span>
            </td>
          </tr>
        </tbody>
      </table>
      <div v-else class="desc">{{ t('models.empty') }}</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Boxes, Copy, RefreshCw } from 'lucide-vue-next'
import { CopyText, ListModels } from '../../wailsjs/go/main/App'
import { useAppStore } from '../stores/app'

interface GWModel {
  id: string
  display_name: string
  provider: string
  provider_name: string
  type: string
}

const app = useAppStore()
const { t } = useI18n({ useScope: 'global' })
const loading = ref(false)
const models = ref<GWModel[]>([])

async function load() {
  loading.value = true
  try {
    const res = JSON.parse(await ListModels())
    if (res.success) {
      models.value = res.data
      app.toast(t('models.toast.loaded', { n: res.data.length }))
    } else {
      app.toast(res.error, 'err')
    }
  } finally {
    loading.value = false
  }
}

async function copyId(id: string) {
  await CopyText(id)
  app.toast(t('models.toast.copied', { id }))
}

onMounted(() => {
  load()
})
</script>

<style scoped>
.spin { animation: rot 1s linear infinite; }
@keyframes rot { to { transform: rotate(360deg); } }
</style>
