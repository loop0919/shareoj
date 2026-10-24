<script setup lang="ts">
import { contestDate, problemLabel, type Contest } from '~~/shared/types/contest'
import type { Clarification, ClarificationList } from '~~/shared/types/clarification'
import { errorCode } from '~/utils/contest-team'

const props = defineProps<{ contest: Contest }>()
const { user } = useAccount()
const { showToast } = useToast()
const base = `/api/my/contests/${props.contest.id}/clarifications`
const items = ref<ClarificationList | null>(null)
const offset = ref(0)
const loadError = ref('')
const saveError = ref('')
// The running action: 'ask', 'announce', 'answer', or the id of the question being published.
const busy = ref('')
const problemId = ref('')
const question = ref('')
const announcement = ref('')
const edit = ref<{ id: string, answer: string, version: number, published: boolean } | null>(null)
const canAsk = computed(() => props.contest.status === 'running' && props.contest.participating && props.contest.official)
const { loading, run, invalidate } = useLatestRequest()
async function load() {
  if (!user.value) return
  const page = offset.value
  await run(String(page), () => $fetch<ClarificationList>(base, { query: { offset: page } }),
    result => { items.value = result; loadError.value = '' },
    () => { items.value = null; loadError.value = '質問・回答を取得できませんでした。もう一度更新してください。' })
}
watch(offset, () => { items.value = null; void load() })
onMounted(load)
usePolling(load, 15000)
function label(id: string) {
  if (!id) return 'コンテスト全体'
  const index = props.contest.problems.findIndex(problem => problem.id === id)
  return index < 0 ? '問題' : `${problemLabel(index)} - ${props.contest.problems[index]!.title}`
}
function failure(error: unknown) {
  const code = errorCode(error)
  return code === 'clarification_conflict' ? '別の担当者が更新しました。一覧を更新し、回答を確認してからやり直してください。入力内容は残しています。'
    : code === 'clarification_unavailable' ? '質問は開催中のコンテストに参加している場合に送信できます。'
    : code === 'invalid_clarification' ? '本文は空白のみを除き、4,000文字以内で入力してください。'
    : '保存できませんでした。権限や通信状態を確認して、もう一度お試しください。'
}
async function ask() {
  if (busy.value) return
  busy.value = 'ask'; saveError.value = ''
  try {
    await $fetch(base, { method: 'POST', body: { problemId: problemId.value, question: question.value } })
    question.value = ''; offset.value = 0
    invalidate()
    showToast('質問を送信しました')
    await load()
  } catch (error) { saveError.value = failure(error) }
  finally { busy.value = '' }
}
async function announce() {
  if (busy.value || !window.confirm('回答と回答者名を、ログインしたすべての利用者に公開します。非公開には戻せません。公開しますか？')) return
  busy.value = 'announce'; saveError.value = ''
  try {
    await $fetch(base, { method: 'POST', body: { problemId: problemId.value, answer: announcement.value } })
    announcement.value = ''; offset.value = 0
    invalidate()
    showToast('回答を全体公開しました')
    await load()
  } catch (error) { saveError.value = failure(error) }
  finally { busy.value = '' }
}
function startAnswer(item: Clarification) {
  edit.value = { id: item.id, answer: item.answer, version: item.version, published: !!item.publishedAt }
  saveError.value = ''
}
async function answer() {
  if (!edit.value || busy.value) return
  busy.value = 'answer'; saveError.value = ''
  try {
    await $fetch(`${base}/${edit.value.id}`, { method: 'PUT', body: { answer: edit.value.answer, version: edit.value.version } })
    edit.value = null
    invalidate()
    showToast('回答を保存しました')
    await load()
  } catch (error) {
    saveError.value = failure(error)
    if (errorCode(error) === 'clarification_conflict') { invalidate(); await load() }
  }
  finally { busy.value = '' }
}
async function publish(item: Clarification) {
  if (busy.value || !window.confirm('質問・回答と投稿者名を、ログインしたすべての利用者に公開します。非公開には戻せません。公開しますか？')) return
  busy.value = item.id; saveError.value = ''
  try {
    await $fetch(`${base}/${item.id}`, { method: 'PUT', body: { publish: true, version: item.version } })
    invalidate()
    showToast('質問・回答を全体公開しました')
    await load()
  } catch (error) {
    saveError.value = failure(error)
    if (errorCode(error) === 'clarification_conflict') { invalidate(); await load() }
  }
  finally { busy.value = '' }
}
</script>

<template>
  <section class="contest-section clarifications" aria-labelledby="clarifications-title">
    <h1 id="clarifications-title">質問</h1>
    <p v-if="!user" class="notice">質問・回答の閲覧には<NuxtLink :to="{ path: '/login', query: { next: $route.fullPath } }">ログイン</NuxtLink>が必要です。</p>
    <template v-else>
      <form v-if="canAsk" class="clarification-form clarification-ask" aria-labelledby="clarification-ask-title" @submit.prevent="ask">
        <h2 id="clarification-ask-title">質問する</h2>
        <div class="clarification-field">
          <label for="clarification-problem">対象問題</label>
          <select id="clarification-problem" v-model="problemId" required :disabled="!!busy">
            <option value="" disabled>問題を選択</option>
            <option v-for="(problem, index) in contest.problems" :key="problem.id || index" :value="problem.id" :disabled="!problem.id">{{ problemLabel(index) }} - {{ problem.title }}</option>
          </select>
        </div>
        <div class="clarification-field">
          <label for="clarification-question">質問（4,000文字以内）</label>
          <textarea id="clarification-question" v-model="question" required maxlength="4000" rows="5" :disabled="!!busy" aria-describedby="clarification-publication-note" />
          <p id="clarification-publication-note" class="muted">担当者の判断で質問・回答と投稿者名が全体公開されることがあります。個人情報は書かないでください。</p>
        </div>
        <button class="editor-button primary clarification-submit" :disabled="!!busy || !question.trim() || !problemId" :aria-busy="busy === 'ask' || undefined">{{ busy === 'ask' ? '送信中…' : '質問を送信' }}</button>
      </form>
      <p v-else-if="!contest.staff" class="muted clarification-intro">質問は開催中のコンテストに参加すると送信できます。</p>
      <form v-if="contest.staff" class="clarification-form clarification-ask" aria-labelledby="clarification-announce-title" @submit.prevent="announce">
        <h2 id="clarification-announce-title">質問なしで回答を全体公開する</h2>
        <div class="clarification-field">
          <label for="clarification-announce-problem">対象問題</label>
          <select id="clarification-announce-problem" v-model="problemId" :disabled="!!busy">
            <option value="">コンテスト全体</option>
            <option v-for="(problem, index) in contest.problems" :key="problem.id || index" :value="problem.id" :disabled="!problem.id">{{ problemLabel(index) }} - {{ problem.title }}</option>
          </select>
        </div>
        <div class="clarification-field">
          <label for="clarification-announcement">全体公開する回答（4,000文字以内）</label>
          <textarea id="clarification-announcement" v-model="announcement" required maxlength="4000" rows="5" :disabled="!!busy" aria-describedby="clarification-announce-note" />
          <p id="clarification-announce-note" class="muted">質問者なしで回答を公開します。ログインしたすべての利用者が閲覧でき、参加者に通知されます。</p>
        </div>
        <button class="editor-button primary clarification-submit" :disabled="!!busy || !announcement.trim()" :aria-busy="busy === 'announce' || undefined">{{ busy === 'announce' ? '公開中…' : '回答を全体公開' }}</button>
      </form>
      <div class="clarification-heading"><h2>質問・回答一覧</h2><RefreshButton label="質問を更新" :busy="loading" :disabled="loading" @click="load" /></div>
      <p v-if="loadError" role="alert" class="notice notice-error">{{ loadError }}</p>
      <p v-if="saveError" role="alert" class="notice notice-error">{{ saveError }}</p>
      <LoadingSpinner v-if="!items && loading" label="質問を読み込み中" />
      <p v-else-if="items && !items.items.length" class="clarification-empty muted">閲覧できる質問・回答はありません。</p>
      <div v-if="items?.items.length" class="clarification-list">
        <article v-for="item in items.items" :key="item.id" class="clarification" :aria-labelledby="`clarification-${item.id}`">
          <div class="clarification-title">
            <h3 :id="`clarification-${item.id}`">{{ label(item.problemId) }}</h3>
            <span class="clarification-tag" :data-answered="!!item.answer || undefined">{{ !item.question ? 'スタッフからのお知らせ' : item.answer ? '回答済み' : '未回答' }}</span>
            <span class="clarification-tag">{{ item.publishedAt ? '全体公開' : '非公開' }}</span>
          </div>
          <p v-if="item.question" class="clarification-meta muted"><UserLink :handle="item.author" /><span aria-hidden="true">·</span><time :datetime="item.createdAt">{{ contestDate(item.createdAt) }}</time></p>
          <p v-if="item.question" class="clarification-text">{{ item.question }}</p>
          <div v-if="item.answer" class="clarification-answer">
            <h4>回答</h4>
            <p class="clarification-text">{{ item.answer }}</p>
            <p v-if="item.answeredAt" class="clarification-meta muted"><UserLink :handle="item.answeredBy" /><span aria-hidden="true">·</span><time :datetime="item.answeredAt">{{ contestDate(item.answeredAt) }}</time></p>
          </div>
          <form v-if="item.canManage && edit?.id === item.id" class="clarification-form clarification-reply" @submit.prevent="answer">
            <div class="clarification-field">
              <label :for="`answer-${item.id}`">回答（4,000文字以内）</label>
              <textarea :id="`answer-${item.id}`" v-model="edit.answer" required maxlength="4000" rows="5" :disabled="!!busy" />
            </div>
            <p v-if="edit.published || item.publishedAt" class="notice">この投稿は全体公開されています。保存した回答も全体公開されます。</p>
            <div v-if="edit.version !== item.version" class="notice clarification-conflict">
              <p>編集中に更新されました。上に表示された最新の回答と公開状態を確認してください。入力中の回答はそのまま残しています。</p>
              <button type="button" class="editor-button" :disabled="!!busy" @click="edit.version = item.version; saveError = ''">最新の内容を確認して編集を続ける</button>
            </div>
            <div class="clarification-actions"><button class="editor-button primary" :disabled="!!busy || !edit.answer.trim() || edit.version !== item.version" :aria-busy="busy === 'answer' || undefined">{{ busy === 'answer' ? '保存中…' : '回答を保存' }}</button><button type="button" class="editor-button" :disabled="busy === 'answer'" @click="edit = null">キャンセル</button></div>
          </form>
          <div v-else-if="item.canManage" class="clarification-actions">
            <button type="button" class="editor-button" :disabled="!!busy || !!edit" @click="startAnswer(item)">{{ item.answer ? '回答を編集' : '回答する' }}</button>
            <button v-if="!item.publishedAt" type="button" class="editor-button" :disabled="!!busy || !!edit" :aria-busy="busy === item.id || undefined" @click="publish(item)">{{ busy === item.id ? '公開中…' : '全体公開する' }}</button>
          </div>
        </article>
      </div>
      <ContentPagination v-if="offset || items?.hasMore" :index="offset / 50" :has-next="!!items?.hasMore" :loading="loading" :disabled="!!edit" @move="direction => offset += direction * 50" />
    </template>
  </section>
</template>

<style scoped>
/* Hallmark · component: contest clarifications · genre: modern-minimal · theme: existing ShareOJ tokens (Plain)
 * states: guest · loading · error · empty · asking · answering · conflict · busy · published
 * pre-emit critique: P4 H4 E4 S4 R5 V4
 * The ask form is a quiet bordered block like the team panel; questions are a ruled list like its members,
 * with tags like the participation division and the answer set off by an accent rule. */
.clarifications { max-width: 52rem; }
.clarifications > h1 { margin: 0 0 20px; font-size: 1.25rem; letter-spacing: normal; }
.clarifications h2 { margin: 0; font-size: 1rem; }
.clarification-intro { margin: 0 0 24px; }
.clarification-form { display: grid; gap: 12px; }
.clarification-ask { margin-bottom: 32px; padding: 16px 20px; border: 1px solid var(--color-line); border-radius: 4px; }
.clarification-field { display: grid; gap: 6px; min-width: 0; }
.clarification-field label { font-size: .875rem; font-weight: 600; }
.clarification-field p { margin: 0; }
.clarification-field select, .clarification-field textarea { width: 100%; min-width: 0; min-height: 44px; padding: 8px 10px; border: 1px solid var(--color-line); border-radius: 4px; font: inherit; }
.clarification-field select { max-width: 24rem; cursor: pointer; }
.clarification-field textarea { resize: vertical; line-height: 1.7; }
.clarification-field select:hover:not(:disabled), .clarification-field textarea:hover:not(:disabled) { border-color: var(--color-muted); }
.clarification-field select:focus-visible, .clarification-field textarea:focus-visible { outline: 3px solid var(--color-accent); outline-offset: 2px; }
.clarification-field textarea:user-invalid { border-color: var(--color-error); }
.clarification-field :disabled { opacity: .6; cursor: not-allowed; }
.clarification-form .editor-button, .clarification-actions .editor-button { min-height: 44px; padding: 8px 14px; }
.clarification-form .editor-button:disabled, .clarification-actions .editor-button:disabled { opacity: .6; cursor: not-allowed; transform: none; }
.clarification-submit { justify-self: start; font-weight: 600; }
.clarification-heading { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 16px; }
.clarification-heading h2 { min-width: 0; }
.clarification-empty { margin: 0; padding-block: 24px; border-top: 1px solid var(--color-line); }
.clarification-list { border-top: 1px solid var(--color-line); }
.clarification { padding-block: 20px; border-bottom: 1px solid var(--color-line); }
.clarification-title { display: flex; flex-wrap: wrap; align-items: center; gap: 4px 8px; margin-bottom: 4px; }
.clarification-title h3 { flex: 0 1 auto; margin: 0 4px 0 0; font-size: .9375rem; font-weight: 600; }
.clarification-tag { padding: 1px 8px; border: 1px solid var(--color-line); border-radius: 4px; color: var(--color-muted); font-size: .75rem; white-space: nowrap; }
.clarification-tag[data-answered] { border-color: var(--color-accent); color: var(--color-accent); background: var(--color-accent-soft); }
.clarification-meta { display: flex; flex-wrap: wrap; align-items: center; gap: 0 6px; margin: 0 0 12px; }
.clarification-text { margin: 0; white-space: pre-wrap; overflow-wrap: anywhere; line-height: 1.8; }
.clarification-answer { margin-top: 16px; padding: 4px 0 4px 16px; border-inline-start: 2px solid var(--color-accent); }
.clarification-answer h4 { margin: 0 0 4px; color: var(--color-accent); font-size: .8125rem; font-weight: 600; }
.clarification-answer .clarification-meta { margin: 8px 0 0; }
.clarification-reply { margin-top: 16px; }
.clarification-reply .notice { margin: 0; }
.clarification-conflict { display: grid; gap: 12px; }
.clarification-conflict p { margin: 0; color: inherit; }
.clarification-conflict .editor-button { justify-self: start; white-space: normal; }
.clarification-actions { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; margin-top: 16px; }
.clarification-reply .clarification-actions { margin-top: 0; }
</style>
