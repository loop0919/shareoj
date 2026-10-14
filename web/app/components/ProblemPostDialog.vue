<script setup lang="ts">
import { accountListSchema, accountProblemSchema, type AccountSummary } from '~~/shared/types/account-problems'
import { accountError } from '~/utils/account-problems'
const props = defineProps<{ featured?: boolean }>()
const emit = defineEmits<{ posted: [] }>()
const preference = ref<'soon' | 'later'>('soon')
const dialog = ref<HTMLDialogElement>()
const { refreshAccount } = useAccount()
const { showToast } = useToast()
const signedIn = ref(false)
async function open() {
  selected.value = ''
  timing.value = 'now'
  scheduledAt.value = ''
  dialog.value?.showModal()
  await load()
  if (typeof route.query.problem === 'string' && available.value.some(p => p.id === route.query.problem)) selected.value = route.query.problem
}
defineExpose({ open })
const available = ref<AccountSummary[]>([])
const selected = ref('')
const route = useRoute()
const timing = ref('now')
const scheduledAt = ref('')
const selectedProblem = computed(() => available.value.find(p => p.id === selected.value))
const scheduleLabel = (at: string) => new Date(at).toLocaleString('ja-JP', { timeZone: 'Asia/Tokyo' })
watch(selected, () => {
  const at = selectedProblem.value?.scheduledAt
  timing.value = at ? 'schedule' : 'now'
  scheduledAt.value = at ? new Date(new Date(at).getTime() + 9 * 60 * 60 * 1000).toISOString().slice(0, 16) : ''
})
const loading = ref(true)
const busy = ref(false)
const message = ref('')
async function load() {
  loading.value = true
  message.value = ''
  available.value = []
  try {
    signedIn.value = !!await refreshAccount()
    if (!signedIn.value) return
    let cursor = ''
    do {
      const page = accountListSchema.parse(await $fetch('/api/my/problems', { query: { cursor } }))
      available.value.push(...page.items.filter(p => !p.publishedVersion && !p.contestId && (!props.featured || (!p.everPublished && !p.scheduledAt))))
      cursor = page.nextCursor
    } while (cursor)
  } catch (error) { message.value = accountError(error) }
  finally { loading.value = false }
}
async function post(cancelSchedule = false) {
  if (busy.value || loading.value || !selected.value) return
  busy.value = true
  message.value = ''
  try {
    let at: string | undefined
    if (!props.featured && !cancelSchedule && timing.value === 'schedule') {
      const date = new Date(`${scheduledAt.value}:00+09:00`)
      if (!scheduledAt.value || !Number.isFinite(date.getTime()) || date.getTime() <= Date.now()) { message.value = '未来の公開日時を日本時間で指定してください。'; return }
      at = date.toISOString()
    }
    const problem = accountProblemSchema.parse(await $fetch(`/api/my/problems/${selected.value}`))
    if (!cancelSchedule && (!problem.draft.title.trim() || !problem.draft.markdown.trim() || problem.draft.difficulty == null)) { message.value = '投稿するには問題のタイトル・本文・難易度を設定してください。'; return }
    if (problem.publishedVersion) { message.value = 'この問題はすでに公開されています。別の問題を選んでください。'; return }
    if (props.featured) {
      await $fetch(`/api/my/problems/${problem.id}/featured`, { method: 'PUT', body: { version: problem.version, preference: preference.value } })
      dialog.value?.close()
      emit('posted')
      return
    }
    await $fetch(`/api/my/problems/${problem.id}/publication`, { method: 'PUT', body: { version: problem.version, publish: !cancelSchedule, ...(at ? { scheduledAt: at } : {}) } })
    dialog.value?.close()
    await navigateTo(at || cancelSchedule ? '/my?tab=problems' : `/problems/${problem.id}`)
    if (!at && !cancelSchedule) showToast('公開しました')
  } catch (error) { message.value = accountError(error) }
  finally { busy.value = false }
}
</script>
<template>
  <Teleport to="body">
    <dialog ref="dialog" class="problem-post-dialog" aria-labelledby="post-title" :aria-describedby="featured ? undefined : 'post-description'" @cancel="busy && $event.preventDefault()">
      <header><h2 id="post-title">{{ featured ? '定期便に応募' : '問題を投稿' }}</h2><button type="button" class="editor-button close-button" aria-label="閉じる" :disabled="busy" @click="dialog?.close()"><svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" aria-hidden="true"><path d="m6 6 12 12M18 6 6 18" /></svg></button></header>
      <p v-if="!featured" id="post-description">未公開の問題を選んで投稿します。コンテストに登録した問題は選べません。</p>
      <LoadingSpinner v-if="loading" label="問題を読み込み中" />
      <p v-if="message" role="alert" class="editor-error">{{ message }}</p>
      <button v-if="message" class="editor-button" :disabled="loading || busy" @click="load">再読み込み</button>
      <p v-if="!loading && !signedIn && !message"><NuxtLink :to="featured ? '/login?next=/featured' : '/login?next=/problems?post=1'">ログインして投稿する</NuxtLink></p>
      <form v-else-if="!loading && signedIn" @submit.prevent="post()">
        <template v-if="available.length">
          <label for="post-problem">投稿する問題</label>
          <select id="post-problem" v-model="selected" required :disabled="busy"><option disabled value="">問題を選択してください</option><option v-for="problem in available" :key="problem.id" :value="problem.id">{{ problem.title.trim() || '無題の問題' }}{{ problem.scheduledAt ? '（公開予約中）' : '' }}</option></select>
          <template v-if="featured">
            <label class="publication-note" for="featured-preference">公開の希望</label>
            <select id="featured-preference" v-model="preference" :disabled="busy"><option value="soon">早めに出したい</option><option value="later">ゆっくりで良い</option></select>
          </template>
          <template v-else>
            <p v-if="selectedProblem?.scheduledAt" class="publication-note">公開予定：{{ scheduleLabel(selectedProblem.scheduledAt) }}（日本時間）</p>
            <label class="publication-note" for="post-timing">公開するタイミング</label>
            <select id="post-timing" v-model="timing" :disabled="busy"><option value="now">今すぐ公開</option><option value="schedule">日時を指定して予約</option></select>
            <template v-if="timing === 'schedule'">
              <label class="publication-note" for="post-scheduled-at">公開日時（日本時間）</label>
              <input id="post-scheduled-at" v-model="scheduledAt" type="datetime-local" required :disabled="busy">
              <p class="publication-note">指定した日時に、予約時の保存済みの内容を公開します。その後の編集を反映するには予約を更新してください。</p>
              <p v-if="selectedProblem?.featuredPreference" class="publication-note">定期便に応募中です。「自分の問題」で応募を取り下げてから予約してください。</p>
            </template>
            <p v-else class="publication-note">保存済みの内容が公開され、誰でも閲覧・提出できるようになります。</p>
            <button v-if="selectedProblem?.scheduledAt" type="button" class="editor-button" :disabled="busy" @click="post(true)">予約を取り消す</button>
          </template>
        </template>
        <p v-else-if="!message">投稿できる未公開の問題はありません。<NuxtLink to="/problems/new?fresh=1">新規問題を作成</NuxtLink></p>
        <details v-if="featured" class="application-details">
          <summary>応募条件・出題のルール</summary>
          <ul>
            <li>完成した未公開問題が対象です。難易度と解説、1件以上のテストケースが必要です。</li>
            <li>応募は作者ごとに3件までです。</li>
            <li>公開したことのある問題、コンテストに登録した問題は応募できません。</li>
            <li>毎週月曜・木曜23時（日本時間）に、Easy（Lv.1〜4）とHard（Lv.5〜10）を一問ずつ出題します。</li>
            <li>「早めに出したい」2回・「ゆっくりで良い」1回を基本に、それぞれ応募順で選びます。</li>
            <li>出題までは作者とテスターだけが問題を確認できます。</li>
          </ul>
        </details>
        <footer v-if="available.length"><button type="button" class="editor-button" :disabled="busy" @click="dialog?.close()">キャンセル</button><button class="editor-button primary" :disabled="busy || !selected" :aria-busy="busy">{{ busy ? (featured ? '応募中…' : '投稿中…') : (featured ? '応募する' : timing === 'schedule' ? (selectedProblem?.scheduledAt ? '予約を更新' : '予約する') : '投稿') }}</button></footer>
      </form>
    </dialog>
  </Teleport>
</template>
<style scoped>
.problem-post-dialog { width: min(520px, calc(100vw - 32px)); max-height: calc(100dvh - 32px); overflow: auto; padding: 24px; border: 1px solid var(--color-line); border-radius: 8px; color: var(--color-ink); background: var(--color-paper); }
.problem-post-dialog::backdrop { background: var(--color-dialog-backdrop); }
header { display: flex; align-items: center; justify-content: space-between; gap: 16px; margin-bottom: 16px; }
h2 { margin: 0; font-size: 1.25rem; }
p { margin: 0 0 20px; font-size: .875rem; line-height: 1.8; }
.close-button { display: inline-flex; align-items: center; justify-content: center; flex-shrink: 0; width: 36px; height: 36px; padding: 0; border: 0; background: transparent; }
label { display: block; margin-bottom: 8px; font-size: .875rem; font-weight: 600; }
select, input { display: block; width: 100%; min-width: 0; min-height: 44px; padding: 10px 12px; border: 1px solid var(--color-line); border-radius: 4px; background: var(--color-paper); color: var(--color-ink); font: inherit; font-size: .875rem; }
select:focus-visible, input:focus-visible { outline: 2px solid var(--color-accent); outline-offset: 2px; }
.publication-note { margin-top: 16px; color: var(--color-muted); }
.application-details { margin-top: 16px; color: var(--color-muted); font-size: .8125rem; }
.application-details summary { width: fit-content; padding-block: 10px; cursor: pointer; }
.application-details summary:hover, .application-details summary:active { color: var(--color-accent); }
.application-details summary:focus-visible { outline: 2px solid var(--color-accent); outline-offset: 2px; border-radius: 2px; }
.application-details ul { margin: 4px 0 16px; padding-left: 1.5em; }
.application-details li { margin-block: 8px; overflow-wrap: anywhere; }
footer { display: flex; justify-content: flex-end; gap: 12px; padding-top: 20px; border-top: 1px solid var(--color-line); }
footer .editor-button { min-height: 40px; padding: 8px 20px; }
</style>
