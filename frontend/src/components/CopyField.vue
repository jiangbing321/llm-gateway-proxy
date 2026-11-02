<template>
  <div class="copybox">
    <code>{{ text }}</code>
    <button class="copy-btn" :title="t('common.copy')" @click="copy"><Copy :size="14" /></button>
  </div>
</template>

<script setup lang="ts">
import { Copy } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import { CopyText } from '../../wailsjs/go/main/App'

const { t } = useI18n({ useScope: 'global' })

const props = defineProps<{ text: string }>()
const emit = defineEmits<{ (e: 'copied'): void }>()

async function copy() {
  try {
    await CopyText(props.text)
    emit('copied')
  } catch {
    /* ignore */
  }
}
</script>
