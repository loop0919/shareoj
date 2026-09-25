<script setup lang="ts">
import { accountListSchema, accountProblemSchema, type AccountSummary } from '~~/shared/types/account-problems'
import { accountError } from '~/utils/account-problems'
const props = defineProps<{ featured?: boolean }>()
const emit = defineEmits<{ posted: [] }>()
const preference = ref<'soon' | 'later'>('soon')
const dialog = ref<HTMLDialogElement>()
const { refreshAccount } = useAccount()
const signedIn = ref(false)
async function open() {
  selected.value = ''
  dialog.value?.showModal()
  await load()
}
defineExpose({ open })
const available = ref<AccountSummary[]>([])
const selected = ref('')
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
      available.value.push(...page.items.filter(p => !p.publishedVersion && !p.contestId && (!props.featured || !p.everPublished)))
      cursor = page.nextCursor
    } while (cursor)
  } catch (error) { message.value = accountError(error) }
  finally { loading.value = false }
}
async function post() {
  if (busy.value || loading.value || !selected.value) return
  busy.value = true
  message.value = ''
  try {
    const problem = accountProblemSchema.parse(await $fetch(`/api/my/problems/${selected.value}`))
    if (!problem.draft.title.trim() || !problem.draft.markdown.trim()) { message.value = '投稿するには問題のタイトルと本文を入力してください。'; return }
    if (problem.publishedVersion) { message.value = 'この問題はすでに公開されています。別の問題を選んでください。'; return }
    if (props.featured) {
      await $fetch(`/api/my/problems/${problem.id}/featured`, { method: 'PUT', body: { version: problem.version, preference: preference.value } })
      dialog.value?.close()
      emit('posted')
      return
    }
    await $fetch(`/api/my/problems/${problem.id}/publication`, { method: 'PUT', body: { version: problem.version, publish: true } })
    dialog.value?.close()
    await navigateTo(`/problems/${problem.id}`)
  } catch (error) { message.value = accountError(error) }
  finally { busy.value = false }
}
</script>
<template>
  <Teleport to="body">
    <dialog ref="dialog" class="problem-post-dialog" aria-labelledby="post-title" aria-describedby="post-description" @cancel="busy && $event.preventDefault()">
      <header><h2 id="post-title">{{ featured ? '定期便に応募' : '問題を投稿' }}</h2><button type="button" class="editor-button close-button" aria-label="閉じる" :disabled="busy" @click="dialog?.close()"><svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" aria-hidden="true"><path d="m6 6 12 12M18 6 6 18" /></svg></button></header>
      <p id="post-description">{{ featured ? '完成した未公開問題を応募します。難易度と解説が必須です。応募は作者ごとに3件まで。公開したことのある問題と、コンテストに登録した問題は選べません。' : '未公開の問題を選んで投稿します。コンテストに登録した問題は選べません。' }}</p>
      <p v-if="loading" role="status">問題を読み込んでいます…</p>
      <p v-if="message" role="alert" class="editor-error">{{ message }}</p>
      <button v-if="message" class="editor-button" :disabled="loading || busy" @click="load">再読み込み</button>
      <p v-if="!loading && !signedIn && !message"><NuxtLink :to="featured ? '/login?next=/featured' : '/login?next=/problems?post=1'">ログインして投稿する</NuxtLink></p>
      <form v-else-if="!loading && available.length" @submit.prevent="post">
        <label for="post-problem">投稿する問題</label>
        <select id="post-problem" v-model="selected" required :disabled="busy"><option disabled value="">問題を選択してください</option><option v-for="problem in available" :key="problem.id" :value="problem.id">{{ problem.title.trim() || '無題の問題' }}</option></select>
        <template v-if="featured">
          <label class="publication-note" for="featured-preference">公開の希望</label>
          <select id="featured-preference" v-model="preference" :disabled="busy"><option value="soon">早めに出したい</option><option value="later">あとからでもよい</option></select>
          <p class="publication-note">Easy（Lv.1〜4）・Hard（Lv.5〜10）から各一問を選び、月曜・木曜23時に出題します。「早め」2回・「あとから」1回を基本に、それぞれ応募順で選びます。選出までは作者とテスターだけが確認できます。</p>
        </template>
        <p v-else class="publication-note">保存済みの内容が公開され、誰でも閲覧・提出できるようになります。</p>
        <footer><button type="button" class="editor-button" :disabled="busy" @click="dialog?.close()">キャンセル</button><button class="editor-button primary" :disabled="busy || !selected" :aria-busy="busy">{{ busy ? (featured ? '応募中…' : '投稿中…') : (featured ? '応募する' : '投稿') }}</button></footer>
      </form>
      <p v-else-if="!loading && signedIn && !message">投稿できる未公開の問題はありません。<NuxtLink to="/problems/new?fresh=1">新規問題を作成</NuxtLink></p>
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
select { display: block; width: 100%; min-width: 0; min-height: 44px; padding: 10px 12px; border: 1px solid var(--color-line); border-radius: 4px; background: var(--color-paper); color: var(--color-ink); font: inherit; font-size: .875rem; }
select:focus-visible { outline: 2px solid var(--color-accent); outline-offset: 2px; }
.publication-note { margin-top: 16px; color: var(--color-muted); }
footer { display: flex; justify-content: flex-end; gap: 12px; padding-top: 20px; border-top: 1px solid var(--color-line); }
footer .editor-button { min-height: 40px; padding: 8px 20px; }
</style>
