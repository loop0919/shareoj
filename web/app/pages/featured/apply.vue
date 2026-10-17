<script setup lang="ts">
import { accountProblemSchema } from '~~/shared/types/account-problems'
import { featuredPlanSchema, type FeaturedPlan } from '~~/shared/types/featured'
import { accountError } from '~/utils/account-problems'
import { choosableFirst, featuredDay, preferenceLabels, problemChoice } from '~/utils/problem-choices'

useSeoMeta({ title: '定期便に応募 | ShareOJ', robots: 'noindex, nofollow' })
const route = useRoute()
const { problems, signedIn, loading, message, load } = useOwnProblems()
const selected = ref('')
const preference = ref<'soon' | 'later'>('soon')
const busy = ref(false)
const error = ref('')
const status = ref('')
const limit = 3
const used = computed(() => problems.value.filter(problem => problem.featuredPreference).length)
// Published problems never return to the queue, so only unpublished ones are listed.
const choices = computed(() => choosableFirst(problems.value.filter(problem => !problem.publishedVersion).map(problem => ({
  ...problemChoice(problem, 'featured', problem.featuredPreference ? `応募中・${preferenceLabels[problem.featuredPreference]}` : ''),
  slot: problem.difficulty ? problem.difficulty <= 4 ? 'Easy' : 'Hard' : undefined,
}))))
const current = computed(() => problems.value.find(problem => problem.id === selected.value))
const choosing = computed(() => !loading.value && !message.value && signedIn.value && choices.value.length > 0)
const preferences = (['soon', 'later'] as const).map(value => ({ value, label: preferenceLabels[value] }))
const unchanged = computed(() => !!current.value?.featuredPreference && current.value.featuredPreference === preference.value)
const label = computed(() => busy.value ? '応募中…' : unchanged.value ? '応募済み' : current.value?.featuredPreference ? '希望を変更' : '応募する')
watch(selected, () => {
  preference.value = current.value?.featuredPreference || 'soon'
  error.value = ''
  status.value = ''
})
watch(preference, () => { if (!unchanged.value) status.value = '' })

const plan = ref<FeaturedPlan>()
const planFailed = ref(false)
const { loading: planLoading, run } = useLatestRequest()
function loadPlan() {
  // Signed-in authors see their own problems and where the chosen one would land.
  const query: Record<string, string> = signedIn.value && selected.value ? { problem: selected.value, preference: preference.value } : {}
  const url = signedIn.value ? '/api/my/featured/schedule' : '/api/featured/schedule'
  return run(`${url}?${new URLSearchParams(query)}`, () => $fetch(url, { query }), value => {
    plan.value = featuredPlanSchema.parse(value)
    planFailed.value = false
  }, () => { planFailed.value = true })
}
watch([selected, preference], loadPlan)
const projection = computed(() => {
  if (!selected.value || !plan.value) return ''
  const index = plan.value.rounds.findIndex(round => round.slots.some(slot => slot.trial))
  if (index < 0) return plan.value.rounds.length ? `「${current.value?.title.trim() || '無題の問題'}」は${plan.value.rounds.length}回より先になる見込みです` : ''
  const round = plan.value.rounds[index]!
  const slot = round.slots.find(entry => entry.trial)!.slot === 'easy' ? 'Easy' : 'Hard'
  return `「${current.value?.title.trim() || '無題の問題'}」は${index ? '' : '次回、'}${featuredDay(round.scheduledAt)}の${slot}に入る見込みです`
})

onMounted(async () => {
  await load()
  const id = route.query.problem
  if (typeof id === 'string' && choices.value.some(choice => choice.id === id && !choice.reason)) selected.value = id
  else void loadPlan()
})

async function apply() {
  if (busy.value || !current.value || unchanged.value) return
  busy.value = true
  error.value = ''
  status.value = ''
  try {
    const problem = accountProblemSchema.parse(await $fetch(`/api/my/problems/${current.value.id}`))
    await $fetch(`/api/my/problems/${problem.id}/featured`, { method: 'PUT', body: { version: problem.version, preference: preference.value } })
    current.value.featuredPreference = preference.value
    await loadPlan()
    status.value = projection.value ? `応募しました。${projection.value}。` : '応募しました。'
  } catch (cause) { error.value = accountError(cause) }
  finally { busy.value = false }
}
</script>

<template>
  <div class="catalogue featured-apply">
    <nav class="breadcrumb" aria-label="パンくずリスト"><NuxtLink to="/featured">定期便</NuxtLink><span aria-hidden="true">/</span><span>応募</span></nav>
    <header class="catalogue-heading">
      <h1>定期便に応募</h1>
      <span v-if="signedIn && !loading" class="apply-quota">応募枠 <b>{{ used }}</b> / {{ limit }}</span>
    </header>
    <div class="apply-layout">
      <div class="apply-form">
        <LoadingSpinner v-if="loading" label="問題を読み込み中" />
        <template v-else-if="message">
          <p role="alert" class="editor-error">{{ message }}</p>
          <button type="button" class="editor-button" @click="load">再読み込み</button>
        </template>
        <p v-else-if="!signedIn"><NuxtLink to="/login?next=/featured/apply">ログイン</NuxtLink>すると、自分の問題を応募できます。</p>
        <EmptyState v-else-if="!choices.length" kind="problem" title="応募できる未公開の問題はありません。" description="解説とテストケースまで書いた問題を保存すると、ここから応募できます。"><NuxtLink class="editor-button" to="/problems/new?fresh=1">新規問題を作成</NuxtLink></EmptyState>
        <form v-else id="featured-apply-form" @submit.prevent="apply">
          <ProblemChoiceList v-model="selected" legend="問題" name="featured-problem" :choices="choices" :disabled="busy" hide-blocked-label="応募不可の問題を隠す" bounded />
          <p class="apply-note">応募すると、予定表に作者名が載ります。取り下げは「自分の問題」からできます。</p>
        </form>
      </div>
      <section class="apply-schedule" aria-labelledby="apply-schedule-title" :aria-busy="planLoading">
        <div class="apply-schedule-heading"><h2 id="apply-schedule-title">これからの出題</h2><NuxtLink to="/featured/schedule">すべての予定</NuxtLink></div>
        <template v-if="plan">
          <FeaturedSchedule :plan="plan" :rounds="6" :trial-label="current?.featuredPreference ? '応募中' : '応募した場合'" />
          <p class="apply-note">2回目以降は見込みです。</p>
        </template>
        <p v-else-if="planFailed" role="alert" class="editor-error">予定を取得できませんでした。<button type="button" class="editor-button" @click="loadPlan">再試行</button></p>
        <LoadingSpinner v-else label="予定を読み込み中" />
      </section>
    </div>
    <!-- Across both columns: however long the list grows, the choice, the preference it depends on and the button stay on screen. -->
    <ActionBar v-if="choosing" class="apply-bar" :status="status" :error="error" :message="projection" :placeholder="selected ? '' : '問題を選ぶと、入る見込みの回を表示します'">
      <SegmentedChoice v-model="preference" legend="希望" name="featured-preference" :options="preferences" :disabled="busy || !selected" hide-legend>
        <template #icon="{ value }"><FeaturedPreferenceIcon :preference="value" :size="20" /></template>
      </SegmentedChoice>
      <template #actions>
        <NuxtLink to="/docs/featured">出題のルール</NuxtLink>
        <button form="featured-apply-form" class="editor-button primary" :disabled="busy || !selected || unchanged" :aria-busy="busy">{{ label }}</button>
      </template>
    </ActionBar>
  </div>
</template>

<style scoped>
/* Hallmark · pre-emit critique: P4 H4 E4 S5 R5 V4 · existing ShareOJ tokens (Plain · modern-minimal)
 * Form left, schedule right from 60rem; stacked below. The schedule answers "when would mine go out" as the choice changes.
 * The list scrolls inside a bounded box; the projection, preference and button sit in a bar across both columns, pinned to the bottom, button at the right.
 * States: guest, loading, load error, empty, choosable/blocked rows, busy, applied, unchanged, request error, schedule error. */
.featured-apply > .breadcrumb { padding-top: 0; }
.apply-quota { color: var(--color-muted); font-size: .875rem; white-space: nowrap; font-variant-numeric: tabular-nums; }
.apply-quota b { color: var(--color-ink); font-family: var(--font-code); font-size: 1.125rem; font-weight: 600; }
.apply-layout { display: grid; grid-template-columns: minmax(0, 1fr); gap: 48px; }
.apply-form form { display: flex; flex-direction: column; gap: 16px; }
.apply-bar { margin-top: 32px; }
.apply-note { margin: 0; color: var(--color-muted); font-size: .8125rem; }
.apply-schedule { min-width: 0; }
.apply-schedule-heading { display: flex; flex-wrap: wrap; align-items: baseline; justify-content: space-between; gap: 4px 16px; margin-bottom: 8px; }
.apply-schedule-heading h2 { margin: 0; font-size: 1.125rem; }
.apply-schedule-heading a { display: inline-flex; align-items: center; min-height: 44px; font-size: .875rem; white-space: nowrap; }
.apply-schedule .apply-note { margin-top: 10px; }
.apply-schedule .editor-button { margin-left: 12px; }
@media (min-width: 60rem) {
  .apply-layout { grid-template-columns: minmax(0, 26rem) minmax(0, 1fr); gap: 56px; align-items: start; }
}
</style>
