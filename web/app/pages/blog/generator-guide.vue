<script setup lang="ts">
import content from '~/content/guides/generator-guide.md?raw'
import { socialPages } from '~~/shared/social-pages'
const config = useRuntimeConfig()
const title = socialPages['/blog/generator-guide']!.title
const description = 'コードによる入力と期待出力の生成、入力の制約を検証する手順をC++17の例で紹介します。'
const canonical = new URL('/blog/generator-guide', config.public.siteUrl).href
useSeoMeta({ title: `${title} | ShareOJ 記事`, description, ogTitle: title, ogDescription: description, ogType: 'article', ogUrl: canonical })
useHead({ link: [{ rel: 'canonical', href: canonical }] })
useSharePreview({ title, path: '/blog/generator-guide', description })
</script>

<template>
  <article class="blog-article">
    <div class="breadcrumb-row"><nav class="breadcrumb" aria-label="パンくずリスト"><NuxtLink to="/blog">記事</NuxtLink><span aria-hidden="true">/</span><span>生成と検証ガイド</span></nav><TweetButton :title="title" :url="canonical" /></div>
    <header class="blog-article-header">
      <p class="eyebrow">SHAREOJ GUIDE</p>
      <h1>{{ title }}</h1>
      <p class="lead">テストケースをコードで作れます。ケース番号から入力を生成し、その入力を解答プログラムに渡して期待出力を揃えましょう。</p>
    </header>
    <nav class="blog-toc" aria-label="記事の目次"><a href="#input">入力を作る</a><a href="#output">期待出力を作る</a><a href="#validation">入力を検証する</a><a href="#steps">画面での手順</a><a href="#limits">上限と失敗時の動作</a><a href="#check">生成後の確認</a></nav>
    <ProblemMarkdown copyable :source="content" />
    <NuxtLink class="return-link" to="/problems/new">問題作成画面へ →</NuxtLink>
  </article>
</template>
