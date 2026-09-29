<script setup lang="ts">
import { accountListSchema, accountProblemSchema, type AccountSummary } from '~~/shared/types/account-problems'
import { accountError } from '~/utils/account-problems'
const props = defineProps<{ testing?: boolean }>()
const heading = computed(() => props.testing ? 'テスト中の問題' : '自分の問題')
const { user, refreshAccount } = useAccount()
const accountEntries = ref<AccountSummary[]>([])
const accountLoading = ref(true)
const accountMessage = ref('')
const nextCursor = ref('')
let listedOwner = ''
const withdrawing = ref('')
const withdrawDialog = ref<HTMLDialogElement>()
const withdrawEntry = ref<AccountSummary>()
const withdrawMessage = ref('')
function confirmWithdraw(entry: AccountSummary) {
  if (withdrawing.value || props.testing) return
  withdrawEntry.value = entry
  withdrawMessage.value = ''
  withdrawDialog.value?.showModal()
}
const tooltipId = useId()
const preferenceLabel = (preference: string) => preference === 'soon' ? '早めに出したい' : 'ゆっくりで良い'
async function withdraw(entry: AccountSummary) {
  if (withdrawing.value || props.testing) return
  withdrawing.value = entry.id
  withdrawMessage.value = ''
  try {
    const problem = accountProblemSchema.parse(await $fetch(`/api/my/problems/${entry.id}`))
    await $fetch(`/api/my/problems/${entry.id}/featured`, { method: 'PUT', body: { version: problem.version, preference: '' } })
    entry.featuredPreference = ''
    withdrawDialog.value?.close()
  } catch (error) { withdrawMessage.value = accountError(error) }
  finally { withdrawing.value = '' }
}
async function loadAccount(more = false) {
  accountLoading.value = true
  accountMessage.value = ''
  try {
    const account = await refreshAccount()
    if (!account) { accountEntries.value = []; nextCursor.value = ''; await navigateTo('/login?next=/my%3Ftab%3Dproblems', { replace: true }); return }
    if (listedOwner !== account.id) { more = false; accountEntries.value = []; nextCursor.value = ''; listedOwner = account.id }
    const result = accountListSchema.parse(await $fetch('/api/my/problems', { query: { ...(props.testing ? { role: 'tester' } : {}), ...(more ? { cursor: nextCursor.value } : {}) } }))
    accountEntries.value = more ? [...accountEntries.value, ...result.items] : result.items
    nextCursor.value = result.nextCursor
  } catch (error) { accountMessage.value = accountError(error) }
  finally { accountLoading.value = false }
}

const updatedLabel = (date: string) => new Date(date).toLocaleString('ja-JP', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
onMounted(() => { void loadAccount() })
</script>

<template>
  <section class="draft-library">
    <header class="draft-library-heading">
      <div><h2>{{ heading }}</h2></div>
      <NuxtLink v-if="!testing" class="editor-button primary" to="/problems/new?fresh=1">新規問題</NuxtLink>
    </header>
    <p v-if="accountMessage" role="alert" class="editor-error">{{ accountMessage }}</p>
    <p v-if="accountLoading" role="status">問題を読み込んでいます…</p>
    <p v-else-if="!user"><NuxtLink to="/login?next=/my%3Ftab%3Dproblems">ログイン</NuxtLink>すると、問題を表示できます。</p>
    <template v-else>
      <p v-if="!accountEntries.length && !accountMessage">{{ testing ? 'テスト中の問題はまだありません。テスターリンクから参加すると、ここに表示されます。' : '保存した問題はまだありません。' }}</p>
      <div v-if="accountEntries.length" class="content-table-scroll">
        <table class="content-table" :aria-label="heading">
          <thead><tr><th scope="col">タイトル</th><th scope="col">状態</th><th scope="col">更新日時</th><th scope="col">操作</th></tr></thead>
          <tbody><tr v-for="entry in accountEntries" :key="entry.id">
            <th scope="row">{{ entry.title.trim() || '無題の問題' }}</th>
            <td>
              <span class="problem-state">
                <ProblemStatus :problem="entry" />
                <span v-if="!entry.publishedVersion && !entry.contestScheduled && entry.featuredPreference" class="featured-preference" tabindex="0" role="img" :aria-label="preferenceLabel(entry.featuredPreference)" :aria-describedby="`${tooltipId}-${entry.id}`">
                  <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                    <template v-if="entry.featuredPreference === 'soon'">
                      <path d="M15 10c-2-3-2-7 0-8 2 0 2 4 2 7M18 9c0-3 1-6 3-6 2 1 0 5-1 7M14 10c-2-1-5 0-7 3-2 2-2 5-1 7h13c2 0 2-2 0-2h-2l1-3h2c3 0 3-4 0-5-2-1-4-1-6 0Z" />
                      <path d="M9 15c3-1 5 3 2 5M5 14a2 2 0 1 0 0 4" /><circle cx="19" cy="12" r=".6" fill="currentColor" stroke="none" />
                    </template>
                    <template v-else>
                      <path d="M3 15c0-5 3-8 7-8s7 3 7 8H3ZM3 15l-2-1M6 16l-1 3h3l1-3m4 0 1 3h3l-1-3M17 12c1-3 5-3 6 0v3h-6M6 9l4 3 4-3m-4 3v3" />
                      <circle cx="20" cy="12" r=".6" fill="currentColor" stroke="none" />
                    </template>
                  </svg>
                  <span :id="`${tooltipId}-${entry.id}`" class="preference-tooltip" role="tooltip">{{ preferenceLabel(entry.featuredPreference) }}</span>
                </span>
              </span>
            </td>
            <td><time :datetime="entry.updatedAt">{{ updatedLabel(entry.updatedAt) }}</time></td>
            <td><div class="problem-actions"><ContentActions :title="entry.title.trim() || '無題の問題'" :edit-to="{ path: '/problems/new', query: { problem: entry.id } }" :view-to="`/problems/${entry.id}`" :published="!!entry.publishedVersion" can-view />
              <button v-if="!testing && entry.featuredPreference" class="editor-button withdraw-featured" :disabled="!!withdrawing" :aria-label="`${entry.title.trim() || '無題の問題'}の定期便応募を取り下げる`" title="定期便応募を取り下げる" @click="confirmWithdraw(entry)">
                <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m9 5-5 5 5 5M4 10h10a6 6 0 0 1 0 12" transform="translate(0 -2)" /></svg>
              </button>
            </div></td>
          </tr></tbody>
        </table>
      </div>
      <button v-if="nextCursor" class="editor-button" :disabled="accountLoading" @click="loadAccount(true)">さらに読み込む</button>
    </template>
    <button v-if="accountMessage" class="editor-button" :disabled="accountLoading" @click="loadAccount()">再試行</button>
    <Teleport to="body">
      <dialog ref="withdrawDialog" class="leave-dialog" :aria-labelledby="`${tooltipId}-withdraw-title`" :aria-describedby="`${tooltipId}-withdraw-description`" @cancel="withdrawing && $event.preventDefault()" @close="withdrawEntry = undefined">
        <h2 :id="`${tooltipId}-withdraw-title`">定期便の応募を取り下げますか？</h2>
        <p class="manage-problem-title">{{ withdrawEntry?.title.trim() || '無題の問題' }}</p>
        <p :id="`${tooltipId}-withdraw-description`">取り下げると、定期便の出題待ちから外れます。問題は未公開のまま保存されます。</p>
        <p v-if="withdrawMessage" role="alert" class="editor-error">{{ withdrawMessage }}</p>
        <div class="leave-dialog-actions">
          <button type="button" class="editor-button" autofocus :disabled="!!withdrawing" @click="withdrawDialog?.close()">キャンセル</button>
          <button type="button" class="editor-button danger" :disabled="!!withdrawing" :aria-busy="!!withdrawing" @click="withdrawEntry && withdraw(withdrawEntry)">{{ withdrawing ? '取り下げ中…' : '取り下げる' }}</button>
        </div>
      </dialog>
    </Teleport>
  </section>
</template>

<style scoped>
.problem-state { display: inline-flex; flex-wrap: wrap; align-items: flex-start; gap: 6px; }
.featured-preference { position: relative; display: inline-flex; align-items: center; justify-content: center; width: 32px; height: 32px; color: var(--color-muted); cursor: help; }
.featured-preference:focus-visible { outline: 2px solid var(--color-accent); outline-offset: 2px; border-radius: 4px; }
.preference-tooltip { display: none; position: absolute; bottom: 100%; left: 50%; transform: translateX(-50%); padding: 4px 8px; border-radius: 4px; background: var(--color-ink); color: var(--color-paper); font-size: .75rem; font-weight: 400; z-index: 1; }
.featured-preference:hover .preference-tooltip, .featured-preference:focus .preference-tooltip { display: block; }
.problem-actions { display: flex; align-items: center; gap: 8px; }
.withdraw-featured { display: inline-flex; align-items: center; justify-content: center; width: 40px; height: 40px; padding: 0; }
</style>
