<script setup lang="ts">
import type { z } from 'zod'
import type { postListSchema } from '~~/shared/types/post'
// Each tag gets a fresh list and pagination.
definePageMeta({ key: route => `posts:${typeof route.query.tag === 'string' ? route.query.tag : ''}` })
const route = useRoute()
const config = useRuntimeConfig()
const tag = typeof route.query.tag === 'string' ? route.query.tag : ''
const path = tag ? `/blog?${new URLSearchParams({ tag })}` : '/blog'
const canonical = new URL(path, config.public.siteUrl).href
const { user } = useAccount()
const { data, error } = await useFetch('/api/posts', { query: tag ? { tag } : {} })
if (error.value?.statusCode === 400) throw createError({ statusCode: 404, statusMessage: 'タグが見つかりません', fatal: true })
if (error.value || !data.value) throw createError({ statusCode: 502, statusMessage: '記事を取得できませんでした', fatal: true })
const { current, index, loading, message, move } = useContentPages(data.value, cursor => $fetch<z.infer<typeof postListSchema>>('/api/posts', { query: tag ? { cursor, tag } : { cursor } }))
const date = (value: string) => new Date(value).toLocaleDateString('ja-JP', { timeZone: 'Asia/Tokyo' })
const heading = tag ? `タグ「${tag}」の記事` : '記事'
const description = tag ? `ShareOJ で「${tag}」のタグが付いた記事の一覧。` : 'ShareOJ の記事一覧。プログラミング問題の解き方や学んだことを共有する記事と、問題作成・コンテスト・実行環境の使い方を読めます。'
useSeoMeta({ title: `${heading} | ShareOJ`, description, ogDescription: description, ogTitle: `${heading} | ShareOJ`, ogUrl: canonical, ogType: 'website' })
useHead({ link: [{ rel: 'canonical', href: canonical }] })
useSharePreview({ type: 'website', title: heading, path, description })
</script>
<template>
  <div class="catalogue">
    <header class="catalogue-heading"><h1>記事</h1><NuxtLink v-if="user" class="editor-button primary" to="/blog/new">新規記事</NuxtLink></header>
    <TagFilter kind="post" :tag="tag" to="/blog" />
    <EmptyState v-if="!current.items.length" kind="post" :title="tag ? `「${tag}」のタグが付いた記事はまだありません。` : '公開された記事はまだありません。'" />
    <div v-else class="content-table-scroll" tabindex="0" role="region" aria-label="記事一覧" :aria-busy="loading">
      <table class="content-table">
        <thead><tr><th scope="col">タイトル</th><th scope="col">投稿者</th><th scope="col">公開日</th></tr></thead>
        <tbody><tr v-for="post in current.items" :key="post.id">
          <th scope="row"><NuxtLink :to="`/blog/${post.id}`">{{ post.title }}</NuxtLink><TagLinks v-if="post.tags.length" class="post-row-tags" :tags="post.tags" to="/blog" /></th>
          <td><UserLink :handle="post.author" /> <span v-if="post.isOperator" class="operator-label">運営</span></td>
          <td><time v-if="post.publishedAt" :datetime="post.publishedAt">{{ date(post.publishedAt) }}</time><span v-else>—</span></td>
        </tr></tbody>
      </table>
    </div>
    <p v-if="message" role="alert">{{ message }}</p>
    <ContentPagination v-if="current.items.length || index" :index="index" :has-next="!!current.nextCursor" :loading="loading" @move="move" />
    <p><NuxtLink to="/blog/difficulty-guide">難易度の目安</NuxtLink></p>
    <p><NuxtLink to="/blog/contest-rules">コンテストのルール</NuxtLink></p>
    <p><NuxtLink to="/blog/language-guide">使える言語と実行環境の仕様</NuxtLink></p>
    <p><NuxtLink to="/blog/generator-guide">入出力生成と入力検証の使い方</NuxtLink></p>
    <p><NuxtLink to="/blog/markdown-guide">Markdown・数式の書き方</NuxtLink></p>
  </div>
</template>
<style scoped>
.post-row-tags { margin-top: 6px; }
.operator-label { color: var(--color-accent); border: 1px solid var(--color-line); padding: 2px 6px; border-radius: 4px; font-size: .75rem; }
</style>
