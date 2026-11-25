<script setup lang="ts">
import { vDialogDismiss } from '~/utils/dialog-dismiss'
import { accountProblemSchema, type AccountSummary } from '~~/shared/types/account-problems'
import { shortId } from '~~/shared/utils/short-id'
import { accountError } from '~/utils/account-problems'
const props = defineProps<{ entry: AccountSummary, testing?: boolean }>()
const tooltipId = useId()
const withdrawing = ref('')
// The dialog exists only while it is open, so a long list does not put one per row into <body>.
const withdrawOpen = ref(false)
const withdrawDialog = ref<HTMLDialogElement>()
const withdrawMessage = ref('')
async function confirmWithdraw() {
  if (withdrawing.value || props.testing) return
  withdrawMessage.value = ''
  withdrawOpen.value = true
  await nextTick()
  withdrawDialog.value?.showModal()
}
async function withdraw() {
  if (withdrawing.value || props.testing) return
  const entry = props.entry
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
</script>

<template>
  <div class="problem-actions"><ContentActions :title="entry.title.trim() || '無題の問題'" :edit-to="{ path: '/problems/new', query: { problem: entry.id } }" :view-to="`/problems/${shortId(entry.id)}`" :published="!!entry.publishedVersion" can-view />
    <NuxtLink v-if="!testing && entry.scheduledAt" class="editor-button" :to="{ path: '/problems/post', query: { problem: entry.id } }">予約を変更</NuxtLink>
    <button v-if="!testing && entry.featuredPreference" class="editor-button withdraw-featured" :disabled="!!withdrawing" :aria-label="`${entry.title.trim() || '無題の問題'}の定期便応募を取り下げる`" title="定期便応募を取り下げる" @click="confirmWithdraw()">
      <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m9 5-5 5 5 5M4 10h10a6 6 0 0 1 0 12" transform="translate(0 -2)" /></svg>
    </button>
  </div>
  <Teleport v-if="withdrawOpen" to="body">
    <dialog v-dialog-dismiss ref="withdrawDialog" class="leave-dialog" :aria-labelledby="`${tooltipId}-withdraw-title`" :aria-describedby="`${tooltipId}-withdraw-description`" @cancel="withdrawing && $event.preventDefault()" @close="withdrawOpen = false">
      <h2 :id="`${tooltipId}-withdraw-title`">定期便の応募を取り下げますか？</h2>
      <p class="manage-problem-title">{{ entry.title.trim() || '無題の問題' }}</p>
      <p :id="`${tooltipId}-withdraw-description`">取り下げると、定期便の出題待ちから外れます。問題は未公開のまま保存されます。</p>
      <p v-if="withdrawMessage" role="alert" class="editor-error">{{ withdrawMessage }}</p>
      <div class="leave-dialog-actions">
        <button type="button" class="editor-button" autofocus :disabled="!!withdrawing" @click="withdrawDialog?.close()">キャンセル</button>
        <button type="button" class="editor-button danger" :disabled="!!withdrawing" :aria-busy="!!withdrawing" @click="withdraw()">{{ withdrawing ? '取り下げ中…' : '取り下げる' }}</button>
      </div>
    </dialog>
  </Teleport>
</template>

<style scoped>
.problem-actions { display: flex; align-items: center; gap: 8px; }
.withdraw-featured { display: inline-flex; align-items: center; justify-content: center; width: 40px; height: 40px; padding: 0; }
</style>
