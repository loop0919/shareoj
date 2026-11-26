<script setup lang="ts">
import { accountListSchema, type AccountSummary } from '~~/shared/types/account-problems'
import { accountError } from '~/utils/account-problems'
import { problemStatusGroup, problemStatusOptions } from '~/utils/library-entries'
import { japanMinute } from '~/utils/japan-time'
const props = defineProps<{ testing?: boolean }>()
const heading = computed(() => props.testing ? 'テスト中の問題' : '自分の問題')
const { user, refreshAccount } = useAccount()
const accountEntries = ref<AccountSummary[]>([])
const accountLoading = ref(true)
const accountMessage = ref('')
const nextCursor = ref('')
let listedOwner = ''
async function loadAccount(more = false) {
  accountLoading.value = true
  accountMessage.value = ''
  try {
    const account = await refreshAccount()
    if (!account) { accountEntries.value = []; nextCursor.value = ''; await navigateTo('/login?next=/my%3Ftab%3Dproblems', { replace: true }); return }
    if (listedOwner !== account.id) { more = false; accountEntries.value = []; nextCursor.value = ''; listedOwner = account.id }
    const result = accountListSchema.parse(await $fetch('/api/my/problems', { query: { ...(props.testing ? { role: 'tester' } : {}), ...(more ? { cursor: nextCursor.value } : {}) } }))
    // Testers cannot publish or apply to 定期便, so the destinations in their list are not shown here.
    if (props.testing) for (const item of result.items) delete item.readiness
    accountEntries.value = more ? [...accountEntries.value, ...result.items] : result.items
    nextCursor.value = result.nextCursor
  } catch (error) { accountMessage.value = accountError(error) }
  finally { accountLoading.value = false }
}

const status = defineModel<string>('status', { default: '' })
const { filter, counts, visible: visibleEntries, label: filterLabel } = useStatusFilter(problemStatusOptions, status, () => accountEntries.value, problemStatusGroup)
// A filter needs every page, or a matching problem past the first 50 would look absent.
watch([filter, nextCursor, accountLoading], () => {
  if (filter.value && nextCursor.value && !accountLoading.value && !accountMessage.value) void loadAccount(true)
})

onMounted(() => { void loadAccount() })
</script>

<template>
  <section class="draft-library">
    <header class="draft-library-heading">
      <div><h2>{{ heading }}</h2></div>
      <NuxtLink v-if="!testing" class="editor-button primary" to="/problems/new?fresh=1">新規問題</NuxtLink>
    </header>
    <p v-if="accountMessage" role="alert" class="editor-error">{{ accountMessage }}</p>
    <LoadingSpinner v-if="accountLoading && !accountEntries.length" label="問題を読み込み中" />
    <p v-else-if="!user"><NuxtLink to="/login?next=/my%3Ftab%3Dproblems">ログイン</NuxtLink>すると、問題を表示できます。</p>
    <template v-else>
      <EmptyState v-if="!accountEntries.length && !accountMessage && testing" kind="testing" title="テスト中の問題はまだありません。" description="テスターリンクから参加すると、ここに表示されます。" />
      <EmptyState v-else-if="!accountEntries.length && !accountMessage" kind="problem" title="保存した問題はまだありません。" description="「新規問題」から作成すると、ここに表示されます。" />
      <StatusFilter v-if="accountEntries.length" v-model="filter" :options="problemStatusOptions" :counts="nextCursor ? undefined : counts" />
      <EmptyState v-if="accountEntries.length && !visibleEntries.length && !nextCursor && !accountLoading" :kind="testing ? 'testing' : 'problem'" :title="`「${filterLabel}」の問題はありません。`"><button type="button" class="editor-button" @click="filter = ''">すべて表示</button></EmptyState>
      <div v-if="visibleEntries.length" class="content-table-scroll">
        <table class="content-table content-table--cards" :aria-label="heading">
          <thead><tr><th scope="col">タイトル</th><th scope="col">状態</th><th scope="col">更新日時</th><th scope="col">難易度</th><th scope="col">操作</th></tr></thead>
          <tbody><tr v-for="entry in visibleEntries" :key="entry.id">
            <th scope="row">{{ entry.title.trim() || '無題の問題' }}<span v-if="entry.viaContest" class="via-contest">コンテスト「{{ entry.viaContest.title.trim() || '無題のコンテスト' }}」の作成者として</span></th>
            <td class="line-cell" data-label="状態">
              <SavedProblemState :entry="entry" />
            </td>
            <td class="corner-cell"><time :datetime="entry.updatedAt">{{ japanMinute(entry.updatedAt) }}</time></td>
            <td class="lead-cell"><DifficultyBadge :level="entry.difficulty" /></td>
            <td class="actions-cell"><SavedProblemActions :entry="entry" :testing="testing" /></td>
          </tr></tbody>
        </table>
      </div>
      <LoadingSpinner v-if="accountLoading" label="問題を読み込み中" />
      <button v-else-if="nextCursor" class="editor-button" @click="loadAccount(true)">さらに読み込む</button>
    </template>
    <button v-if="accountMessage" class="editor-button" :disabled="accountLoading" @click="loadAccount()">再試行</button>
  </section>
</template>

<style scoped>
/* Rights through a contest end with its results, unlike an invitation, so the row says where they come from. */
.via-contest { display: block; margin-top: 2px; color: var(--color-muted); font-size: .8125rem; font-weight: 400; }
</style>
