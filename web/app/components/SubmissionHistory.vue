<script setup lang="ts">
import { submissionUsage } from '~/utils/submission-usage'
import { runtimeLabel } from '~/utils/runtime-label'
import type { Submission } from '~~/shared/types/submission'
withDefaults(defineProps<{ embedded?: boolean }>(), { embedded: false })
const items = ref<Submission[]>([])
const message = ref('')
const { loading, run } = useLatestRequest()
function load() {
  message.value = ''
  const url = '/api/my/submissions'
  return run(url, () => $fetch<{ items: Submission[] }>(url),
    result => { items.value = result.items },
    () => { message.value = '提出履歴を取得できませんでした。ログイン状態を確認してください。' })
}
onMounted(load)
usePolling(load, 2000, () => !message.value && (items.value.some(item => item.status !== 'DONE')))
</script>

<template>
  <section class="draft-library">
    <div class="submission-heading">
      <div><component :is="embedded ? 'h2' : 'h1'" id="submission-history-title">提出履歴</component><p class="muted">最新50件を表示します。</p></div>
      <button class="editor-button" :disabled="loading" @click="load">{{ loading ? '更新中…' : '更新' }}</button>
    </div>
    <p v-if="message" role="alert">{{ message }}</p>
    <p v-if="loading && !items.length" role="status">読み込み中…</p>
    <p v-else-if="!items.length && !message">提出はまだありません。</p>
    <div v-if="items.length" class="submission-table-scroll" role="region" aria-labelledby="submission-history-title" tabindex="0">
      <table aria-label="提出履歴">
        <thead><tr><th scope="col">提出日時</th><th scope="col">問題</th><th scope="col">言語</th><th scope="col">コード長</th><th scope="col" title="各テストケースの最大CPU時間・最大メモリ使用量">実行時間・メモリ</th><th scope="col">結果</th><th scope="col">詳細</th></tr></thead>
        <tbody>
          <tr v-for="item in items" :key="item.id">
            <td class="submission-date"><time :datetime="item.createdAt">{{ new Date(item.createdAt).toLocaleString('ja-JP') }}</time></td>
            <td class="submission-problem"><NuxtLink :to="item.contestId ? `/contests/${item.contestId}/problems/${item.problemId}` : `/problems/${item.problemId}`">{{ item.problemTitle }}</NuxtLink></td>
            <td class="submission-language">{{ runtimeLabel(item.runtime) }}</td>
            <td class="submission-usage">{{ item.sourceBytes == null ? '—' : `${item.sourceBytes.toLocaleString('en-US')} bytes` }}</td>
            <td class="submission-usage">{{ submissionUsage(item.result) }}</td>
            <td class="submission-result"><span role="status"><SubmissionStatus :item="item" /></span></td>
            <td><NuxtLink :to="`/my/submissions/${item.id}`" :aria-label="`${item.problemTitle}の提出詳細`">詳細</NuxtLink></td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>

<style scoped>
/* Existing Plain theme: compact submission table with striped rows. */
.submission-heading { display: flex; align-items: center; justify-content: space-between; gap: 20px; margin-bottom: 24px; }
.submission-heading h1, .submission-heading h2 { margin-top: 0; margin-bottom: 8px; }
.submission-heading p { margin: 0; }
.submission-heading button { min-height: 44px; padding-inline: 16px; }
.submission-heading button:disabled { opacity: .5; cursor: not-allowed; transform: none; }
.submission-table-scroll { max-width: 100%; overflow-x: auto; border: 1px solid var(--color-line); border-radius: 4px; }
.submission-table-scroll:focus-visible { outline: 2px solid var(--color-accent); outline-offset: 3px; }
table { width: 100%; border-collapse: collapse; font-size: .875rem; }
th, td { padding: 12px 16px; text-align: left; border-bottom: 1px solid var(--color-line); }
th { background: var(--color-surface); font-weight: 600; white-space: nowrap; }
th + th, td + td { border-left: 1px solid var(--color-line); }
tbody tr:nth-child(even) { background: var(--color-surface); }
tbody tr:hover { background: var(--color-accent-soft); }
tbody tr:last-child td { border-bottom: 0; }
.submission-date { white-space: nowrap; font-variant-numeric: tabular-nums; }
.submission-problem { min-width: 200px; overflow-wrap: anywhere; }
.submission-usage { white-space: nowrap; font-variant-numeric: tabular-nums; }
.submission-language, .submission-result, td:last-child { white-space: nowrap; }
.submission-result { text-align: center; }
</style>
