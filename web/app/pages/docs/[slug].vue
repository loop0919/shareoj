<script setup lang="ts">
import { docPages, docPath, docSection, findDoc } from '~~/shared/docs'
import { sourceFence, splitDocExamples } from '~/utils/doc-content'
const route = useRoute()
const page = findDoc(String(route.params.slug))
if (!page) throw createError({ statusCode: 404, statusMessage: 'ガイドが見つかりません', fatal: true })
const config = useRuntimeConfig()
const path = docPath(page.slug)
const canonical = new URL(path, config.public.siteUrl).href
useSeoMeta({ title: `${page.title} | ShareOJ ガイド`, description: page.description })
useHead({ link: [{ rel: 'canonical', href: canonical }] })
useSharePreview({ title: page.title, path, description: page.description })

const position = docPages.indexOf(page)
const previous = docPages[position - 1]
const next = docPages[position + 1]
const sources = import.meta.glob<string>('../../content/docs/*.md', { query: '?raw', import: 'default' })
const source = await sources[`../../content/docs/${page.slug}.md`]!()
const blocks = splitDocExamples(source)

// Links between guides in the Markdown body move within the app instead of reloading the page.
function followLink(event: MouseEvent) {
  if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return
  const link = (event.target as Element).closest('.markdown-body a')
  const href = link?.getAttribute('href')
  if (!href?.startsWith('/') || href.startsWith('//')) return
  event.preventDefault()
  void navigateTo(href)
}
</script>

<template>
  <article class="blog-article" @click="followLink">
    <div class="breadcrumb-row"><nav class="breadcrumb" aria-label="パンくずリスト"><NuxtLink to="/docs">ガイド</NuxtLink><span aria-hidden="true">/</span><span>{{ docSection(page.slug).title }}</span></nav><TweetButton :title="page.title" :url="canonical" /></div>
    <header class="blog-article-header">
      <p class="eyebrow">SHAREOJ GUIDE</p>
      <h1>{{ page.title }}</h1>
      <p class="lead">{{ page.lead }}</p>
    </header>
    <template v-for="(block, index) in blocks" :key="index">
      <ProblemMarkdown v-if="block.kind === 'markdown'" :source="block.source" />
      <template v-else>
        <ProblemMarkdown :source="sourceFence(block.source)" />
        <div class="guide-result"><p class="guide-result-label">表示例</p><ProblemMarkdown :source="block.source" /></div>
      </template>
    </template>
    <nav class="docs-pager" aria-label="前後のガイド">
      <NuxtLink v-if="previous" class="docs-pager-previous" :to="docPath(previous.slug)"><span>前のガイド</span>{{ previous.label }}</NuxtLink>
      <NuxtLink v-if="next" class="docs-pager-next" :to="docPath(next.slug)"><span>次のガイド</span>{{ next.label }}</NuxtLink>
    </nav>
  </article>
</template>

<style scoped>
.docs-pager { display: grid; grid-template-columns: minmax(0, 1fr); gap: 12px; margin-top: 56px; padding-top: 24px; border-top: 1px solid var(--color-line); }
.docs-pager a { display: grid; gap: 2px; padding: 12px 16px; border: 1px solid var(--color-line); border-radius: 4px; font-weight: 600; text-decoration: none; white-space: nowrap; }
.docs-pager a:hover { border-color: var(--color-accent); background: var(--color-accent-soft); }
.docs-pager span { color: var(--color-muted); font-size: .75rem; font-weight: 400; }
/* The body follows the title directly; its first heading needs no extra gap. */
.blog-article-header + :deep(.markdown-body > :first-child) { margin-top: 0; }
/* Screenshots in the guides need an edge against the page background. */
.blog-article :deep(.markdown-body img) { display: block; border: 1px solid var(--color-line); border-radius: 4px; }
.docs-pager-next { text-align: end; }
@media (min-width: 30rem) {
  .docs-pager { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .docs-pager-next { grid-column: 2; }
}
</style>
