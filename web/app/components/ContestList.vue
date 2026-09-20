<script setup lang="ts">
import { contestDate, contestStatus, type ContestList } from '~~/shared/types/contest'
const props = defineProps<{ mine?: boolean }>()
const endpoint = props.mine ? '/api/my/contests' : '/api/contests'
const offset = ref(0)
const { data, error, status } = await useFetch<ContestList>(endpoint, { query: { offset }, server: !props.mine })
</script>
<template>
  <section :class="mine ? 'draft-library' : 'catalogue'">
    <header v-if="mine" class="draft-library-heading"><h2>作成したコンテスト</h2><NuxtLink class="editor-button primary" to="/my/contests/new">新規コンテスト</NuxtLink></header>
    <header v-else class="catalogue-heading"><h1>コンテスト</h1><NuxtLink class="editor-button primary" to="/my/contests/new">新規コンテスト</NuxtLink></header>
    <p v-if="error" role="alert">コンテストを取得できませんでした。ページを再読み込みしてください。</p>
    <p v-else-if="!data" role="status">読み込み中…</p>
    <template v-else>
      <p v-if="!data.items.length" class="muted">コンテストはまだありません。</p>
      <div v-else class="content-table-scroll" role="region" aria-label="コンテスト一覧" tabindex="0" :aria-busy="status === 'pending'">
        <table class="content-table"><thead><tr><th scope="col">コンテスト</th><th scope="col">状態</th><th scope="col">開始（日本時間）</th><th scope="col">終了（日本時間）</th><th scope="col">作成者</th><th v-if="mine" scope="col">操作</th></tr></thead>
          <tbody><tr v-for="c in data.items" :key="c.id"><th scope="row"><NuxtLink :to="`/contests/${c.id}`">{{ c.title }}</NuxtLink></th><td>{{ contestStatus[c.status] }}</td><td>{{ contestDate(c.startsAt) }}</td><td>{{ contestDate(c.endsAt) }}</td><td><UserLink :handle="c.author" /></td><td v-if="mine"><NuxtLink v-if="c.status === 'draft' || c.status === 'scheduled'" class="editor-button" :to="`/my/contests/${c.id}`">編集</NuxtLink><span v-else class="muted">—</span></td></tr></tbody>
        </table>
      </div>
      <ContentPagination :index="offset / 50" :has-next="data.hasMore" :loading="status === 'pending'" @move="direction => offset += direction * 50" />
    </template>
  </section>
</template>
