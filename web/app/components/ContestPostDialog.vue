<script setup lang="ts">
import type { Contest, ContestList } from '~~/shared/types/contest'
const dialog = ref<HTMLDialogElement>()
const { refreshAccount } = useAccount()
const signedIn = ref(false)
const available = ref<Contest[]>([])
const selected = ref('')
const loading = ref(true)
const busy = ref(false)
const message = ref('')
async function open() {
  dialog.value?.showModal()
  await load()
}
defineExpose({ open })
async function load() {
  loading.value = true
  message.value = ''
  selected.value = ''
  available.value = []
  try {
    signedIn.value = !!await refreshAccount()
    if (!signedIn.value) return
    const contests: Contest[] = []
    let offset = 0
    let more = true
    while (more) {
      const page = await $fetch<ContestList>('/api/my/contests', { query: { offset } })
      contests.push(...page.items.filter(contest => contest.status === 'draft'))
      more = page.hasMore
      offset += 50
    }
    available.value = contests
  } catch { message.value = 'コンテストを読み込めませんでした。ログイン状態を確認して再読み込みしてください。' }
  finally { loading.value = false }
}
async function post() {
  if (busy.value || loading.value || !selected.value) return
  busy.value = true
  message.value = ''
  try {
    const contest = await $fetch<Contest>(`/api/my/contests/${selected.value}`)
    if (contest.status !== 'draft') { message.value = 'このコンテストはすでに公開されています。再読み込みして別のコンテストを選んでください。'; return }
    if (new Date(contest.startsAt).getTime() <= Date.now()) { message.value = '投稿するには、編集画面で開始日時を未来に変更して保存してください。'; return }
    const { title, description, startsAt, endsAt, penaltyMinutes, version } = contest
    await $fetch(`/api/my/contests/${contest.id}`, { method: 'PUT', body: {
      title, description, startsAt, endsAt, penaltyMinutes, version, publish: true,
      problems: contest.problems.map(({ id, points }) => ({ id, points })),
    } })
    dialog.value?.close()
    await navigateTo(`/contests/${contest.id}`)
  } catch (error) {
    const status = (error as { statusCode?: number }).statusCode
    message.value = status === 409 ? '投稿できませんでした。開催日時・問題の状態や別画面での更新を確認して、再読み込みしてください。' : '投稿を確認できませんでした。コンテストの公開状態とログイン状態を確認してください。'
  } finally { busy.value = false }
}
</script>
<template>
  <Teleport to="body">
    <dialog ref="dialog" class="contest-post-dialog" aria-labelledby="contest-post-title" aria-describedby="contest-post-description" @cancel="busy && $event.preventDefault()">
      <header><h2 id="contest-post-title">コンテストを投稿</h2><button type="button" class="editor-button close-button" aria-label="閉じる" :disabled="busy" @click="dialog?.close()"><svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" aria-hidden="true"><path d="m6 6 12 12M18 6 6 18" /></svg></button></header>
      <p id="contest-post-description">保存済みの未公開コンテストを選んで投稿します。</p>
      <p v-if="loading" role="status">コンテストを読み込んでいます…</p>
      <p v-if="message" role="alert" class="editor-error">{{ message }}</p>
      <button v-if="message" class="editor-button" :disabled="loading || busy" @click="load">再読み込み</button>
      <p v-if="!loading && !signedIn && !message"><NuxtLink to="/login?next=/contests?post=1">ログインして投稿する</NuxtLink></p>
      <form v-else-if="!loading && available.length" @submit.prevent="post">
        <label for="post-contest">投稿するコンテスト</label>
        <select id="post-contest" v-model="selected" required :disabled="busy"><option disabled value="">コンテストを選択してください</option><option v-for="contest in available" :key="contest.id" :value="contest.id">{{ contest.title }}</option></select>
        <p class="publication-note">投稿するとコンテストが一覧に公開されます。問題は開始日時に、解説は終了後に公開されます。</p>
        <p v-if="selected && !busy"><NuxtLink :to="`/my/contests/${selected}`">選択したコンテストを編集</NuxtLink></p>
        <footer><button type="button" class="editor-button" :disabled="busy" @click="dialog?.close()">キャンセル</button><button class="editor-button primary" :disabled="busy || !selected" :aria-busy="busy">{{ busy ? '投稿中…' : '投稿' }}</button></footer>
      </form>
      <p v-else-if="!loading && signedIn && !message">投稿できる未公開のコンテストはありません。<NuxtLink to="/my/contests/new">新規コンテストを作成</NuxtLink></p>
    </dialog>
  </Teleport>
</template>
<style scoped>
.contest-post-dialog { width: min(520px, calc(100vw - 32px)); max-height: calc(100dvh - 32px); overflow: auto; padding: 24px; border: 1px solid var(--color-line); border-radius: 8px; color: var(--color-ink); background: var(--color-paper); }
.contest-post-dialog::backdrop { background: var(--color-dialog-backdrop); }
header { display: flex; align-items: center; justify-content: space-between; gap: 16px; margin-bottom: 16px; }
h2 { margin: 0; font-size: 1.25rem; }
p { margin: 0 0 20px; font-size: .875rem; line-height: 1.8; }
.close-button { display: inline-flex; align-items: center; justify-content: center; flex-shrink: 0; width: 36px; height: 36px; padding: 0; border: 0; background: transparent; }
label { display: block; margin-bottom: 8px; font-size: .875rem; font-weight: 600; }
select { display: block; width: 100%; min-width: 0; min-height: 44px; padding: 10px 12px; border: 1px solid var(--color-line); border-radius: 4px; background: var(--color-paper); color: var(--color-ink); font: inherit; font-size: .875rem; }
select:focus-visible { outline: 2px solid var(--color-accent); outline-offset: 2px; }
.publication-note { margin-top: 16px; color: var(--color-muted); }
footer { display: flex; justify-content: flex-end; gap: 12px; padding-top: 20px; border-top: 1px solid var(--color-line); }
footer .editor-button { min-height: 40px; padding: 8px 20px; }
</style>
