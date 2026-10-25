<script setup lang="ts">
import { docPath, footerDocs } from '~~/shared/docs'
import { fullId } from '~~/shared/utils/short-id'
const route = useRoute()
const pageError = useError()
// Contest pages swap the site menu for the contest's own menu. Error pages keep the site menu.
const contestId = computed(() => !pageError.value && String(route.name ?? '').startsWith('contests-id') ? fullId(route.params.id) : '')
const { data: judgeCatalog, error: judgeCatalogError, refresh: refreshJudge } = await useJudgeCatalog()
function refreshVisibleJudge() { if (document.visibilityState === 'visible') void refreshJudge({ dedupe: 'defer' }) }
// A refresh during hydration reuses Nuxt's server payload.
onNuxtReady(refreshVisibleJudge)
usePolling(() => refreshJudge({ dedupe: 'defer' }), 30_000)
onMounted(() => {
  document.addEventListener('visibilitychange', refreshVisibleJudge)
})
onBeforeUnmount(() => {
  document.removeEventListener('visibilitychange', refreshVisibleJudge)
})
watch(() => route.fullPath, () => { void refreshJudge({ dedupe: 'defer' }) })
const createMenu = ref<HTMLDetailsElement>()
function closeCreateMenu() { if (createMenu.value) createMenu.value.open = false }
function onOutsideClick(event: MouseEvent) {
  if (event.target instanceof Node && !createMenu.value?.contains(event.target)) closeCreateMenu()
}
onMounted(() => document.addEventListener('click', onOutsideClick))
onBeforeUnmount(() => document.removeEventListener('click', onOutsideClick))
watch(() => route.fullPath, closeCreateMenu)
const { user, refreshAccount, refreshProfile } = useAccount()
onMounted(() => { void refreshAccount().then(account => { if (account) return refreshProfile() }).catch(() => {}) })
useEventAlarms()
useNotificationPolling()
</script>

<template>
  <div class="site-shell" :class="{ 'site-shell--editor': route.meta.editorLayout, 'site-shell--contest': contestId }">
    <a class="skip-link" href="#main">本文へ移動</a>
    <div v-if="judgeCatalog?.maintenance || judgeCatalogError" class="judge-banner" role="status">
      {{ judgeCatalog?.maintenance ? judgeMaintenanceMessage : 'ジャッジ機能の状態を確認できません。現在、提出等を利用できません。' }}
    </div>
    <header v-if="!route.meta.editorLayout" class="site-header" :class="{ 'site-header--contest': contestId }">
      <NuxtLink class="wordmark" to="/" aria-label="ShareOJ ホーム">Share<span>OJ</span><span class="wordmark-beta">(β)</span></NuxtLink>
      <ContestHeaderTitle v-if="contestId" :id="contestId" class="contest-title" />
      <ContestMenu v-if="contestId" :id="contestId" />
      <nav v-else aria-label="メインナビゲーション">
        <div class="nav-links">
          <NuxtLink to="/">ホーム</NuxtLink>
          <NuxtLink to="/problems">問題</NuxtLink>
          <NuxtLink to="/featured">定期便</NuxtLink>
          <NuxtLink to="/contests">コンテスト</NuxtLink>
          <NuxtLink to="/blog">記事</NuxtLink>
          <NuxtLink to="/run">コード実行</NuxtLink>
        </div>
        <details ref="createMenu" class="create-menu" @keydown.esc.prevent="closeCreateMenu(); createMenu?.querySelector('summary')?.focus()">
          <summary>作成</summary>
          <div class="create-menu-links" @click="closeCreateMenu">
            <NuxtLink to="/problems/new?fresh=1"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m7 6-6 6 6 6m10-12 6 6-6 6M14 4l-4 16" /></svg>新規問題</NuxtLink>
            <NuxtLink to="/my/contests/new"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M8 3h8v6a4 4 0 0 1-8 0V3Zm0 2H4v2a4 4 0 0 0 4 4m8-6h4v2a4 4 0 0 1-4 4m-4 2v5m-4 3h8m-6-3h4v3" /></svg>新規コンテスト</NuxtLink>
            <NuxtLink to="/blog/new"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m16 3 5 5-12 12-6 1 1-6Z" /><path d="m13 6 5 5" /></svg>新規記事</NuxtLink>
          </div>
        </details>
      </nav>
      <div class="header-tools">
        <ThemeSelect />
        <NotificationBell v-if="user" />
        <AccountMenu v-if="user" />
        <NuxtLink v-else to="/login">ログイン</NuxtLink>
      </div>
    </header>
    <PolicyNotice v-if="!route.meta.editorLayout" />
    <main id="main" tabindex="-1"><slot /></main>
    <footer v-if="!route.meta.editorLayout" class="site-footer">
      <div class="site-footer-brand">
        <span class="wordmark">ShareOJ<span class="wordmark-beta">(β)</span></span>
        <span class="site-footer-tagline">Share Online Judge</span>
      </div>
      <nav class="site-footer-group" aria-labelledby="site-footer-guides">
        <p id="site-footer-guides" class="site-footer-heading">ガイド</p>
        <ul>
          <li><NuxtLink to="/docs">ガイドのトップ</NuxtLink></li>
          <li v-for="page in footerDocs" :key="page.slug"><NuxtLink :to="docPath(page.slug)">{{ page.label }}</NuxtLink></li>
        </ul>
      </nav>
      <nav class="site-footer-group" aria-labelledby="site-footer-policies">
        <p id="site-footer-policies" class="site-footer-heading">規約</p>
        <ul>
          <li><NuxtLink to="/terms">利用規約</NuxtLink></li>
          <li><NuxtLink to="/privacy">プライバシーポリシー</NuxtLink></li>
        </ul>
      </nav>
      <nav class="site-footer-group" aria-labelledby="site-footer-external">
        <p id="site-footer-external" class="site-footer-heading">外部リンク</p>
        <ul>
          <li><a href="https://x.com/share_oj" target="_blank" rel="noopener noreferrer">X（@share_oj） ↗</a></li>
          <li><a href="https://github.com/loop0919/shareoj" target="_blank" rel="noopener noreferrer">GitHub ↗</a></li>
        </ul>
      </nav>
    </footer>
    <ContestClock v-if="contestId" :id="contestId" />
  </div>
</template>

<style scoped>
/* Hallmark · component: two-row mobile header · existing green theme
 * pre-emit critique: P4 H5 E4 S5 R5 V4 */
/* Room below the footer so the fixed contest clock never covers the end of the page. */
.site-shell--contest { padding-bottom: 96px; }
.judge-banner { flex-shrink: 0; padding: 12px 16px; background: var(--color-surface); color: var(--color-ink); border-bottom: 2px solid var(--color-accent); font-size: .875rem; overflow-wrap: anywhere; }
.site-header { display: grid; grid-template-columns: auto minmax(0, 1fr) auto; }
.site-header nav { position: relative; width: auto; justify-content: flex-end; }
.header-tools { position: relative; display: flex; align-items: center; gap: 4px; }
.header-tools a { font-size: .875rem; text-decoration: none; }
@media (max-width: 59.999rem) {
  .site-header { grid-template-columns: minmax(0, 1fr) auto; gap: 4px 8px; }
  .header-tools { grid-column: 2; grid-row: 1; gap: 0; }
  .site-header nav { grid-column: 1 / -1; grid-row: 2; justify-content: space-between; gap: 0 8px; flex-wrap: nowrap; }
  /* Only the links scroll sideways: 「作成」 stays outside the scroll box, whose edge would cut off its absolute panel. */
  .nav-links { display: flex; flex: 1; justify-content: space-between; gap: 0 8px; min-width: 0; overflow-x: auto; scrollbar-width: none; }
  .nav-links::-webkit-scrollbar { display: none; }
  .nav-links a, .create-menu { flex-shrink: 0; }
}
@media (min-width: 60rem) { .nav-links { display: contents; } }
/* Contest pages: the contest title sits beside the wordmark and its menu becomes a tab row on the header line. */
.site-header--contest { row-gap: 4px; padding-bottom: 0; }
.site-header--contest .contest-title { grid-column: 2; grid-row: 1; justify-self: start; max-width: 100%; }
.site-header--contest .contest-title::before { content: ''; flex-shrink: 0; width: 1px; height: 24px; background: var(--color-line); }
.site-header--contest .header-tools { grid-column: 3; grid-row: 1; }
.site-header--contest .contest-menu { grid-column: 1 / -1; grid-row: 2; justify-content: flex-start; gap: 0 clamp(12px, 4vw, 28px); flex-wrap: nowrap; }
@media (max-width: 59.999rem) {
  .site-header--contest .contest-title { grid-column: 1 / -1; grid-row: 2; }
  .site-header--contest .contest-title::before { display: none; }
  .site-header--contest .header-tools { grid-column: 2; }
  .site-header--contest .contest-menu { grid-row: 3; }
}
/* The narrowest phones tighten the spacing; what still does not fit scrolls sideways in the menu row (ADR 0031). */
@media (max-width: 22.499rem) {
  .site-header--contest .contest-menu { gap: 0 8px; }
}
.create-menu { font-size: .875rem; }
.create-menu summary { display: flex; align-items: center; gap: 6px; min-height: 44px; cursor: pointer; list-style: none; color: var(--color-accent); }
.create-menu summary::-webkit-details-marker { display: none; }
.create-menu summary::after { content: '▾'; font-size: .75rem; }
.create-menu-links { position: absolute; right: 0; top: 100%; z-index: 20; display: grid; min-width: max-content; padding: 6px; background: var(--color-paper); border: 1px solid var(--color-line); border-radius: 6px; box-shadow: 0 4px 12px #0001; }
.create-menu-links a { gap: 8px; padding: 0 12px; }
.create-menu-links a:hover, .create-menu-links a:focus-visible { background: var(--color-surface); }
</style>
