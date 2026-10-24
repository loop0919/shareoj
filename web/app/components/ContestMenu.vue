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
onMounted(() => document.addEventListener('click', outside))
onBeforeUnmount(() => document.removeEventListener('click', outside))
watch(() => route.fullPath, close)
</script>

<template>
  <nav class="contest-menu" aria-label="コンテストメニュー">
    <NuxtLink v-for="view in (['overview', 'problems', 'standings'] as const)" :key="view" :to="link(view)" :aria-current="current(view)">{{ contestViews[view] }}</NuxtLink>
    <details v-if="canViewAll" ref="submissionsMenu" class="submissions-menu" :data-current="inSubmissions || undefined" @keydown.esc.prevent="close(); submissionsMenu?.querySelector('summary')?.focus()">
      <summary>提出</summary>
      <div class="submissions-menu-links">
        <NuxtLink :to="link('my-submissions')" :aria-current="current('my-submissions')">{{ own }}</NuxtLink>
        <NuxtLink :to="link('submissions')" :aria-current="current('submissions')">{{ contestViews.submissions }}</NuxtLink>
      </div>
    </details>
    <!-- A link to the whole list still lands in this place, so it stays marked there too. -->
    <NuxtLink v-else :to="link('my-submissions')" :aria-current="current('my-submissions') ?? (inSubmissions ? 'true' : undefined)">{{ own }}</NuxtLink>
    <NuxtLink v-for="view in (['editorials', 'clarifications'] as const)" :key="view" :to="link(view)" :aria-current="current(view)">{{ contestViews[view] }}</NuxtLink>
  </nav>
</template>

<style scoped>
.contest-menu > a, .submissions-menu summary { min-height: 44px; margin-bottom: -1px; border-bottom: 2px solid transparent; color: var(--color-muted); }
.contest-menu > a:hover, .submissions-menu summary:hover { color: var(--color-accent); }
.contest-menu > a[aria-current], .submissions-menu[data-current] summary { border-bottom-color: var(--color-accent); color: var(--color-accent); font-weight: 600; }
.submissions-menu { position: relative; }
.submissions-menu summary { display: flex; align-items: center; gap: 6px; font-size: .875rem; white-space: nowrap; cursor: pointer; list-style: none; }
.submissions-menu summary::-webkit-details-marker { display: none; }
.submissions-menu summary::after { content: '▾'; font-size: .75rem; }
.submissions-menu-links { position: absolute; left: 0; top: 100%; z-index: 20; display: grid; min-width: max-content; padding: 6px; background: var(--color-paper); border: 1px solid var(--color-line); border-radius: 6px; box-shadow: 0 4px 12px #0001; }
.submissions-menu-links a { padding: 0 12px; color: var(--color-ink); }
.submissions-menu-links a:hover, .submissions-menu-links a:focus-visible { background: var(--color-surface); }
.submissions-menu-links a[aria-current] { color: var(--color-accent); font-weight: 600; }
/* With the layout's narrower gap, this keeps the row within the narrowest phones. */
@media (max-width: 22.499rem) {
  .contest-menu > a, .submissions-menu summary { font-size: .8125rem; }
}
</style>
