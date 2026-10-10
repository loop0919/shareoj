<script setup lang="ts">
import { contestDate, contestStatus, type ContestDraftList, type ContestList } from '~~/shared/types/contest'
const props = defineProps<{ mine?: boolean }>()
const endpoint = props.mine ? '/api/my/contests' : '/api/contests'
const offset = ref(0)
const { data, error, status } = await useFetch<ContestList>(endpoint, { query: { offset }, server: !props.mine })
// Drafts are private, so only the author's list asks for them.
const draftOffset = ref(0)
const { data: drafts, status: draftStatus } = await useFetch<ContestDraftList>('/api/my/contests/drafts', { query: { offset: draftOffset }, server: false, immediate: props.mine })
</script>
<template>
  <section :class="mine ? 'draft-library' : 'catalogue'">
    <header v-if="mine" class="draft-library-heading"><h2>作成したコンテスト</h2><NuxtLink class="editor-button primary" to="/my/contests/new">新規コンテスト</NuxtLink></header>
    <header v-else class="catalogue-heading"><h1>コンテスト</h1><NuxtLink class="editor-button primary" to="/my/contests/new">新規コンテスト</NuxtLink></header>
    <p v-if="error" role="alert">コンテストを取得できませんでした。ページを再読み込みしてください。</p>
    <p v-else-if="!data" role="status">読み込み中…</p>
    <template v-else>
      <template v-if="mine && drafts && (drafts.items.length || draftOffset)">
        <h3 class="contest-list-heading">下書き（未公開）</h3>
        <div class="content-table-scroll" role="region" aria-label="コンテストの下書き" tabindex="0" :aria-busy="draftStatus === 'pending'">
          <table class="content-table"><thead><tr><th scope="col">下書き</th><th scope="col">開始予定（日本時間）</th><th scope="col">更新（日本時間）</th><th scope="col">操作</th></tr></thead>
            <tbody><tr v-for="d in drafts.items" :key="d.id"><th scope="row"><NuxtLink :to="`/my/contests/${d.id}`">{{ d.title.trim() || '無題のコンテスト' }}</NuxtLink></th><td>{{ d.startsAt ? contestDate(d.startsAt) : '未設定' }}</td><td>{{ contestDate(d.updatedAt) }}</td><td><NuxtLink class="editor-button" :to="`/my/contests/${d.id}`">編集</NuxtLink></td></tr></tbody>
          </table>
        </div>
        <ContentPagination v-if="drafts.hasMore || draftOffset" :index="draftOffset / 50" :has-next="drafts.hasMore" :loading="draftStatus === 'pending'" @move="direction => draftOffset += direction * 50" />
        <h3 class="contest-list-heading">公開したコンテスト</h3>
      </template>
      <EmptyState v-if="!data.items.length && drafts?.items.length" kind="contest" title="公開したコンテストはまだありません。" description="下書きを開いて「公開」すると、ここに表示されます。" />
      <EmptyState v-else-if="!data.items.length" kind="contest" title="コンテストはまだありません。" :description="mine ? '「新規コンテスト」から作成すると、ここに表示されます。' : ''" />
      <div v-else class="content-table-scroll" role="region" aria-label="コンテスト一覧" tabindex="0" :aria-busy="status === 'pending'">
        <table class="content-table"><thead><tr><th scope="col">コンテスト</th><th scope="col">状態</th><th scope="col">開始（日本時間）</th><th scope="col">終了（日本時間）</th><th scope="col">作成者</th><th v-if="mine" scope="col">操作</th></tr></thead>
          <tbody><tr v-for="c in data.items" :key="c.id"><th scope="row"><NuxtLink :to="`/contests/${c.id}`">{{ c.title }}</NuxtLink></th><td>{{ contestStatus[c.status] }}</td><td>{{ contestDate(c.startsAt) }}</td><td>{{ contestDate(c.endsAt) }}</td><td><UserLink :handle="c.author" /></td><td v-if="mine"><NuxtLink v-if="c.status === 'scheduled'" class="editor-button" :to="`/my/contests/${c.id}`">編集</NuxtLink><span v-else class="muted">—</span></td></tr></tbody>
        </table>
      </div>
      <ContentPagination v-if="data.items.length || offset" :index="offset / 50" :has-next="data.hasMore" :loading="status === 'pending'" @move="direction => offset += direction * 50" />
    </template>
  </section>
</template>
<style scoped>
.contest-list-heading { font-size: 1rem; margin: 0 0 12px; }
.content-table-scroll ~ .contest-list-heading { margin-top: 32px; }
</style>
