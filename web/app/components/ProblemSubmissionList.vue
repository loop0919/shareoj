<script setup lang="ts">
import { contestDate } from '~~/shared/types/contest'
import type { Submission } from '~~/shared/types/submission'
import { shortId } from '~~/shared/utils/short-id'
import { submissionUsage } from '~/utils/submission-usage'
import { runtimeLabel } from '~/utils/runtime-label'
import { formatScore } from '~/utils/score'
// team lists the viewer's and their teammates' submissions in a team contest; onsite keeps the onsite participants'
// (ADR 0024). scored adds the score column of a partially scored problem (ADR 0025).
const props = defineProps<{ problemId: string, contestId?: string, mine: boolean, team?: boolean, onsite?: boolean, scored?: boolean }>()
const { user, profile } = useAccount()
const route = useRoute()
const title = computed(() => props.team ? 'チームの提出' : props.mine ? '自分の提出' : 'すべての提出')
const data = ref<{ items: Submission[], hasMore: boolean } | null>(null)
const offset = ref(0)
const { loading, run, invalidate } = useLatestRequest()
const message = ref('')
function load() {
  if (props.mine && !user.value) { invalidate(); data.value = null; return Promise.resolve() }
  const base = props.contestId ? `/api/contests/${props.contestId}/problems/${props.problemId}` : `/api/problems/${props.problemId}`
  const query = { mine: props.team ? 'team' : props.mine ? '1' : '0', offset: offset.value, ...props.onsite && !props.mine ? { scope: 'onsite' } : {} }
  message.value = ''
  return run(JSON.stringify([base, query, user.value?.id]),
    () => $fetch<{ items: Submission[], hasMore: boolean }>(`${base}/submissions`, { query }),
    result => { data.value = result },
    () => { message.value = '提出一覧を取得できませんでした。閲覧権限を確認し、再取得してください。' })
}
function detail(item: Submission) {
  // A teammate's submission is not the viewer's own, so it opens on the shared detail page. Either way the breadcrumb
  // leads back to 「チームの提出」.
  if (props.mine && (!props.team || item.author === profile.value?.handle)) return props.contestId ? `/contests/${shortId(props.contestId)}/my-submissions/${shortId(item.id)}${props.team ? '?from=team' : ''}` : `/my/submissions/${shortId(item.id)}?from=problem`
  return props.contestId ? `/contests/${shortId(props.contestId)}/submissions/${shortId(item.id)}?from=${props.team ? 'team' : 'problem'}` : `/problems/${shortId(props.problemId)}/submissions/${shortId(item.id)}`
}
watch(() => [user.value?.id, props.problemId, props.contestId, props.mine, props.team, props.onsite], () => {
  invalidate(); data.value = null; offset.value = 0
}, { flush: 'sync' })
watch([offset, () => user.value?.id, () => props.problemId, () => props.contestId, () => props.mine, () => props.team, () => props.onsite], load)
onMounted(load)
usePolling(load, 15000)

// Results judged before the problem became partially scored carry no score and show 「—」.
const showScore = computed(() => props.scored || !!data.value?.items.some(item => item.result?.scoring))
const { allowed: canRejudge, busy: rejudging, message: rejudgeMessage, rejudge } = useRejudge(() => props.problemId)
const selected = ref<string[]>([])
// Only finished judgements can be rejudged. Selection belongs to the visible page.
const selectable = computed(() => data.value?.items.filter(item => item.status === 'DONE').map(item => item.id) ?? [])
const allSelected = computed(() => selectable.value.length > 0 && selectable.value.every(id => selected.value.includes(id)))
watch(selectable, ids => { selected.value = selected.value.filter(id => ids.includes(id)) })
async function rejudgeSelected() {
  const count = selected.value.length
  if (!count || !window.confirm(`選択した${count}件の提出をリジャッジしますか？\n保存済みの問題の内容で採点し直します。`)) return
  if (await rejudge([...selected.value])) { selected.value = []; await load() }
}
// Follow rejudged rows more closely than the regular refresh.
usePolling(load, 3000, () => canRejudge.value && !!data.value?.items.some(item => item.status !== 'DONE'))
</script>
<template>
  <section class="problem-submissions" :aria-label="title">
    <header class="submission-heading">
      <h2>{{ title }}</h2>
      <div class="heading-actions">
        <!-- Appears only with a selection, so the list stays quiet until someone picks rows. -->
        <RejudgeButton v-if="canRejudge && selected.length" :busy="rejudging" :disabled="rejudging" :aria-label="`選択した${selected.length}件の提出をリジャッジ`" @click="rejudgeSelected" />
        <RefreshButton v-if="!mine || user" :busy="loading" :disabled="loading" @click="load" />
      </div>
    </header>
    <p v-if="rejudgeMessage" class="rejudge-message" role="status">{{ rejudgeMessage }}</p>
    <p v-if="mine && !user" class="notice"><NuxtLink :to="{ path: '/login', query: { next: route.fullPath } }">ログイン</NuxtLink>すると、この問題への自分の提出を確認できます。</p>
    <template v-else>
      <p v-if="message" class="notice notice-error" role="alert">{{ message }}</p>
      <LoadingSpinner v-if="loading && !data" label="提出一覧を読み込み中" />
      <template v-if="data">
        <p v-if="!data.items.length" class="muted">提出はまだありません。</p>
        <div v-if="data.items.length" class="content-table-scroll" role="region" aria-label="提出一覧のスクロール領域" tabindex="0" :aria-busy="loading">
          <table class="content-table"><thead><tr><th v-if="canRejudge" scope="col" class="select-cell"><input type="checkbox" :checked="allSelected" :disabled="!selectable.length" aria-label="このページの提出をすべて選択" @change="selected = ($event.target as HTMLInputElement).checked ? [...selectable] : []"></th><th scope="col">提出日時 (JST)</th><th scope="col">ユーザー</th><th scope="col">言語</th><th scope="col">コード長</th><th scope="col" title="各テストケースの最大CPU時間・最大メモリ使用量">実行時間・メモリ</th><th v-if="showScore" scope="col">得点</th><th scope="col">結果</th><th scope="col">詳細</th></tr></thead>
            <tbody><tr v-for="item in data.items" :key="item.id"><td v-if="canRejudge" class="select-cell"><input v-model="selected" type="checkbox" :value="item.id" :disabled="!selectable.includes(item.id)" :aria-label="`${contestDate(item.createdAt)}の提出を選択`"></td><td><time :datetime="item.createdAt">{{ contestDate(item.createdAt) }}</time></td><td><UserLink :handle="item.author" /></td><td>{{ runtimeLabel(item.runtime) }}</td><td class="submission-usage">{{ item.sourceBytes == null ? '—' : `${item.sourceBytes.toLocaleString('en-US')} bytes` }}</td><td class="submission-usage">{{ submissionUsage(item.result) }}</td><td v-if="showScore" class="submission-score" :data-full="(item.result?.scoring && item.result.scoring.score === item.result.scoring.maxScore) || undefined">{{ item.result?.scoring ? formatScore(item.result.scoring.score) : '—' }}</td><td><SubmissionStatus :item="item" /></td><td><NuxtLink :to="detail(item)">詳細</NuxtLink></td></tr></tbody>
          </table>
        </div>
        <ContentPagination :index="offset / 50" :has-next="data.hasMore" :loading="loading" @move="direction => offset += direction * 50" />
      </template>
    </template>
  </section>
</template>
<style scoped>
.submission-usage { white-space: nowrap; font-variant-numeric: tabular-nums; }
.submission-score { text-align: right; white-space: nowrap; font-variant-numeric: tabular-nums; }
.submission-score[data-full] { color: var(--color-accent); font-weight: 600; }
.problem-submissions { padding-block: 32px 64px; }
.submission-heading { display: flex; align-items: center; justify-content: space-between; gap: 16px; margin-bottom: 20px; }
.submission-heading h2 { margin: 0; }
.submission-heading button { min-height: 40px; padding-inline: 16px; }
.heading-actions { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 8px; }
.rejudge-message { margin: 0 0 16px; color: var(--color-muted); font-size: .8125rem; }
.select-cell { width: 44px; text-align: center; }
.select-cell input { width: 18px; height: 18px; margin: 0; accent-color: var(--color-accent); cursor: pointer; }
.select-cell input:disabled { cursor: not-allowed; }
</style>
