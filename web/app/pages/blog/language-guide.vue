<script setup lang="ts">
import content from '~/content/guides/language-guide.md?raw'
import { socialPages } from '~~/shared/social-pages'
const config = useRuntimeConfig()
const title = socialPages['/blog/language-guide']!.title
const description = 'ShareOJに提出できる言語のバージョン、コンパイル設定、追加ライブラリ、コードの書き方と実行制限を説明します。'
const canonical = new URL('/blog/language-guide', config.public.siteUrl).href
useSeoMeta({ title: `${title} | ShareOJ 記事`, description, ogTitle: title, ogDescription: description, ogType: 'article', ogUrl: canonical })
useHead({ link: [{ rel: 'canonical', href: canonical }] })
useSharePreview({ title, path: '/blog/language-guide', description })
</script>

<template>
  <article class="blog-article">
    <div class="breadcrumb-row"><nav class="breadcrumb" aria-label="パンくずリスト"><NuxtLink to="/blog">記事</NuxtLink><span aria-hidden="true">/</span><span>{{ title }}</span></nav><TweetButton :title="title" :url="canonical" /></div>
    <header class="blog-article-header">
      <p class="eyebrow">SHAREOJ GUIDE</p>
      <h1>{{ title }}</h1>
      <p class="lead">提出するコードに合わせて、言語と実行環境を選んでください。</p>
    </header>
    <nav class="blog-toc" aria-label="記事の目次"><a href="#submission">提出の基本</a><a href="#languages">言語とバージョン</a><a href="#libraries">追加ライブラリ</a><a href="#testlib">testlib形式の判定</a><a href="#limits">実行制限</a></nav>
    <ProblemMarkdown copyable :source="content" />
    <NuxtLink class="return-link" to="/blog">記事一覧へ →</NuxtLink>
  </article>
</template>
