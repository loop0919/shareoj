<script setup lang="ts">
import type { RouteLocationRaw } from 'vue-router'
import { lastPage, pageRange } from '~/utils/pagination'
import { pageSizes, type PageSize } from '~/composables/usePageSize'
// Above a paged table: which rows are on screen, the page size and the page numbers (ADR 0028). It counts rows, so the
// standings count tied ranks and a team's row once each. The list leaves it out while it is empty.
const props = defineProps<{ total: number, page: number, unit: '問' | '件', link: (page: number) => RouteLocationRaw }>()
const size = defineModel<PageSize>('size', { required: true })
const sizeId = useId()
const count = (value: number) => value.toLocaleString('en-US')
const summary = computed(() => {
  const { first, last } = pageRange(props.total, props.page, size.value)
  return `全 ${count(props.total)} ${props.unit}中 ${count(first)}〜${count(last)} ${props.unit}目`
})
</script>
<template>
  <div class="list-toolbar">
    <p class="list-count">{{ summary }}</p>
    <div class="list-size"><label :for="sizeId">表示件数</label><select :id="sizeId" v-model="size"><option v-for="option in pageSizes" :key="option" :value="option">{{ option }}</option></select></div>
    <PageNav :page="page" :last="lastPage(total, size)" :link="link" />
  </div>
</template>
<style scoped>
.list-toolbar { display: flex; flex-wrap: wrap; align-items: center; gap: 8px 24px; margin-bottom: 12px; font-size: .8125rem; }
.list-count { margin: 0; font-variant-numeric: tabular-nums; }
.list-size { display: flex; align-items: center; gap: 8px; color: var(--color-muted); white-space: nowrap; }
.list-size select { min-height: var(--editor-control-size); padding: 2px 8px; border: 1px solid var(--color-line); border-radius: 4px; font: inherit; font-variant-numeric: tabular-nums; cursor: pointer; }
.list-size select:hover { border-color: var(--color-muted); }
.list-size select:focus-visible { outline: 3px solid var(--color-accent); outline-offset: 2px; }
/* The numbers keep to the right, on the count's line while it fits and on a line of their own after. */
.list-toolbar > .page-nav { margin-inline-start: auto; }
</style>
