<script setup lang="ts">
definePageMeta({ key: route => String(route.params.id) })
const route = useRoute()
const config = useRuntimeConfig()
const { data: post, error } = await useFetch(() => `/api/posts/${encodeURIComponent(String(route.params.id))}`)
if (error.value || !post.value) throw createError({ statusCode: error.value?.statusCode === 404 ? 404 : 502, statusMessage: error.value?.statusCode === 404 ? '記事が見つかりません' : '記事を取得できませんでした', fatal: true })
const canonical = computed(() => new URL(`/blog/${post.value!.id}`, config.public.siteUrl).href)
useSeoMeta({ title: () => `${post.value?.title} | ShareOJ`, description: () => post.value?.markdown.slice(0, 160), ogTitle: () => `${post.value?.title} | ShareOJ`, ogUrl: () => canonical.value, ogType: 'article' })
useHead(() => ({ link: [{ rel: 'canonical', href: canonical.value }] }))
useSharePreview({ title: () => post.value!.title, path: () => `/blog/${post.value!.id}`, description: () => post.value!.markdown.slice(0, 160) })
</script>
<template>
  <article v-if="post" class="post-page">
    <div class="breadcrumb-row"><nav class="breadcrumb" aria-label="パンくずリスト"><NuxtLink to="/blog">記事</NuxtLink><span aria-hidden="true">/</span><span>{{ post.title }}</span></nav><TweetButton :title="post.title" :url="canonical" /></div>
    <h1>{{ post.title }}</h1>
    <p class="muted"><UserLink :handle="post.author" /> <span v-if="post.isOperator">・運営</span> · <time :datetime="post.publishedAt">{{ new Date(post.publishedAt).toLocaleDateString('ja-JP', { timeZone: 'Asia/Tokyo' }) }}</time></p>
    <ProblemMarkdown copyable :source="post.markdown" />
  </article>
</template>
<style scoped>
.post-page { margin: 40px 0 80px; overflow-wrap: anywhere; }
</style>
