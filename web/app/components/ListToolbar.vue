<script setup lang="ts">
import type { RouteLocationRaw } from 'vue-router'
import { lastPage, pageRange } from '~/utils/pagination'
import { pageSizes, type PageSize } from '~/composables/usePageSize'
// Above a paged table: which rows are on screen, the page size and the page numbers (ADR 0028). It counts rows, so the
// standings count tied ranks and a team's row once each. The list leaves it out while it is empty.
const props = defineProps<{ total: number, page: number, unit: '問' | '件', link: (page: number) => RouteLocationRaw }>()
const size = defineModel<PageSize>('size', { required: true })
const sizeId = useId()
const toolbar = ref<HTMLElement>()
const count = (value: number) => value.toLocaleString('en-US')
const summary = computed(() => {
  const { first, last } = pageRange(props.total, props.page, size.value)
  return `全 ${count(props.total)} ${props.unit}中 ${count(first)}〜${count(last)} ${props.unit}目`
})
const countText = ref<HTMLElement>()
// What the status line says: the range, once the shown page or the size has changed. A count that only grows with the
// 15-second polls is not announced.
const announced = ref('')
// The router keeps the scroll position when only the query changes, so a page chosen under the table would open at its
// bottom. Once the shown page changes while this is above the screen, the list comes back up to its first row. The
// number that was activated is gone by then and focus has fallen to the body, so it moves to the count.
watch([() => props.page, size], ([page], [before]) => {
  announced.value = summary.value
  if (page === before) return
  const focused = document.activeElement
  if (!focused || focused === document.body || !document.contains(focused)) countText.value?.focus({ preventScroll: true })
  if (toolbar.value && toolbar.value.getBoundingClientRect().top < 0) toolbar.value.scrollIntoView()
}, { flush: 'post' })
</script>
<template>
  <div ref="toolbar" class="list-toolbar">
    <p ref="countText" class="list-count" tabindex="-1">{{ summary }}</p>
    <span class="sr-only" role="status">{{ announced }}</span>
    <div class="list-size"><label :for="sizeId">表示件数</label><select :id="sizeId" v-model="size"><option v-for="option in pageSizes" :key="option" :value="option">{{ option }}</option></select></div>
    <PageNav :page="page" :last="lastPage(total, size)" :link="link" />
  </div>
</template>
<style scoped>
.list-toolbar { display: flex; flex-wrap: wrap; align-items: center; gap: 8px 24px; margin-bottom: 12px; font-size: .8125rem; scroll-margin-top: 16px; }
.list-count { margin: 0; font-variant-numeric: tabular-nums; }
.list-count:focus-visible { outline: 3px solid var(--color-accent); outline-offset: 2px; }
.sr-only { position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; border: 0; }
.list-size { display: flex; align-items: center; gap: 8px; color: var(--color-muted); white-space: nowrap; }
.list-size select { min-height: var(--editor-control-size); padding: 2px 8px; border: 1px solid var(--color-line); border-radius: 4px; font: inherit; font-variant-numeric: tabular-nums; cursor: pointer; }
.list-size select:hover { border-color: var(--color-muted); }
.list-size select:focus-visible { outline: 3px solid var(--color-accent); outline-offset: 2px; }
/* The numbers keep to the right, on the count's line while it fits and on a line of their own after. */
.list-toolbar > .page-nav { margin-inline-start: auto; }
</style>
