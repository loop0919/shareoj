<script setup lang="ts">
import { contestDate } from '~~/shared/types/contest'
import type { Submission } from '~~/shared/types/submission'
import { submissionUsage } from '~/utils/submission-usage'
import { runtimeLabel } from '~/utils/runtime-label'
const props = defineProps<{ problemId: string, contestId?: string, mine: boolean }>()
const { user } = useAccount()
const route = useRoute()
const title = computed(() => props.mine ? '自分の提出' : 'すべての提出')
const data = ref<{ items: Submission[], hasMore: boolean } | null>(null)
const offset = ref(0)
const { loading, run, invalidate } = useLatestRequest()
const message = ref('')
function load() {
  if (props.mine && !user.value) { invalidate(); data.value = null; return Promise.resolve() }
  const base = props.contestId ? `/api/contests/${props.contestId}/problems/${props.problemId}` : `/api/problems/${props.problemId}`
  const query = { mine: props.mine ? '1' : '0', offset: offset.value }
  message.value = ''
  return run(JSON.stringify([base, query, user.value?.id]),
    () => $fetch<{ items: Submission[], hasMore: boolean }>(`${base}/submissions`, { query }),
    result => { data.value = result },
    () => { message.value = '提出一覧を取得できませんでした。閲覧権限を確認し、再取得してください。' })
}
function detail(item: Submission) {
  if (props.mine) return `/my/submissions/${item.id}?from=${props.contestId ? 'contest-problem' : 'problem'}`
  return props.contestId ? `/contests/${props.contestId}/submissions/${item.id}?from=problem` : `/problems/${props.problemId}/submissions/${item.id}`
}
watch(() => [user.value?.id, props.problemId, props.contestId, props.mine], () => {
  invalidate(); data.value = null; offset.value = 0
}, { flush: 'sync' })
watch([offset, () => user.value?.id, () => props.problemId, () => props.contestId, () => props.mine], load)
onMounted(load)
usePolling(load, 15000)
</script>
<template>
  <section class="problem-submissions" :aria-label="title">
    <header class="submission-heading"><h2>{{ title }}</h2><button v-if="!mine || user" class="editor-button" :disabled="loading" @click="load">{{ loading ? '更新中…' : '更新' }}</button></header>
    <p v-if="mine && !user" class="notice"><NuxtLink :to="{ path: '/login', query: { next: route.fullPath } }">ログイン</NuxtLink>すると、この問題への自分の提出を確認できます。</p>
    <template v-else>
      <p v-if="message" class="notice notice-error" role="alert">{{ message }}</p>
      <p v-if="loading && !data" class="muted" role="status">読み込み中…</p>
      <template v-if="data">
        <p v-if="!data.items.length" class="muted">提出はまだありません。</p>
        <div v-else class="content-table-scroll" role="region" aria-label="提出一覧のスクロール領域" tabindex="0" :aria-busy="loading">
          <table class="content-table"><thead><tr><th scope="col">提出日時（日本時間）</th><th scope="col">ユーザー</th><th scope="col">言語</th><th scope="col">コード長</th><th scope="col" title="各テストケースの最大CPU時間・最大メモリ使用量">実行時間・メモリ</th><th scope="col">結果</th><th scope="col">詳細</th></tr></thead>
            <tbody><tr v-for="item in data.items" :key="item.id"><td><time :datetime="item.createdAt">{{ contestDate(item.createdAt) }}</time></td><td><UserLink :handle="item.author" /></td><td>{{ runtimeLabel(item.runtime) }}</td><td class="submission-usage">{{ item.sourceBytes == null ? '—' : `${item.sourceBytes.toLocaleString('en-US')} bytes` }}</td><td class="submission-usage">{{ submissionUsage(item.result) }}</td><td><SubmissionStatus :item="item" /></td><td><NuxtLink :to="detail(item)">詳細</NuxtLink></td></tr></tbody>
          </table>
        </div>
        <ContentPagination :index="offset / 50" :has-next="data.hasMore" :loading="loading" @move="direction => offset += direction * 50" />
      </template>
    </template>
  </section>
</template>
<style scoped>
.submission-usage { white-space: nowrap; font-variant-numeric: tabular-nums; }
.problem-submissions { padding-block: 32px 64px; }
.submission-heading { display: flex; align-items: center; justify-content: space-between; gap: 16px; margin-bottom: 20px; }
.submission-heading h2 { margin: 0; }
.submission-heading button { min-height: 40px; padding-inline: 16px; }
</style>
