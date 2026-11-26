<script setup lang="ts">
import type { Submission } from '~~/shared/types/submission'
import { shortId } from '~~/shared/utils/short-id'
// A public problem's list; a contest's problems use the contest's lists (ADR 0034). scored adds the score column of a
// partially scored problem (ADR 0025).
const props = defineProps<{ problemId: string, mine: boolean, scored?: boolean }>()
const { user } = useAccount()
const route = useRoute()
const title = computed(() => props.mine ? '自分の提出' : 'すべての提出')
const data = ref<{ items: Submission[], hasMore: boolean } | null>(null)
const offset = ref(0)
const { loading, run, invalidate } = useLatestRequest()
const message = ref('')
function load() {
  if (props.mine && !user.value) { invalidate(); data.value = null; return Promise.resolve() }
  const base = `/api/problems/${props.problemId}`
  const query = { mine: props.mine ? '1' : '0', offset: offset.value }
  message.value = ''
  return run(JSON.stringify([base, query, user.value?.id]),
    () => $fetch<{ items: Submission[], hasMore: boolean }>(`${base}/submissions`, { query }),
    result => { data.value = result },
    () => { message.value = '提出一覧を取得できませんでした。閲覧権限を確認し、再取得してください。' })
}
function detail(item: Submission) {
  return props.mine ? `/my/submissions/${shortId(item.id)}` : `/problems/${shortId(props.problemId)}/submissions/${shortId(item.id)}`
}
watch(() => [user.value?.id, props.problemId, props.mine], () => {
  invalidate(); data.value = null; offset.value = 0
}, { flush: 'sync' })
watch([offset, () => user.value?.id, () => props.problemId, () => props.mine], load)
onMounted(load)
usePolling(load, 15000)

// Results judged before the problem became partially scored carry no score and show 「—」.
const showScore = computed(() => props.scored || !!data.value?.items.some(item => item.result?.scoring))
const { allowed: canRejudge, busy: rejudging, message: rejudgeMessage, rejudge } = useRejudge(() => props.problemId)
const { selected, selectable, rejudgeSelected } = useRejudgeSelection(() => data.value?.items, rejudge, load)
// Follow rejudged rows more closely than the regular refresh.
usePolling(load, 3000, { enabled: () => canRejudge.value && !!data.value?.items.some(item => item.status !== 'DONE') })
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
        <SubmissionTable v-if="data.items.length" v-model:selected="selected" :items="data.items" :detail="detail" :show-score="showScore" :select="canRejudge" :selectable="selectable" :aria-busy="loading" />
        <ContentPagination :index="offset / 50" :has-next="data.hasMore" :loading="loading" @move="direction => offset += direction * 50" />
      </template>
    </template>
  </section>
</template>
<style scoped>
.problem-submissions { padding-block: 32px 64px; }
.submission-heading { display: flex; align-items: center; justify-content: space-between; gap: 16px; margin-bottom: 20px; }
.submission-heading h2 { margin: 0; }
.submission-heading button { min-height: 40px; padding-inline: 16px; }
.heading-actions { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 8px; }
.rejudge-message { margin: 0 0 16px; color: var(--color-muted); font-size: .8125rem; }
</style>
