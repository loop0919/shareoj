<script setup lang="ts">
import { normalizeTag, tagListSchema, type Tag } from '~~/shared/types/tag'
import { canonicalTag } from '~/utils/tag-search'

// Narrows a public list to one tag. The choices are the tags that list actually uses.
const props = defineProps<{ kind: 'problem' | 'post', tag: string, to: '/problems' | '/blog' }>()
const known = ref<Tag[]>([])
const text = ref(props.tag)
onMounted(async () => {
  try { known.value = tagListSchema.parse(await $fetch('/api/tags')).items } catch {}
})
const choices = computed(() => known.value.filter(tag => props.kind === 'problem' ? tag.problemCount : tag.postCount))
const id = computed(() => `${props.kind}-tag-filter`)
async function apply() {
  const value = normalizeTag(text.value)
  const next = value ? canonicalTag(choices.value, value) : ''
  if (next !== props.tag) await navigateTo(next ? { path: props.to, query: { tag: next } } : props.to)
}
// Picking a suggestion applies it at once; typed text waits for Enter.
function picked(event: Event) {
  const kind = (event as InputEvent).inputType
  if ((!kind || kind === 'insertReplacementText') && choices.value.some(choice => choice.name === text.value)) void apply()
}
</script>
<template>
  <form class="tag-filter" role="search" :aria-label="kind === 'problem' ? '問題をタグで絞り込む' : '記事をタグで絞り込む'" @submit.prevent="apply">
    <div class="tag-filter-field">
      <input :id="id" v-model="text" :class="{ clearable: tag }" :list="`${id}-options`" aria-label="タグで絞り込む" placeholder="タグで絞り込む" autocomplete="off" enterkeyhint="search" @input="picked">
      <datalist :id="`${id}-options`"><option v-for="choice in choices" :key="choice.name" :value="choice.name" /></datalist>
      <NuxtLink v-if="tag" class="tag-filter-clear" :to="to" aria-label="絞り込みを解除" title="絞り込みを解除"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M6 6l12 12M18 6 6 18" /></svg></NuxtLink>
    </div>
    <button type="submit" class="editor-button" :disabled="!text.trim() && !tag">絞り込む</button>
  </form>
</template>
<style scoped>
.tag-filter { display: flex; gap: 8px; width: min(100%, 24rem); margin-bottom: 20px; }
.tag-filter-field { position: relative; flex: 1; min-width: 0; }
.tag-filter input { box-sizing: border-box; width: 100%; min-height: 36px; padding: 6px 10px; border: 1px solid var(--color-line); border-radius: 4px; background: var(--color-paper); color: var(--color-ink); font: inherit; font-size: .875rem; }
.tag-filter input.clearable { padding-right: 36px; }
/* The search button replaces Chrome's datalist arrow, which sat left of the clear button. */
.tag-filter input::-webkit-calendar-picker-indicator { display: none !important; }
.tag-filter input:hover { border-color: var(--color-muted); }
.tag-filter input:focus-visible { outline: 3px solid var(--color-accent); outline-offset: 2px; }
.tag-filter-clear { position: absolute; inset-block: 0; right: 2px; display: inline-flex; align-items: center; justify-content: center; width: 32px; color: var(--color-muted); }
.tag-filter-clear:hover { color: var(--color-ink); }
.tag-filter-clear:focus-visible { outline: 3px solid var(--color-accent); outline-offset: -3px; border-radius: 4px; }
.tag-filter-clear .editor-icon { width: 14px; height: 14px; }
.tag-filter .editor-button { flex-shrink: 0; min-height: 36px; }
@media (max-width: 39.999rem) { .tag-filter input { font-size: 1rem; } }
</style>
