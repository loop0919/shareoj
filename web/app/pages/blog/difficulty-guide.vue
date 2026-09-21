<script setup lang="ts">
import content from '~/content/guides/difficulty-guide.md?raw'
import { socialPages } from '~~/shared/social-pages'
const config = useRuntimeConfig()
const title = socialPages['/blog/difficulty-guide']!.title
const description = 'Lv.1〜10の難易度を、AtCoder Beginner ContestとAtCoder Problemsのdiffを目安に説明します。'
const canonical = new URL('/blog/difficulty-guide', config.public.siteUrl).href
useSeoMeta({ title: `${title} | ShareOJ 記事`, description, ogTitle: title, ogDescription: description, ogType: 'article', ogUrl: canonical })
useHead({ link: [{ rel: 'canonical', href: canonical }] })
useSharePreview({ title, path: '/blog/difficulty-guide', description })
</script>

<template>
  <article class="blog-article">
    <div class="breadcrumb-row"><nav class="breadcrumb" aria-label="パンくずリスト"><NuxtLink to="/blog">記事</NuxtLink><span aria-hidden="true">/</span><span>難易度の目安</span></nav><TweetButton :title="title" :url="canonical" /></div>
    <header class="blog-article-header">
      <p class="eyebrow">SHAREOJ GUIDE</p>
      <h1>{{ title }}</h1>
      <p class="lead">問題選びや難易度評価に使える、Lv.1〜10の目安です。</p>
    </header>
    <ProblemMarkdown :source="content" />
    <NuxtLink class="return-link" to="/problems">問題一覧へ →</NuxtLink>
  </article>
</template>
