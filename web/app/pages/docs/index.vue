<script setup lang="ts">
import { docPath, docSections } from '~~/shared/docs'
const config = useRuntimeConfig()
const description = 'ShareOJ の使い方のガイド。提出と判定結果、コンテストのルールと開き方、問題の作り方、定期便、作問CLIと公開APIを目的別にまとめています。'
const canonical = new URL('/docs', config.public.siteUrl).href
useSeoMeta({ title: 'ガイド | ShareOJ', description })
useHead({ link: [{ rel: 'canonical', href: canonical }] })
useSharePreview({ type: 'website', title: 'ガイド', path: '/docs', description })
</script>

<template>
  <div class="docs-index">
    <header>
      <h1>ガイド</h1>
      <p class="lead">ShareOJ の使い方を、やりたいことごとにまとめています。</p>
    </header>
    <section v-for="section in docSections" :key="section.title">
      <h2>{{ section.title }}</h2>
      <ul>
        <li v-for="page in section.pages" :key="page.slug">
          <NuxtLink :to="docPath(page.slug)">{{ page.title }}</NuxtLink>
          <p>{{ page.description }}</p>
        </li>
      </ul>
    </section>
  </div>
</template>

<style scoped>
.docs-index { padding-block: 24px 64px; }
.docs-index h1 { font-size: 1.75rem; margin: 0 0 12px; }
.docs-index section { margin-top: 40px; }
.docs-index h2 { margin-bottom: 4px; font-size: 1rem; color: var(--color-muted); }
.docs-index ul { margin: 0; padding: 0; list-style: none; }
.docs-index li { padding-block: 16px; border-bottom: 1px solid var(--color-line); }
.docs-index li a { display: inline-flex; align-items: center; min-height: 32px; font-weight: 600; }
.docs-index li p { margin: 0; color: var(--color-muted); font-size: .875rem; }
@media (pointer: coarse) { .docs-index li a { min-height: 44px; } }
</style>
