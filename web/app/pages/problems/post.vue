<script setup lang="ts">
import { accountProblemSchema } from '~~/shared/types/account-problems'
import { contestDate } from '~~/shared/types/contest'
import { shortId } from '~~/shared/utils/short-id'
import { accountError } from '~/utils/account-problems'
import { fromJapanInput, japanInput } from '~/utils/japan-time'
import { choosableFirst, problemChoice } from '~/utils/problem-choices'

useSeoMeta({ title: '問題を投稿 | ShareOJ', robots: 'noindex, nofollow' })
const route = useRoute()
const { showToast } = useToast()
const { problems, signedIn, loading, message, load } = useOwnProblems()
const selected = ref('')
const timing = ref<'now' | 'schedule'>('now')
const scheduledAt = ref('')
const busy = ref(false)
const error = ref('')
const invalidDate = ref(false)
const candidates = computed(() => problems.value.filter(problem => !problem.publishedVersion))
const choices = computed(() => choosableFirst(candidates.value.map(problem => problemChoice(problem, 'publish',
  problem.scheduledAt ? `公開予約中・${contestDate(problem.scheduledAt)}` : problem.featuredPreference ? '定期便に応募中' : ''))))
const current = computed(() => candidates.value.find(problem => problem.id === selected.value))
// A reservation and a 定期便 application exclude each other.
const applied = computed(() => !!current.value?.featuredPreference)
const reserved = computed(() => !!current.value?.scheduledAt)
const timings = computed(() => [
  { value: 'now' as const, label: '今すぐ' },
  { value: 'schedule' as const, label: '日時を指定', disabled: applied.value, describedby: applied.value ? 'post-applied' : undefined },
])
const label = computed(() => busy.value ? '送信中…' : timing.value === 'now' ? '投稿する' : reserved.value ? '予約を更新' : '予約する')
const when = (value: string) => {
  const date = fromJapanInput(value)
  return Number.isFinite(date.getTime()) ? new Intl.DateTimeFormat('ja-JP', { month: 'numeric', day: 'numeric', weekday: 'short', hour: '2-digit', minute: '2-digit', timeZone: 'Asia/Tokyo' }).format(date) : ''
}
// The bar says what the button will do, so the choice reads back even when the list has scrolled away.
const summary = computed(() => {
  if (!current.value) return ''
  const title = `「${current.value.title.trim() || '無題の問題'}」`
  if (timing.value === 'now') return `${title}を今すぐ公開します`
  return scheduledAt.value && when(scheduledAt.value) ? `${title}を${when(scheduledAt.value)}に公開します` : `${title}の公開日時を選んでください`
})
watch(selected, () => {
  const at = current.value?.scheduledAt
  timing.value = at ? 'schedule' : 'now'
  scheduledAt.value = at ? japanInput(at) : ''
  error.value = ''
})
// An error describes the last attempt; editing the choice clears it.
watch([timing, scheduledAt], () => { error.value = ''; invalidDate.value = false })

onMounted(async () => {
  await load()
  const id = route.query.problem
  if (typeof id === 'string' && choices.value.some(choice => choice.id === id && !choice.reason)) selected.value = id
})

async function post(cancel = false) {
  if (busy.value || !current.value) return
  error.value = ''
  let at: string | undefined
  if (!cancel && timing.value === 'schedule') {
    const date = fromJapanInput(scheduledAt.value)
    if (!scheduledAt.value || !Number.isFinite(date.getTime()) || date.getTime() <= Date.now()) { invalidDate.value = true; return }
    at = date.toISOString()
  }
  busy.value = true
  try {
    const problem = accountProblemSchema.parse(await $fetch(`/api/my/problems/${current.value.id}`))
    if (problem.publishedVersion) { error.value = 'この問題はすでに公開されています。'; return }
    await $fetch(`/api/my/problems/${problem.id}/publication`, { method: 'PUT', body: { version: problem.version, publish: !cancel, ...(at ? { scheduledAt: at } : {}) } })
    if (at || cancel) return await navigateTo('/my?tab=problems')
    await navigateTo(`/problems/${shortId(problem.id)}`)
    showToast('公開しました')
  } catch (cause) { error.value = accountError(cause, 'publish') }
  finally { busy.value = false }
}
</script>

<template>
  <div class="catalogue problem-post">
    <nav class="breadcrumb" aria-label="パンくずリスト"><NuxtLink to="/problems">問題</NuxtLink><span aria-hidden="true">/</span><span>投稿</span></nav>
    <header class="catalogue-heading"><h1>問題を投稿</h1></header>
    <LoadingSpinner v-if="loading" label="問題を読み込み中" />
    <template v-else-if="message">
      <p role="alert" class="editor-error">{{ message }}</p>
      <button type="button" class="editor-button" @click="load">再読み込み</button>
    </template>
    <p v-else-if="!signedIn"><NuxtLink to="/login?next=/problems/post">ログイン</NuxtLink>すると、自分の問題を投稿できます。</p>
    <EmptyState v-else-if="!choices.length" kind="problem" title="投稿できる未公開の問題はありません。" description="問題を作成して保存すると、ここから公開できます。"><NuxtLink class="editor-button" to="/problems/new?fresh=1">新規問題を作成</NuxtLink></EmptyState>
    <form v-else class="post-form" @submit.prevent="post()">
      <div class="post-columns">
        <ProblemChoiceList v-model="selected" legend="問題" name="post-problem" :choices="choices" :disabled="busy" hide-blocked-label="投稿不可の問題を隠す" bounded />
        <div class="post-timing">
          <SegmentedChoice v-model="timing" legend="公開のタイミング" name="post-timing" :options="timings" :disabled="busy || !selected" />
          <p v-if="applied" id="post-applied" class="post-hint">定期便に応募中のため予約できません。予約するには「自分の問題」で応募を取り下げてください。</p>
          <div v-if="timing === 'schedule'" class="post-when">
            <label for="post-scheduled-at">公開日時 (JST)</label>
            <input id="post-scheduled-at" v-model="scheduledAt" type="datetime-local" required :disabled="busy" :aria-invalid="invalidDate || undefined" :aria-describedby="invalidDate ? 'post-date-error post-when-hint' : 'post-when-hint'">
            <p v-if="invalidDate" id="post-date-error" class="field-error" role="alert">未来の公開日時をJSTで指定してください。</p>
            <p id="post-when-hint" class="post-hint">予約した時点の保存済みの内容を公開します。そのあとの編集は、予約を更新すると反映されます。</p>
          </div>
          <p v-else-if="selected" class="post-hint">保存済みの内容を公開し、誰でも閲覧・提出できるようにします。</p>
        </div>
      </div>
      <!-- However long the list grows, the choice and the buttons stay on screen. -->
      <ActionBar class="post-bar" :error="error" :message="summary" placeholder="問題を選んで、公開のタイミングを決めてください">
        <template #actions>
          <NuxtLink to="/docs/authoring#readiness">公開の条件</NuxtLink>
          <button v-if="reserved" type="button" class="editor-button" :disabled="busy" @click="post(true)">予約を取り消す</button>
          <button class="editor-button primary" :disabled="busy || !selected" :aria-busy="busy">{{ label }}</button>
        </template>
      </ActionBar>
    </form>
  </div>
</template>

<style scoped>
/* Hallmark · pre-emit critique: P4 H4 E4 S5 R5 V4 · existing ShareOJ tokens (Plain · modern-minimal)
 * Same shape as the 定期便 application: a bounded list on the left, the timing on the right, the action bar across both with the button at the bottom right.
 * Under 60rem: list, timing, then the bar, pinned to the bottom of the screen.
 * States: guest, loading, load error, empty, blocked rows (hideable), scheduled (update/cancel), applied (no schedule), invalid date, busy, request error. */
.problem-post > .breadcrumb { padding-top: 0; }
.post-form { display: flex; flex-direction: column; gap: 20px; }
.post-columns { display: grid; grid-template-columns: minmax(0, 1fr); gap: 28px; }
.post-timing { display: grid; gap: 12px; min-width: 0; }
.post-when { display: grid; gap: 6px; }
.post-when label { font-size: .8125rem; font-weight: 600; }
.post-when input { width: 100%; max-width: 18rem; min-height: 44px; padding: 8px 12px; border: 1px solid var(--color-line); border-radius: 4px; background: var(--color-paper); color: var(--color-ink); font: inherit; font-size: .9375rem; }
.post-when input:focus-visible { outline: 3px solid var(--color-accent); outline-offset: 3px; }
.post-when input[aria-invalid] { border-color: var(--color-error); }
.post-when input:disabled { opacity: .55; cursor: not-allowed; }
.post-when .field-error { margin: 0; }
.post-hint { margin: 0; color: var(--color-muted); font-size: .8125rem; line-height: 1.7; }
@media (hover: hover) {
  .post-when input:not(:disabled):hover { background: var(--color-surface); }
}
@media (min-width: 60rem) {
  .post-columns { grid-template-columns: minmax(0, 26rem) minmax(0, 26rem); gap: 56px; align-items: start; }
}
</style>
