<script setup lang="ts">
import { canViewAllSubmissions, contestView, contestViews, ownSubmissionsLabel, type ContestView } from '~~/shared/types/contest'
import { shortId } from '~~/shared/utils/short-id'
// The contest's menu in the site header. Both submission lists share one place, as on AtCoder: 「提出」 opens them once
// the whole list may be read, and until then the own list stands there alone (ADR 0030).
const props = defineProps<{ id: string }>()
const route = useRoute()
const { data: contest } = useContest(() => props.id)
// The team page belongs to the overview, where participation starts.
const section = computed<ContestView>(() => {
  const name = String(route.name)
  if (name === 'contests-id') return contestView(route.query.view)
  if (name === 'contests-id-team') return 'overview'
  if (name.startsWith('contests-id-submissions')) return 'submissions'
  if (name.startsWith('contests-id-my-submissions')) return 'my-submissions'
  // A problem's editorial is opened from 解説 (ADR 0034).
  if (name === 'contests-id-label' && route.query.view === 'editorial') return 'editorials'
  return 'problems'
})
const inSubmissions = computed(() => section.value === 'submissions' || section.value === 'my-submissions')
const canViewAll = computed(() => !!contest.value && canViewAllSubmissions(contest.value))
const own = computed(() => contest.value ? ownSubmissionsLabel(contest.value) : contestViews['my-submissions'])
const link = (view: ContestView) => ({ path: `/contests/${shortId(props.id)}`, query: view === 'overview' ? {} : { view } })
// 'page' marks the view itself, and 'true' a page under it, such as a submission.
const current = (view: ContestView) => section.value === view ? route.name === 'contests-id' ? 'page' : 'true' : undefined
const submissionsMenu = ref<HTMLDetailsElement>()
function close() { if (submissionsMenu.value) submissionsMenu.value.open = false }
function outside(event: MouseEvent) { if (event.target instanceof Node && !submissionsMenu.value?.contains(event.target)) close() }
// The menu row scrolls sideways on narrow screens and clips what is absolutely positioned inside it, so the panel is
// fixed to the viewport at the summary's position instead, and closes whenever that position could go stale.
const panel = ref<HTMLElement>()
const panelStyle = ref<Record<string, string>>({})
async function place() {
  const summary = submissionsMenu.value?.querySelector('summary')
  if (!submissionsMenu.value?.open || !summary) return
  const rect = summary.getBoundingClientRect()
  panelStyle.value = { left: `${rect.left}px`, top: `${rect.bottom}px` }
  await nextTick()
  const width = panel.value?.offsetWidth ?? 0
  panelStyle.value = { left: `${Math.max(0, Math.min(rect.left, innerWidth - width - 8))}px`, top: `${rect.bottom}px` }
}
onMounted(() => {
  document.addEventListener('click', outside)
  window.addEventListener('scroll', close)
  window.addEventListener('resize', close)
})
onBeforeUnmount(() => {
  document.removeEventListener('click', outside)
  window.removeEventListener('scroll', close)
  window.removeEventListener('resize', close)
})
watch(() => route.fullPath, close)
</script>

<template>
  <nav class="contest-menu" aria-label="コンテストメニュー" @scroll.passive="close">
    <NuxtLink v-for="view in (['overview', 'problems', 'standings'] as const)" :key="view" :to="link(view)" :aria-current="current(view)">{{ contestViews[view] }}</NuxtLink>
    <details v-if="canViewAll" ref="submissionsMenu" class="submissions-menu" :data-current="inSubmissions || undefined" @toggle="place" @keydown.esc.prevent="close(); submissionsMenu?.querySelector('summary')?.focus()">
      <summary>提出</summary>
      <div ref="panel" class="submissions-menu-links" :style="panelStyle">
        <NuxtLink :to="link('my-submissions')" :aria-current="current('my-submissions')">{{ own }}</NuxtLink>
        <NuxtLink :to="link('submissions')" :aria-current="current('submissions')">{{ contestViews.submissions }}</NuxtLink>
      </div>
    </details>
    <!-- A link to the whole list still lands in this place, so it stays marked there too. -->
    <NuxtLink v-else :to="link('my-submissions')" :aria-current="current('my-submissions') ?? (inSubmissions ? 'true' : undefined)">{{ own }}</NuxtLink>
    <NuxtLink v-for="view in (['editorials', 'clarifications', 'run'] as const)" :key="view" :to="link(view)" :aria-current="current(view)">{{ contestViews[view] }}</NuxtLink>
  </nav>
</template>

<style scoped>
.contest-menu { margin-bottom: -1px; overflow-x: auto; scrollbar-width: none; }
.contest-menu::-webkit-scrollbar { display: none; }
/* The scroll box clips outlines, so the focus ring is drawn inside the item. */
.contest-menu > a:focus-visible, .submissions-menu summary:focus-visible { outline-offset: -3px; }
/* Items touch each other and their padding makes the space between labels, so all of it is clickable. */
.contest-menu > a, .submissions-menu summary { flex-shrink: 0; min-height: 44px; padding-inline: clamp(6px, 2vw, 14px); border-bottom: 2px solid transparent; color: var(--color-muted); }
.contest-menu > a:hover, .submissions-menu summary:hover { color: var(--color-accent); }
.contest-menu > a[aria-current], .submissions-menu[data-current] summary { border-bottom-color: var(--color-accent); color: var(--color-accent); font-weight: 600; }
.submissions-menu { position: relative; }
.submissions-menu summary { display: flex; align-items: center; gap: 6px; font-size: .875rem; white-space: nowrap; cursor: pointer; list-style: none; }
.submissions-menu summary::-webkit-details-marker { display: none; }
.submissions-menu summary::after { content: '▾'; font-size: .75rem; }
.submissions-menu-links { position: fixed; z-index: 20; display: grid; min-width: max-content; padding: 6px; background: var(--color-paper); border: 1px solid var(--color-line); border-radius: 6px; box-shadow: 0 4px 12px #0001; }
.submissions-menu-links a { padding: 0 12px; color: var(--color-ink); }
.submissions-menu-links a:hover, .submissions-menu-links a:focus-visible { background: var(--color-surface); }
.submissions-menu-links a[aria-current] { color: var(--color-accent); font-weight: 600; }
/* The row's own box overlaps the header's rule by 1px, so the current item's underline sits on the rule without the
 * scroll box clipping it. The narrowest phones tighten the spacing; what still does not fit scrolls sideways (ADR 0031). */
@media (max-width: 22.499rem) {
  .contest-menu > a, .submissions-menu summary { padding-inline: 4px; font-size: .8125rem; }
}
</style>
