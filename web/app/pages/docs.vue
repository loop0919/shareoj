<script setup lang="ts">
import { docPath, docSections } from '~~/shared/docs'
const route = useRoute()
// Narrow screens fold the guide list behind a button; it closes after each move.
const menuOpen = ref(false)
watch(() => route.path, () => { menuOpen.value = false })
</script>

<template>
  <div class="docs-layout">
    <nav class="docs-nav" aria-label="ガイドの一覧">
      <button type="button" class="docs-nav-toggle" aria-controls="docs-nav-list" :aria-expanded="menuOpen" @click="menuOpen = !menuOpen">ガイドの一覧</button>
      <div id="docs-nav-list" class="docs-nav-list" :class="{ 'docs-nav-list--open': menuOpen }">
        <NuxtLink to="/docs">ガイドのトップ</NuxtLink>
        <div v-for="section in docSections" :key="section.title">
          <p class="docs-nav-heading">{{ section.title }}</p>
          <ul>
            <li v-for="page in section.pages" :key="page.slug"><NuxtLink :to="docPath(page.slug)">{{ page.label }}</NuxtLink></li>
          </ul>
        </div>
      </div>
    </nav>
    <div class="docs-content"><NuxtPage /></div>
  </div>
</template>

<style scoped>
/* Hallmark · component: docs side rail · existing green theme (Plain · modern-minimal)
 * states: default · hover · focus (global ring) · active (global a) · current page · menu open/closed
 * pre-emit critique: P4 H4 E4 S4 R5 V4 */
.docs-layout { display: grid; grid-template-columns: minmax(0, 1fr); }
.docs-content { min-width: 0; }
.docs-nav { border-bottom: 1px solid var(--color-line); }
.docs-nav-toggle { display: flex; align-items: center; justify-content: space-between; width: 100%; min-height: 48px; padding: 0; border: 0; background: none; color: var(--color-ink); font: inherit; font-size: .875rem; font-weight: 600; cursor: pointer; }
.docs-nav-toggle::after { content: '▾'; color: var(--color-muted); font-size: .75rem; }
.docs-nav-toggle[aria-expanded="true"]::after { content: '▴'; }
.docs-nav-toggle:hover { color: var(--color-accent); }
.docs-nav-toggle:focus-visible { outline: 3px solid var(--color-accent); outline-offset: 2px; }
.docs-nav-list { display: none; padding-bottom: 16px; }
.docs-nav-list--open { display: block; }
.docs-nav-heading { margin: 16px 0 4px; color: var(--color-muted); font-size: .75rem; font-weight: 600; }
.docs-nav ul { margin: 0; padding: 0; list-style: none; }
.docs-nav a { display: flex; align-items: center; min-height: 44px; padding-inline: 12px; border-inline-start: 2px solid transparent; color: var(--color-ink); font-size: .875rem; text-decoration: none; white-space: nowrap; }
.docs-nav a:hover { color: var(--color-accent); background: var(--color-surface); }
.docs-nav a[aria-current="page"] { border-inline-start-color: var(--color-accent); background: var(--color-accent-soft); color: var(--color-accent); font-weight: 600; }
/* Wide screens keep the list open beside the page and pin it while the page scrolls. */
@media (min-width: 60rem) {
  .docs-layout { grid-template-columns: 13rem minmax(0, 1fr); column-gap: 48px; }
  .docs-nav { border-bottom: 0; }
  .docs-nav-toggle { display: none; }
  .docs-nav-list { display: block; position: sticky; top: 0; max-height: 100svh; overflow-y: auto; padding-block: 24px 48px; }
  .docs-nav a { min-height: 36px; }
}
@media (min-width: 60rem) and (pointer: coarse) { .docs-nav a { min-height: 44px; } }
</style>
