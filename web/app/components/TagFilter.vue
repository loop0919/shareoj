<script setup lang="ts">
import { normalizeTag, tagListSchema, type Tag } from '~~/shared/types/tag'
import { canonicalTag } from '~/utils/tag-search'

// Narrows a public list to one tag. The choices are the tags that list actually uses.
const props = defineProps<{ kind: 'problem' | 'post', tag: string, to: '/problems' | '/blog' }>()
const known = ref<Tag[]>([])
const text = ref('')
onMounted(async () => {
  try { known.value = tagListSchema.parse(await $fetch('/api/tags')).items } catch {}
})
const choices = computed(() => known.value.filter(tag => props.kind === 'problem' ? tag.problemCount : tag.postCount))
const id = computed(() => `${props.kind}-tag-filter`)
async function apply() {
  const value = normalizeTag(text.value)
  if (value) await navigateTo({ path: props.to, query: { tag: canonicalTag(choices.value, value) } })
}
</script>
<template>
  <div class="tag-filter">
    <form role="search" :aria-label="kind === 'problem' ? '問題をタグで絞り込む' : '記事をタグで絞り込む'" @submit.prevent="apply">
      <label :for="id">タグで絞り込む</label>
      <input :id="id" v-model="text" :list="`${id}-options`" autocomplete="off" enterkeyhint="search" placeholder="タグ名">
      <datalist :id="`${id}-options`"><option v-for="choice in choices" :key="choice.name" :value="choice.name" /></datalist>
      <button type="submit" class="editor-button" :disabled="!text.trim()">絞り込む</button>
    </form>
    <p v-if="tag" class="tag-filter-current">タグ「<strong>{{ tag }}</strong>」で絞り込んでいます。<NuxtLink :to="to">絞り込みを解除</NuxtLink></p>
  </div>
</template>
<style scoped>
.tag-filter { margin-bottom: 20px; }
.tag-filter form { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
.tag-filter label { font-size: .8125rem; font-weight: 600; }
.tag-filter input { flex: 0 1 16rem; min-width: 0; min-height: 32px; padding: 4px 10px; border: 1px solid var(--color-line); border-radius: 4px; background: var(--color-paper); color: var(--color-ink); font: inherit; font-size: .875rem; }
.tag-filter input:focus-visible { outline: 3px solid var(--color-accent); outline-offset: 2px; }
.tag-filter-current { margin: 12px 0 0; font-size: .875rem; overflow-wrap: anywhere; }
.tag-filter-current a { margin-inline-start: 8px; }
</style>
