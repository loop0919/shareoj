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
const saving = ref(false)
const problemId = ref('')
const question = ref('')
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
  if (saving.value) return
  saving.value = true; saveError.value = ''
  try {
    await $fetch(base, { method: 'POST', body: { problemId: problemId.value, question: question.value } })
    question.value = ''; offset.value = 0
    invalidate()
    showToast('質問を送信しました')
    await load()
  } catch (error) { saveError.value = failure(error) }
  finally { saving.value = false }
}
function startAnswer(item: Clarification) {
  edit.value = { id: item.id, answer: item.answer, version: item.version, published: !!item.publishedAt }
  saveError.value = ''
}
async function answer() {
  if (!edit.value || saving.value) return
  saving.value = true; saveError.value = ''
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
  finally { saving.value = false }
}
async function publish(item: Clarification) {
  if (saving.value || !window.confirm('質問・回答と投稿者名を、ログインしたすべての利用者に公開します。非公開には戻せません。公開しますか？')) return
  saving.value = true; saveError.value = ''
  try {
    await $fetch(`${base}/${item.id}`, { method: 'PUT', body: { publish: true, version: item.version } })
    invalidate()
    showToast('質問・回答を全体公開しました')
    await load()
  } catch (error) {
    saveError.value = failure(error)
    if (errorCode(error) === 'clarification_conflict') { invalidate(); await load() }
  }
  finally { saving.value = false }
}
</script>

<template>
  <section class="contest-section clarifications" aria-labelledby="clarifications-title">
    <h1 id="clarifications-title">質問</h1>
    <p v-if="!user" class="notice">質問・回答の閲覧には<NuxtLink :to="{ path: '/login', query: { next: $route.fullPath } }">ログイン</NuxtLink>が必要です。</p>
    <template v-else>
      <p class="muted">非公開の質問は、質問者本人・コンテスト作成者本人・対象問題の作問者とテスターだけが閲覧できます。全体公開された質問は、ログインしたすべての利用者が閲覧できます。</p>
      <form v-if="canAsk" class="clarification-form" @submit.prevent="ask">
        <h2>質問する</h2>
        <label for="clarification-problem">対象問題</label>
        <select id="clarification-problem" v-model="problemId" required :disabled="saving">
          <option value="" disabled>問題を選択</option>
          <option v-for="(problem, index) in contest.problems" :key="problem.id || index" :value="problem.id" :disabled="!problem.id">{{ problemLabel(index) }} - {{ problem.title }}</option>
        </select>
        <label for="clarification-question">質問（4,000文字以内）</label>
        <textarea id="clarification-question" v-model="question" required maxlength="4000" rows="5" :disabled="saving" aria-describedby="clarification-publication-note" />
        <p id="clarification-publication-note" class="muted">担当者の判断で質問・回答と投稿者名が全体公開されることがあります。個人情報は書かないでください。</p>
        <button class="editor-button primary" :disabled="saving || !question.trim() || !problemId">質問を送信</button>
      </form>
      <p v-else-if="!contest.staff" class="muted">質問は開催中のコンテストに参加すると送信できます。</p>
      <div class="clarification-actions"><h2>質問一覧</h2><button class="editor-button" :disabled="loading" @click="load">質問を更新</button></div>
      <p v-if="loadError" role="alert" class="notice">{{ loadError }}</p>
      <p v-if="saveError" role="alert" class="notice">{{ saveError }}</p>
      <p v-if="!items && loading" role="status">読み込み中…</p>
      <p v-else-if="items && !items.items.length" class="muted">閲覧できる質問はありません。</p>
      <article v-for="item in items?.items" :key="item.id" class="clarification" :aria-labelledby="`clarification-${item.id}`">
        <h3 :id="`clarification-${item.id}`">{{ label(item.problemId) }}</h3>
        <p class="muted">{{ item.publishedAt ? '全体公開' : '非公開' }} · {{ item.answer ? '回答済み' : '未回答' }} · <UserLink :handle="item.author" /> · <time :datetime="item.createdAt">{{ contestDate(item.createdAt) }}</time></p>
        <p class="clarification-text">{{ item.question }}</p>
        <template v-if="item.answer">
          <h4>回答</h4>
          <p class="clarification-text">{{ item.answer }}</p>
          <p v-if="item.answeredAt" class="muted"><UserLink :handle="item.answeredBy" /> · <time :datetime="item.answeredAt">{{ contestDate(item.answeredAt) }}</time></p>
        </template>
        <form v-if="item.canManage && edit?.id === item.id" class="clarification-form" @submit.prevent="answer">
          <label :for="`answer-${item.id}`">回答（4,000文字以内）</label>
          <textarea :id="`answer-${item.id}`" v-model="edit.answer" required maxlength="4000" rows="5" :disabled="saving" />
          <p v-if="edit.published || item.publishedAt" class="notice">この質問は全体公開されています。保存した回答も全体公開されます。</p>
          <div v-if="edit.version !== item.version" class="notice">
            <p>編集中に更新されました。上に表示された最新の回答と公開状態を確認してください。入力中の回答はそのまま残しています。</p>
            <button type="button" class="editor-button" :disabled="saving" @click="edit.version = item.version; saveError = ''">最新の内容を確認して編集を続ける</button>
          </div>
          <div class="clarification-actions"><button class="editor-button primary" :disabled="saving || !edit.answer.trim() || edit.version !== item.version">回答を保存</button><button type="button" class="editor-button" :disabled="saving" @click="edit = null">キャンセル</button></div>
        </form>
        <div v-else-if="item.canManage" class="clarification-actions">
          <button class="editor-button" :disabled="saving || !!edit" @click="startAnswer(item)">{{ item.answer ? '回答を編集' : '回答する' }}</button>
          <button v-if="!item.publishedAt" class="editor-button" :disabled="saving || !!edit" @click="publish(item)">全体公開する</button>
        </div>
      </article>
      <nav v-if="offset || items?.hasMore" class="clarification-actions" aria-label="質問一覧のページ">
        <button class="editor-button" :disabled="!offset || loading || !!edit" @click="offset -= 50">前の50件</button>
        <button class="editor-button" :disabled="!items?.hasMore || loading || !!edit" @click="offset += 50">次の50件</button>
      </nav>
    </template>
  </section>
</template>

<style scoped>
.clarifications > h1 { margin: 0 0 20px; font-size: 1.25rem; letter-spacing: normal; }
.clarification { border-top: 1px solid var(--color-border); padding: 24px 0; }
.clarification h3, .clarification h4 { margin: 0 0 12px; }
.clarification-text { white-space: pre-wrap; overflow-wrap: anywhere; }
.clarification-form { display: grid; gap: 12px; margin: 24px 0; }
.clarification-form textarea, .clarification-form select { width: 100%; font: inherit; padding: 10px; border: 1px solid var(--color-border); border-radius: 4px; color: var(--color-ink); background: var(--color-paper); }
.clarification-form textarea { resize: vertical; }
.clarification-form > button { justify-self: start; }
.clarification-actions { display: flex; align-items: center; flex-wrap: wrap; gap: 12px; }
</style>
