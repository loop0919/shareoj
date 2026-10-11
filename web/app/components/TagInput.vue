<script setup lang="ts">
import { maxTags, normalizeTag, tagError, tagKey, tagListSchema, type Tag } from '~~/shared/types/tag'
import { canonicalTag, searchTags, tagUsage } from '~/utils/tag-search'

// Suggests the tags already in use, so spellings converge instead of splitting into 「DP」「dp」「動的計画法」.
const model = defineModel<string[]>({ required: true })
const props = defineProps<{ id: string, label: string, disabled?: boolean }>()
const root = ref<HTMLElement>()
const known = ref<Tag[]>([])
const text = ref('')
const message = ref('')
const focused = ref(false)
const dismissed = ref(false)
const active = ref(-1)
onMounted(async () => {
  // Without suggestions, tags can still be typed.
  try { known.value = tagListSchema.parse(await $fetch('/api/tags')).items } catch {}
})
type Option = { name: string, note: string, exact: boolean }
const options = computed<Option[]>(() => {
  const value = normalizeTag(text.value)
  if (!value) return []
  const taken = new Set(model.value.map(tagKey))
  const list: Option[] = searchTags(known.value, value).filter(tag => !taken.has(tagKey(tag.name))).slice(0, 8)
    .map(tag => ({ name: tag.name, note: tagUsage(tag), exact: tag.exact }))
  if (!taken.has(tagKey(value)) && !known.value.some(tag => tagKey(tag.name) === tagKey(value)) && !tagError(value)) list.push({ name: value, note: '新しいタグ', exact: false })
  return list
})
const open = computed(() => focused.value && !dismissed.value && options.value.length > 0)
// Enter picks a tag in use that matches after folding case, width and kana; otherwise it adds the typed text.
watch(options, list => { active.value = list[0]?.exact ? 0 : -1 })

// Adding returns the next list, or nothing with a message. A split adds several tags before the parent updates the model.
function add(list: string[], name: string) {
  if (list.some(tag => tagKey(tag) === tagKey(name))) { message.value = `「${name}」はすでに追加されています。`; return }
  if (list.length >= maxTags) { message.value = `タグは${maxTags}個までです。`; return }
  message.value = ''
  return [...list, name]
}
function commit(list: string[], raw: string) {
  const value = normalizeTag(raw)
  if (!value) return list
  const error = tagError(value)
  if (error) { message.value = error; return }
  return add(list, canonicalTag(known.value, value))
}
function apply(next: string[] | undefined) {
  if (!next) return
  if (next !== model.value) model.value = next
  text.value = ''
}
function choose(name: string) { apply(add(model.value, name)) }
// A comma or 「、」 separates tags, including in pasted text. IME input is split once it is confirmed.
function split(event?: Event) {
  dismissed.value = false
  message.value = ''
  if ((event as InputEvent | undefined)?.isComposing) return
  const parts = text.value.split(/[,，、]/)
  if (parts.length < 2) return
  const rest = parts.pop()!
  let list = model.value
  for (const part of parts) list = commit(list, part) ?? list
  if (list !== model.value) model.value = list
  text.value = rest
}
async function reveal() {
  await nextTick()
  root.value?.querySelector(`#${props.id}-option-${active.value}`)?.scrollIntoView({ block: 'nearest' })
}
function keydown(event: KeyboardEvent) {
  if (event.isComposing || event.keyCode === 229) return
  if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
    const count = options.value.length
    if (!count) return
    event.preventDefault()
    dismissed.value = false
    active.value = event.key === 'ArrowDown' ? (active.value + 1) % count : active.value <= 0 ? count - 1 : active.value - 1
    void reveal()
  } else if (event.key === 'Enter') {
    event.preventDefault()
    const option = open.value ? options.value[active.value] : undefined
    if (option) choose(option.name)
    else apply(commit(model.value, text.value))
  } else if (event.key === 'Escape' && open.value) {
    event.preventDefault()
    dismissed.value = true
  }
}
// Leaving the field keeps what was typed instead of dropping it silently.
function blur() {
  focused.value = false
  if (text.value.trim()) apply(commit(model.value, text.value))
}
function remove(index: number) {
  model.value = model.value.filter((_, i) => i !== index)
  message.value = ''
}
watch(() => props.disabled, value => { if (value) dismissed.value = true })
</script>
<template>
  <div ref="root" class="tag-input">
    <ul v-if="model.length" class="tag-chips" :aria-label="label">
      <li v-for="(tag, index) in model" :key="tag"><span>{{ tag }}</span><button type="button" :aria-label="`${tag}を外す`" title="外す" :disabled="disabled" @click="remove(index)"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M6 6l12 12M18 6 6 18" /></svg></button></li>
    </ul>
    <div class="tag-combobox">
      <input
        :id="id" v-model="text" role="combobox" aria-autocomplete="list" autocomplete="off" enterkeyhint="done" placeholder="入力してEnterで追加"
        :aria-expanded="open" :aria-controls="`${id}-options`" :aria-activedescendant="open && active >= 0 ? `${id}-option-${active}` : undefined"
        :aria-describedby="message ? `${id}-message` : undefined" :disabled="disabled"
        @input="split" @compositionend="split()" @keydown="keydown" @focus="focused = true; dismissed = false" @blur="blur"
      >
      <ul v-show="open" :id="`${id}-options`" class="tag-options" role="listbox" :aria-label="`${label}の候補`">
        <li v-for="(option, index) in options" :id="`${id}-option-${index}`" :key="option.name" role="option" :aria-selected="index === active" @mousedown.prevent @click="choose(option.name)">
          <span class="tag-option-name">{{ option.name }}</span><span class="tag-option-note">{{ option.note }}</span>
        </li>
      </ul>
    </div>
    <p v-if="message" :id="`${id}-message`" class="field-error" role="alert">{{ message }}</p>
  </div>
</template>
<style scoped>
.tag-input { display: flex; flex-wrap: wrap; align-items: center; gap: 6px 8px; }
.tag-chips { display: flex; flex-wrap: wrap; gap: 6px 8px; margin: 0; padding: 0; list-style: none; }
.tag-chips li { display: inline-flex; align-items: center; gap: 2px; max-width: 100%; min-height: 32px; padding: 0 2px 0 10px; border: 1px solid var(--color-line); border-radius: 999px; background: var(--color-surface); font-size: .8125rem; overflow-wrap: anywhere; }
.tag-chips button { display: inline-flex; align-items: center; justify-content: center; width: 28px; height: 28px; border: 0; border-radius: 999px; background: none; color: var(--color-muted); cursor: pointer; }
.tag-chips button:hover:not(:disabled) { color: var(--color-ink); background: var(--color-paper); }
.tag-chips .editor-icon { width: 14px; height: 14px; }
.tag-combobox { position: relative; flex: 1 1 14rem; min-width: 0; }
.tag-options { position: absolute; top: calc(100% + 4px); inset-inline: 0; z-index: 30; max-height: min(20rem, 45dvh); overflow-y: auto; margin: 0; padding: 4px; list-style: none; border: 1px solid var(--color-line); border-radius: 4px; background: var(--color-paper); font-size: .875rem; }
.tag-options [role="option"] { display: flex; align-items: baseline; justify-content: space-between; gap: 12px; min-height: 36px; padding: 8px; cursor: pointer; overflow-wrap: anywhere; }
.tag-options [role="option"]:hover, .tag-options [aria-selected="true"] { background: var(--color-surface); }
.tag-option-note { flex-shrink: 0; font-size: .75rem; color: var(--color-muted); }
.field-error { flex-basis: 100%; }
@media (pointer: coarse) { .tag-options [role="option"] { min-height: 44px; } }
</style>
