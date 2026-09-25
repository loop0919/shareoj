<script setup lang="ts">
import { contestDate } from '~~/shared/types/contest'
import { featuredApplicationsSchema, type FeaturedApplication } from '~~/shared/types/featured'
import { accountProblemSchema } from '~~/shared/types/account-problems'
import { accountError } from '~/utils/account-problems'

useResponseHeader('Cache-Control').value = 'no-store'
useSeoMeta({ title: '定期便 | ShareOJ', description: '毎週月曜・木曜23時（日本時間）にEasyとHardを一問ずつ出題します。新作の解説は翌22時に公開。' })
const offset = ref(0)
const { data, error, status, refresh } = await useFetch('/api/featured', { query: { offset } })
const dialog = ref<{ open: () => Promise<void> }>()
const { user, refreshAccount } = useAccount()
const applications = ref<FeaturedApplication[]>([])
const message = ref('')
const busy = ref(false)
const { run, invalidate } = useLatestRequest()
function loadApplications() {
  if (!user.value) { invalidate(); applications.value = []; return Promise.resolve() }
  message.value = ''
  return run(user.value.id, () => $fetch<unknown>('/api/my/featured'), result => {
    applications.value = featuredApplicationsSchema.parse(result).items
  }, error => { message.value = accountError(error) })
}
async function withdraw(id: string) {
  if (busy.value) return
  busy.value = true
  message.value = ''
  try {
    const problem = accountProblemSchema.parse(await $fetch(`/api/my/problems/${id}`))
    await $fetch(`/api/my/problems/${id}/featured`, { method: 'PUT', body: { version: problem.version, preference: '' } })
    await Promise.all([loadApplications(), refresh()])
  } catch (error) { message.value = accountError(error) }
  finally { busy.value = false }
}
watch(() => user.value?.id, () => { applications.value = []; void loadApplications() })
onMounted(async () => { await refreshAccount(); await loadApplications() })
usePolling(() => refresh(), 60000)
</script>

<template>
  <div class="catalogue featured-page">
    <ProblemPostDialog ref="dialog" featured @posted="() => { loadApplications(); refresh() }" />
    <header class="catalogue-heading"><h1>定期便</h1><button class="editor-button primary" @click="dialog?.open()">新作を応募</button></header>
    <p>毎週月曜・木曜23時（日本時間）に、Easy（Lv.1〜4）とHard（Lv.5〜10）を一問ずつ出題します。</p>
    <p class="muted">新作の解説と他者の提出は翌22時に公開します。復刻は解説公開済みです。</p>
    <FeaturedDelivery v-if="data" :page="data" />
    <p v-if="error" role="alert">定期便を取得できませんでした。<button class="editor-button" @click="refresh()">再試行</button></p>
    <section v-if="user" class="applications" aria-labelledby="applications-title">
      <h2 id="applications-title">応募中の問題</h2>
      <p v-if="message" role="alert">{{ message }} <button class="editor-button" @click="loadApplications">再試行</button></p>
      <p v-if="!applications.length && !message" class="muted">応募中の問題はありません。</p>
      <ul v-else>
        <li v-for="application in applications" :key="application.problemId">
          <NuxtLink :to="{ path: '/problems/new', query: { problem: application.problemId } }">{{ application.title }}</NuxtLink>
          <span>{{ application.preference === 'soon' ? '早めに出したい' : 'あとからでもよい' }}</span>
          <button class="editor-button" :disabled="busy" @click="withdraw(application.problemId)">取り下げる</button>
        </li>
      </ul>
      <p class="muted">応募後も編集できます。難易度・解説などの公開条件を満たさなくなると、修正するまで選出されません。通常公開やコンテスト登録をすると応募は解除されます。</p>
    </section>
    <section aria-labelledby="history-title" :aria-busy="status === 'pending'">
      <h2 id="history-title">{{ offset ? '過去の出題' : '最新・過去の出題' }}</h2>
      <p v-if="data && !data.items.length" class="muted">まだ出題されていません。次回の定期便をお待ちください。</p>
      <article v-for="round in data?.items" :key="round.scheduledAt" class="round">
        <h3><time :datetime="round.scheduledAt">{{ contestDate(round.scheduledAt) }}</time></h3>
        <div class="slots">
          <section v-for="slot in round.slots" :key="slot.slot" class="slot">
            <div class="slot-heading"><h4>{{ slot.slot === 'easy' ? 'Easy' : (slot.difficulty ?? 0) >= 9 ? 'Ultimate' : 'Hard' }}</h4><span>{{ slot.kind === 'new' ? '新作' : slot.kind === 'revival' ? '復刻' : '欠番' }}</span></div>
            <p v-if="slot.kind === 'missing'" class="muted">この枠の出題はありません。</p>
            <template v-else>
              <p v-if="slot.problemId"><NuxtLink :to="`/problems/${slot.problemId}`">{{ slot.title }}</NuxtLink> <DifficultyBadge :level="slot.difficulty" /></p>
              <p v-else class="muted">現在、この問題は公開されていません。</p>
              <p v-if="slot.editorialHidden" class="muted">解説・他者の提出：{{ contestDate(slot.revealAt) }} 公開</p>
              <p v-else class="muted">解説公開済み<template v-if="slot.problemId"> · <NuxtLink :to="{ path: `/problems/${slot.problemId}`, query: { view: 'editorial' } }">解説を読む</NuxtLink></template></p>
            </template>
          </section>
        </div>
      </article>
      <ContentPagination v-if="data" :index="offset / 20" :has-next="data.hasMore" :loading="status === 'pending'" @move="direction => offset += direction * 20" />
    </section>
  </div>
</template>

<style scoped>
.featured-page > p { line-height: 1.8; }
.applications { margin-block: 32px; padding-block: 24px; border-block: 1px solid var(--color-line); }
.applications ul { padding: 0; list-style: none; }
.applications li { display: flex; align-items: center; flex-wrap: wrap; gap: 12px; margin-block: 12px; }
.applications li button { min-height: 40px; padding-inline: 12px; }
.applications li span { color: var(--color-muted); font-size: .875rem; }
.round { margin-block: 28px; }
.round h3 { font-size: 1rem; font-variant-numeric: tabular-nums; }
.slots { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16px; }
.slot { padding: 20px; border: 1px solid var(--color-line); border-radius: 4px; }
.slot-heading { display: flex; align-items: baseline; gap: 12px; }
.slot h4 { margin: 0; font-size: 1.125rem; }
.slot-heading span, .slot p { font-size: .875rem; }
.slot p { line-height: 1.8; overflow-wrap: anywhere; }
@media (max-width: 600px) { .slots { grid-template-columns: minmax(0, 1fr); } }
</style>
