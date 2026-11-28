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
// The sort and filters are in the URL, beside the page's view. A viewer's own list has no submitter to choose.
const { conditions: routeConditions, change, clear, query: conditionQuery } = useSubmissionConditions({ keep: ['view'], withUser: () => !props.mine })
// A problem's list has no 問題 condition: one in the URL is neither shown nor sent.
const conditions = computed(() => ({ ...routeConditions.value, problem: '' }))
const listQuery = computed(() => {
  const { problem: _problem, ...query } = conditionQuery.value
  return query
})
const { data: catalog } = useJudgeCatalog()
const languages = computed(() => catalog.value?.languages ?? [])
// Whether a condition may be what emptied the list.
const narrowed = computed(() => Object.keys(listQuery.value).length > 0)
// The panel stays as the viewer left it through refreshes and changes of conditions, and starts closed on each visit.
const conditionsOpen = ref(false)
const conditionsPanel = ref<{ focus: () => void }>()
const follow = useRejudgeFollow({ items: () => data.value?.items, narrowed: () => narrowed.value })
function load() {
  if (props.mine && !user.value) { invalidate(); data.value = null; return Promise.resolve() }
  const base = `/api/problems/${props.problemId}`
  const query = { mine: props.mine ? '1' : '0', offset: offset.value, ...listQuery.value }
  message.value = ''
  const read = follow.read(() => $fetch<{ items: Submission[], hasMore: boolean }>(`${base}/submissions`, { query }), result => { data.value = result })
  return run(JSON.stringify([base, query, user.value?.id]), read.fetch, read.apply,
    () => { message.value = '提出一覧を取得できませんでした。閲覧権限を確認し、再取得してください。' })
}
function detail(item: Submission) {
  return props.mine ? `/my/submissions/${shortId(item.id)}` : `/problems/${shortId(props.problemId)}/submissions/${shortId(item.id)}`
}
watch(() => [user.value?.id, props.problemId, props.mine], () => {
  invalidate(); data.value = null; offset.value = 0
}, { flush: 'sync' })
// New conditions list from the first page, in one request with them. The rows on screen stay until it answers.
watch(listQuery, () => { offset.value = 0 }, { flush: 'sync' })
watch([offset, () => user.value?.id, () => props.problemId, () => props.mine, listQuery], load)
onMounted(load)
usePolling(load, 15000)
// The empty list's button goes with the empty list, so the focus goes to the panel's button.
function clearEmpty() {
  void clear()
  conditionsPanel.value?.focus()
}

// Results judged before the problem became partially scored carry no score and show 「—」.
const showScore = computed(() => props.scored || !!data.value?.items.some(item => item.result?.scoring))
const { allowed: canRejudge, busy: rejudging, message: rejudgeMessage, rejudge } = useRejudge(() => props.problemId)
const { selected, selectable, rejudgeSelected } = useRejudgeSelection(() => data.value?.items, follow.follows(rejudge), load)
// Follow rejudged rows more closely than the regular refresh.
usePolling(load, 3000, { enabled: () => canRejudge.value && follow.active() })
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
      <SubmissionConditions ref="conditionsPanel" v-model:open="conditionsOpen" class="submission-conditions" :conditions="conditions" :languages="languages" :with-user="!mine" @change="change" @clear="clear" />
      <p v-if="message" class="notice notice-error" role="alert">{{ message }}</p>
      <LoadingSpinner v-if="loading && !data" label="提出一覧を読み込み中" />
      <template v-if="data">
        <template v-if="!data.items.length">
          <div v-if="narrowed" class="submission-empty">
            <p class="muted">条件に合う提出はありません。</p>
            <button type="button" class="editor-button" @click="clearEmpty">条件をすべて外す</button>
          </div>
          <p v-else class="muted">提出はまだありません。</p>
        </template>
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
.submission-conditions { margin-bottom: 20px; }
.submission-empty { display: flex; flex-wrap: wrap; align-items: center; gap: 8px 16px; }
.submission-empty p { margin: 0; }
</style>
