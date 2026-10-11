<script setup lang="ts">
import { contestDate, contestStatus, type ContestDraftList, type ContestList } from '~~/shared/types/contest'
const props = defineProps<{ mine?: boolean }>()
const endpoint = props.mine ? '/api/my/contests' : '/api/contests'
const offset = ref(0)
const { data, error, status } = await useFetch<ContestList>(endpoint, { query: { offset }, server: !props.mine })
// Drafts are private, so only the author's list asks for them.
const draftOffset = ref(0)
const { data: drafts, status: draftStatus } = await useFetch<ContestDraftList>('/api/my/contests/drafts', { query: { offset: draftOffset }, server: false, immediate: props.mine })

// 下書き and 公開中 are the two lists below, so a filter shows one of them and keeps its paging.
const statusOptions = [{ key: 'draft', label: '下書き' }, { key: 'published', label: '公開中' }] as const
const selected = defineModel<string>('status', { default: '' })
const { filter, counts, label: filterLabel } = useStatusFilter(statusOptions, selected, () => [...(drafts.value?.items ?? []).map(() => 'draft' as const), ...(data.value?.items ?? []).map(() => 'published' as const)], group => group)
// Counts are known only while each list fits on its first page.
const complete = computed(() => !offset.value && !draftOffset.value && !!data.value && !!drafts.value && !data.value.hasMore && !drafts.value.hasMore)
</script>
<template>
  <section :class="mine ? 'draft-library' : 'catalogue'">
    <header v-if="mine" class="draft-library-heading"><h2>作成したコンテスト</h2><NuxtLink class="editor-button primary" to="/my/contests/new">新規コンテスト</NuxtLink></header>
    <header v-else class="catalogue-heading"><h1>コンテスト</h1><NuxtLink class="editor-button primary" to="/my/contests/new">新規コンテスト</NuxtLink></header>
    <p v-if="error" role="alert">コンテストを取得できませんでした。ページを再読み込みしてください。</p>
    <LoadingSpinner v-else-if="!data" label="コンテストを読み込み中" />
    <template v-else>
      <StatusFilter v-if="mine && (data.items.length || offset || drafts?.items.length || draftOffset || filter)" v-model="filter" :options="statusOptions" :counts="complete ? counts : undefined" />
      <template v-if="filter === 'draft'">
        <LoadingSpinner v-if="!drafts" label="コンテストを読み込み中" />
        <EmptyState v-else-if="!drafts.items.length && !draftOffset" kind="contest" :title="`「${filterLabel}」のコンテストはありません。`"><button type="button" class="editor-button" @click="filter = ''">すべて表示</button></EmptyState>
      </template>
      <template v-if="mine && filter !== 'published' && drafts && (drafts.items.length || draftOffset)">
        <h3 v-if="!filter" class="contest-list-heading">下書き（未公開）</h3>
        <div class="content-table-scroll" role="region" aria-label="コンテストの下書き" tabindex="0" :aria-busy="draftStatus === 'pending'">
          <table class="content-table"><thead><tr><th scope="col">下書き</th><th scope="col">開始予定（日本時間）</th><th scope="col">更新（日本時間）</th><th scope="col">操作</th></tr></thead>
            <tbody><tr v-for="d in drafts.items" :key="d.id"><th scope="row"><NuxtLink :to="`/my/contests/${d.id}`">{{ d.title.trim() || '無題のコンテスト' }}</NuxtLink></th><td>{{ d.startsAt ? contestDate(d.startsAt) : '未設定' }}</td><td>{{ contestDate(d.updatedAt) }}</td><td><NuxtLink class="editor-button edit-link" :to="`/my/contests/${d.id}`" :aria-label="`${d.title.trim() || '無題のコンテスト'}を編集`" title="編集"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m16 3 5 5-12 12-6 1 1-6Z" /><path d="m13 6 5 5" /></svg></NuxtLink></td></tr></tbody>
          </table>
        </div>
        <ContentPagination v-if="drafts.hasMore || draftOffset" :index="draftOffset / 50" :has-next="drafts.hasMore" :loading="draftStatus === 'pending'" @move="direction => draftOffset += direction * 50" />
        <h3 v-if="!filter" class="contest-list-heading">公開したコンテスト</h3>
      </template>
      <template v-if="filter !== 'draft'">
        <EmptyState v-if="filter && !data.items.length" kind="contest" :title="`「${filterLabel}」のコンテストはありません。`"><button type="button" class="editor-button" @click="filter = ''">すべて表示</button></EmptyState>
        <EmptyState v-else-if="!data.items.length && drafts?.items.length" kind="contest" title="公開したコンテストはまだありません。" description="下書きを開いて「公開」すると、ここに表示されます。" />
        <EmptyState v-else-if="!data.items.length" kind="contest" title="コンテストはまだありません。" :description="mine ? '「新規コンテスト」から作成すると、ここに表示されます。' : ''" />
        <div v-else class="content-table-scroll" role="region" aria-label="コンテスト一覧" tabindex="0" :aria-busy="status === 'pending'">
          <table class="content-table"><thead><tr><th scope="col">コンテスト</th><th scope="col">状態</th><th scope="col">開始（日本時間）</th><th scope="col">終了（日本時間）</th><th scope="col">作成者</th><th v-if="mine" scope="col">操作</th></tr></thead>
            <tbody><tr v-for="c in data.items" :key="c.id"><th scope="row"><NuxtLink :to="`/contests/${c.id}`">{{ c.title }}</NuxtLink></th><td>{{ contestStatus[c.status] }}</td><td>{{ contestDate(c.startsAt) }}</td><td>{{ contestDate(c.endsAt) }}</td><td><UserLink :handle="c.author" /></td><td v-if="mine"><NuxtLink v-if="c.status === 'scheduled'" class="editor-button edit-link" :to="`/my/contests/${c.id}`" :aria-label="`${c.title}を編集`" title="編集"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m16 3 5 5-12 12-6 1 1-6Z" /><path d="m13 6 5 5" /></svg></NuxtLink><span v-else class="muted">—</span></td></tr></tbody>
          </table>
        </div>
        <ContentPagination v-if="data.items.length || offset" :index="offset / 50" :has-next="data.hasMore" :loading="status === 'pending'" @move="direction => offset += direction * 50" />
      </template>
    </template>
  </section>
</template>
<style scoped>
.contest-list-heading { font-size: 1rem; margin: 0 0 12px; }
.content-table-scroll ~ .contest-list-heading { margin-top: 32px; }
.edit-link { display: inline-flex; align-items: center; justify-content: center; width: 40px; height: 40px; padding: 0; }
</style>
