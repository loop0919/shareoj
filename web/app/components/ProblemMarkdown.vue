<script setup lang="ts">
import { renderProblemMarkdown } from '~/utils/problem-markdown'

// A live preview re-renders while its source is edited. It marks source lines for scroll sync and
// keeps folds open across renders: by position while the count is unchanged (such as while a title
// is edited), otherwise by title.
const props = defineProps<{ source: string, live?: boolean }>()
const html = computed(() => renderProblemMarkdown(props.source, { sourceLines: props.live }))
const body = ref<HTMLElement>()
let folds: { title: string, open: boolean }[] = []
const foldElements = () => [...body.value?.querySelectorAll('details') ?? []]
const foldTitle = (fold: HTMLDetailsElement) => fold.querySelector(':scope > summary')?.textContent ?? ''
watch(html, () => {
  if (props.live) folds = foldElements().map(fold => ({ title: foldTitle(fold), open: fold.open }))
}, { flush: 'pre' })
watch(html, () => {
  if (!props.live || !folds.some(fold => fold.open)) return
  const current = foldElements()
  if (current.length === folds.length) {
    current.forEach((fold, index) => { fold.open = folds[index]!.open })
    return
  }
  const byTitle = new Map<string, boolean[]>()
  for (const fold of folds) byTitle.set(fold.title, [...byTitle.get(fold.title) ?? [], fold.open])
  for (const fold of current) fold.open = byTitle.get(foldTitle(fold))?.shift() ?? false
}, { flush: 'post' })
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
  <div ref="body" class="markdown-body" @click="copyCode" v-html="html" />
  <span class="code-copy-status" role="status">{{ copyMessage }}</span>
</template>
