<script setup lang="ts">
import type { Readiness } from '~~/shared/types/account-problems'
import { issueMessage } from '~/utils/problem-readiness'
const props = defineProps<{ problemId: string, title: string, readiness?: Readiness, scheduled: boolean, applied: boolean }>()
const dialog = ref<HTMLDialogElement>()
defineExpose({ open: () => dialog.value?.showModal() })
const reason = (destination: 'publish' | 'featured') => {
  const code = props.readiness?.[destination][0]
  return code ? issueMessage(code, destination) : ''
}
const options = computed(() => [
  { key: 'publish', to: { path: '/problems/post', query: { problem: props.problemId } }, title: props.scheduled ? '公開予約を変更' : '問題を投稿', detail: 'すぐに、または日時を決めて公開する', reason: reason('publish') },
  { key: 'featured', to: { path: '/featured/apply', query: { problem: props.problemId } }, title: props.applied ? '定期便の希望を変更' : '定期便に応募', detail: '月曜・木曜 23時の出題候補に入れる', reason: reason('featured') },
])
</script>

<template>
  <Teleport to="body">
    <dialog ref="dialog" class="publish-choice" aria-labelledby="publish-choice-title" aria-describedby="publish-choice-problem">
      <header>
        <div>
          <h2 id="publish-choice-title">公開・応募</h2>
          <p id="publish-choice-problem">{{ title || '無題の問題' }}</p>
        </div>
        <button type="button" class="editor-button close-button" aria-label="閉じる" @click="dialog?.close()"><svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" aria-hidden="true"><path d="m6 6 12 12M18 6 6 18" /></svg></button>
      </header>
      <ul>
        <li v-for="option in options" :key="option.key">
          <NuxtLink v-if="!option.reason" class="publish-option" :to="option.to" @click="dialog?.close()">
            <span class="option-mark" aria-hidden="true">
              <svg v-if="option.key === 'publish'" viewBox="0 0 24 24"><path d="M12 15V4m0 0-4.5 4.5M12 4l4.5 4.5M4 20h16" /></svg>
              <svg v-else viewBox="0 0 24 24"><rect x="3" y="5" width="18" height="16" rx="2" /><path d="M3 10h18M8 3v4m8-4v4" /></svg>
            </span>
            <span class="option-text"><strong>{{ option.title }}</strong><span>{{ option.detail }}</span></span>
            <svg class="option-arrow" viewBox="0 0 24 24" aria-hidden="true"><path d="m9 6 6 6-6 6" /></svg>
          </NuxtLink>
          <div v-else class="publish-option" aria-disabled="true">
            <span class="option-mark" aria-hidden="true">
              <svg v-if="option.key === 'publish'" viewBox="0 0 24 24"><path d="M12 15V4m0 0-4.5 4.5M12 4l4.5 4.5M4 20h16" /></svg>
              <svg v-else viewBox="0 0 24 24"><rect x="3" y="5" width="18" height="16" rx="2" /><path d="M3 10h18M8 3v4m8-4v4" /></svg>
            </span>
            <span class="option-text"><strong>{{ option.title }}</strong><span class="option-reason">{{ option.reason }}</span></span>
          </div>
        </li>
      </ul>
      <p class="publish-choice-note">保存済みの内容で判定しています。</p>
    </dialog>
  </Teleport>
</template>

<style scoped>
/* Hallmark · component: publish/apply chooser dialog · genre: modern-minimal · theme: existing ShareOJ tokens (Plain)
 * states: default · hover · focus-visible · active · disabled with reason · Escape/close button
 * Two rows that leave the editor for their own page; the dialog only asks which. */
.publish-choice { width: min(460px, calc(100vw - 32px)); max-height: calc(100dvh - 32px); padding: 24px; border: 1px solid var(--color-line); border-radius: 8px; color: var(--color-ink); background: var(--color-paper); }
.publish-choice::backdrop { background: var(--color-dialog-backdrop); }
header { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; }
h2 { margin: 0; font-size: 1.25rem; }
header p { margin: 2px 0 0; color: var(--color-muted); font-size: .8125rem; overflow-wrap: anywhere; }
.close-button { display: inline-flex; align-items: center; justify-content: center; flex-shrink: 0; width: 40px; height: 40px; padding: 0; border: 0; background: transparent; }
ul { display: grid; gap: 10px; margin: 20px 0 0; padding: 0; list-style: none; }
.publish-option { display: flex; align-items: center; gap: 14px; min-height: 72px; padding: 12px 16px; border: 1px solid var(--color-line); border-radius: 6px; color: var(--color-ink); text-decoration: none; }
a.publish-option:active { transform: translateY(1px); }
a.publish-option:focus-visible { outline: 3px solid var(--color-accent); outline-offset: 2px; }
.publish-option[aria-disabled] { background: var(--color-surface); color: var(--color-muted); }
.option-mark { display: inline-flex; align-items: center; justify-content: center; flex: none; width: 40px; height: 40px; border-radius: 50%; background: var(--color-accent-soft); color: var(--color-accent); }
.publish-option[aria-disabled] .option-mark { background: var(--color-paper); color: var(--color-muted); }
.option-mark svg, .option-arrow { width: 20px; height: 20px; fill: none; stroke: currentColor; stroke-width: 1.8; stroke-linecap: round; stroke-linejoin: round; }
.option-text { display: grid; flex: 1 1 auto; min-width: 0; line-height: 1.6; }
.option-text strong { font-size: 1rem; }
.option-text span { color: var(--color-muted); font-size: .8125rem; overflow-wrap: anywhere; }
.option-text .option-reason { color: var(--color-error); }
.option-arrow { flex: none; color: var(--color-muted); }
.publish-choice-note { margin: 16px 0 0; color: var(--color-muted); font-size: .75rem; }
@media (hover: hover) {
  a.publish-option:hover { background: var(--color-surface); }
  a.publish-option:hover .option-arrow { color: var(--color-accent); }
}
</style>
