<script setup lang="ts">
import { renderProblemMarkdown } from '~/utils/problem-markdown'

const props = defineProps<{ source: string, copyable?: boolean }>()
const html = computed(() => renderProblemMarkdown(props.source))
const copyMessage = ref('')
let messageTimer: ReturnType<typeof setTimeout> | undefined
onBeforeUnmount(() => clearTimeout(messageTimer))

async function copyCode(event: MouseEvent) {
  const button = (event.target as Element).closest<HTMLButtonElement>('.code-copy')
  const code = button?.parentElement?.querySelector('pre > code')
  if (!button || !code) return
  await copyText(code.textContent ?? '', 'コードを選択してコピーしてください。')
}

async function copyText(text: string, advice = 'もう一度お試しください。') {
  clearTimeout(messageTimer)
  copyMessage.value = ''
  try {
    await navigator.clipboard.writeText(text)
    copyMessage.value = 'コピーしました。'
  } catch {
    copyMessage.value = `コピーできませんでした。${advice}`
  }
  messageTimer = setTimeout(() => { copyMessage.value = '' }, 4000)
}
</script>

<template>
  <div :class="{ 'copyable-markdown': copyable }">
    <button v-if="copyable" type="button" class="code-copy markdown-copy" aria-label="Markdownをコピー" title="Markdownをコピー" @click="copyText(source)">
      <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><rect x="8" y="8" width="12" height="12" rx="2"/><path d="M16 8V4H4v12h4"/></svg>
    </button>
    <!-- Raw HTML is disabled by the parser. Only its escaped output and trusted KaTeX markup are inserted. -->
    <div class="markdown-body" @click="copyCode" v-html="html" />
    <span class="code-copy-status" role="status">{{ copyMessage }}</span>
  </div>
</template>

<style scoped>
.copyable-markdown { position: relative; }
.copyable-markdown > .markdown-body { padding-inline-end: 44px; }
.markdown-copy { top: 0; right: 0; }
</style>
