<script setup lang="ts">
import { accountProblemSchema } from '~~/shared/types/account-problems'
import { contestDate } from '~~/shared/types/contest'
import { shortId } from '~~/shared/utils/short-id'
import { accountError } from '~/utils/account-problems'
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
const label = computed(() => busy.value ? '送信中…' : timing.value === 'now' ? '投稿する' : reserved.value ? '予約を更新' : '予約する')
const japanTime = (at: string) => new Date(new Date(at).getTime() + 9 * 60 * 60 * 1000).toISOString().slice(0, 16)
watch(selected, () => {
  const at = current.value?.scheduledAt
  timing.value = at ? 'schedule' : 'now'
  scheduledAt.value = at ? japanTime(at) : ''
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
    const date = new Date(`${scheduledAt.value}:00+09:00`)
    if (!scheduledAt.value || !Number.isFinite(date.getTime()) || date.getTime() <= Date.now()) { error.value = '未来の公開日時を日本時間で指定してください。'; invalidDate.value = true; return }
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
  } catch (cause) { error.value = accountError(cause) }
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
      <ProblemChoiceList v-model="selected" legend="問題" name="post-problem" :choices="choices" :disabled="busy" hide-blocked-label="投稿不可の問題を隠す" />
      <fieldset class="post-timing" :disabled="busy || !selected">
        <legend>公開のタイミング</legend>
        <div class="post-pills">
          <label><input v-model="timing" type="radio" name="post-timing" value="now">今すぐ</label>
          <label><input v-model="timing" type="radio" name="post-timing" value="schedule" :disabled="applied" :aria-describedby="applied ? 'post-applied' : undefined">日時を指定</label>
        </div>
        <p v-if="applied" id="post-applied" class="post-hint">定期便に応募中のため予約できません。予約するには「自分の問題」で応募を取り下げてください。</p>
        <div v-if="timing === 'schedule'" class="post-when">
          <label for="post-scheduled-at">公開日時（日本時間）</label>
          <input id="post-scheduled-at" v-model="scheduledAt" type="datetime-local" required :aria-invalid="invalidDate || undefined" :aria-describedby="invalidDate ? 'post-when-hint post-error' : 'post-when-hint'">
          <p id="post-when-hint" class="post-hint">予約した時点の保存済みの内容を公開します。そのあとの編集は、予約を更新すると反映されます。</p>
        </div>
        <p v-else class="post-hint">保存済みの内容を公開し、誰でも閲覧・提出できるようにします。</p>
      </fieldset>
      <p v-if="error" id="post-error" role="alert" class="editor-error">{{ error }}</p>
      <div class="post-actions">
        <button v-if="reserved" type="button" class="editor-button" :disabled="busy" @click="post(true)">予約を取り消す</button>
        <button class="editor-button primary" :disabled="busy || !selected" :aria-busy="busy">{{ label }}</button>
      </div>
    </form>
  </div>
</template>

<style scoped>
/* Hallmark · pre-emit critique: P4 H4 E4 S5 R5 V4 · existing ShareOJ tokens (Plain · modern-minimal)
 * One column of choices: problem, then timing, then the action. The constraint between reservations and 定期便 sits on the disabled option.
 * States: guest, loading, load error, empty, blocked rows, scheduled (update/cancel), applied (no schedule), invalid date, busy, request error. */
.problem-post > .breadcrumb { padding-top: 0; }
.post-form { display: grid; gap: 28px; max-width: 36rem; }
.post-timing { min-width: 0; margin: 0; padding: 0; border: 0; }
.post-timing legend { margin-bottom: 8px; padding: 0; font-size: .875rem; font-weight: 600; }
.post-pills { display: flex; flex-wrap: wrap; gap: 8px; }
.post-pills label { display: inline-flex; align-items: center; gap: 8px; min-height: 44px; padding: 6px 18px 6px 14px; border: 1px solid var(--color-line); border-radius: 999px; font-size: .875rem; white-space: nowrap; cursor: pointer; }
.post-pills label:has(input:checked) { border-color: var(--color-accent); background: var(--color-accent-soft); }
.post-pills label:has(input:disabled), .post-timing:disabled label { opacity: .55; cursor: not-allowed; }
.post-pills input { width: 16px; height: 16px; margin: 0; accent-color: var(--color-accent); }
.post-timing input:focus-visible { outline: 3px solid var(--color-accent); outline-offset: 3px; }
.post-when { display: grid; gap: 6px; margin-top: 16px; }
.post-when label { font-size: .8125rem; font-weight: 600; }
.post-when input[aria-invalid] { border-color: var(--color-error); }
.post-when input { width: 100%; max-width: 18rem; min-height: 44px; padding: 8px 12px; border: 1px solid var(--color-line); border-radius: 4px; background: var(--color-paper); color: var(--color-ink); font: inherit; font-size: .9375rem; }
.post-hint { margin: 10px 0 0; color: var(--color-muted); font-size: .8125rem; line-height: 1.7; }
.post-when .post-hint { margin-top: 0; }
.post-form .editor-error { margin: 0; }
.post-actions { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 12px; }
.post-actions .editor-button { min-height: 44px; padding: 8px 22px; font-size: .9375rem; }
.post-actions .primary { padding-inline: 28px; font-weight: 600; }
.post-actions .editor-button:disabled { opacity: .55; cursor: not-allowed; text-decoration: none; }
@media (hover: hover) {
  .post-pills label:not(:has(input:checked)):not(:has(input:disabled)):hover, .post-when input:hover { background: var(--color-surface); }
}
</style>
