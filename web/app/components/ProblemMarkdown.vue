<script setup lang="ts">
import { renderProblemMarkdown } from '~/utils/problem-markdown'

const props = defineProps<{ source: string }>()
const html = computed(() => renderProblemMarkdown(props.source))
const copyMessage = ref('')
let messageTimer: ReturnType<typeof setTimeout> | undefined
onBeforeUnmount(() => clearTimeout(messageTimer))

async function copyCode(event: MouseEvent) {
  const button = (event.target as Element).closest<HTMLButtonElement>('.code-copy')
  const code = button?.parentElement?.querySelector('pre > code')
  if (!button || !code) return
  clearTimeout(messageTimer)
  copyMessage.value = ''
  try {
    await navigator.clipboard.writeText(code.textContent ?? '')
    copyMessage.value = 'コピーしました。'
  } catch {
    copyMessage.value = 'コピーできませんでした。コードを選択してコピーしてください。'
  }
  messageTimer = setTimeout(() => { copyMessage.value = '' }, 4000)
}
</script>

<template>
  <!-- Raw HTML is disabled by the parser. Only its escaped output and trusted KaTeX markup are inserted. -->
  <div class="markdown-body" @click="copyCode" v-html="html" />
  <span class="code-copy-status" role="status">{{ copyMessage }}</span>
</template>
