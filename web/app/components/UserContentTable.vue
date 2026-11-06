<script setup lang="ts">
import type { z } from 'zod'
import { shortId } from '~~/shared/utils/short-id'
import type { publicProblemSummarySchema } from '~~/shared/types/problem'
import type { postSummarySchema } from '~~/shared/types/post'
type Item = z.infer<typeof publicProblemSummarySchema> | z.infer<typeof postSummarySchema>
type Page = { items: Item[], nextCursor: string }
const props = defineProps<{ initial: Page, kind: 'problems' | 'posts', fetchPage: (cursor: string) => Promise<Page> }>()
const { current, index, loading, message, move } = useContentPages(props.initial, props.fetchPage)
const label = props.kind === 'problems' ? '問題' : '記事'
const date = (value: string) => new Date(value).toLocaleDateString('ja-JP', { timeZone: 'Asia/Tokyo' })
</script>
<template>
  <EmptyState v-if="!current.items.length" :kind="kind === 'problems' ? 'problem' : 'post'" :title="`公開された${label}はまだありません。`" />
  <div v-else class="content-table-scroll" role="region" :aria-label="`公開済みの${label}`" tabindex="0" :aria-busy="loading">
    <table class="content-table content-table--cards">
      <thead><tr>
        <!-- Problems lead with the date like the problem list; posts end with it like the post list. -->
        <th v-if="kind === 'problems'" scope="col">公開日</th>
        <th scope="col">タイトル</th><th scope="col">{{ kind === 'problems' ? '作成者' : '投稿者' }}</th>
        <template v-if="kind === 'problems'"><th scope="col"><abbr title="実行時間制限 / メモリ制限">TL / ML</abbr></th><th scope="col">正解者数</th><th scope="col">難易度</th></template>
        <th v-else scope="col">公開日</th>
      </tr></thead>
      <tbody><tr v-for="item in current.items" :key="item.id">
        <td v-if="kind === 'problems'" class="corner-cell"><time v-if="item.publishedAt" :datetime="item.publishedAt">{{ date(item.publishedAt) }}</time></td>
        <th scope="row"><NuxtLink :to="`/${kind === 'posts' ? 'blog' : 'problems'}/${shortId(item.id)}`">{{ item.title }}</NuxtLink></th>
        <td :class="kind === 'problems' ? 'line-cell' : 'inline-cell'" data-label="作成者"><UserLink :handle="item.author" /></td>
        <template v-if="kind === 'problems' && 'solverCount' in item">
          <td class="stat-cell wide-cell" data-label="TL / ML">{{ item.timeLimitMs == null ? '—' : `${item.timeLimitMs / 1000} 秒` }}・{{ item.memoryLimitMb == null ? '—' : `${item.memoryLimitMb} MiB` }}</td>
          <td class="stat-cell" data-label="正解者数">{{ item.solverCount }}</td><td class="lead-cell"><DifficultyBadge :level="item.difficulty" /></td>
        </template>
        <td v-else class="end-cell"><time v-if="item.publishedAt" :datetime="item.publishedAt">{{ date(item.publishedAt) }}</time></td>
      </tr></tbody>
    </table>
  </div>
  <p v-if="message" role="alert">{{ message }}</p>
  <ContentPagination v-if="current.items.length || index" :index="index" :has-next="!!current.nextCursor" :loading="loading" @move="move" />
</template>
