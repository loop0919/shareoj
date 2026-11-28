<script setup lang="ts">
import { isSubmissionUser } from '~~/shared/utils/submission-conditions'

// Narrows a submission list to one submitter, by user ID. Typed text waits for Enter or the button; an empty box takes
// the condition off.
const props = defineProps<{ user: string }>()
const emit = defineEmits<{ change: [user: string] }>()
const id = useId()
const text = ref(props.user)
// Why the typed text is no user ID, until it changes.
const message = ref('')
// The box shows the submitter in force when the list changes it, not what was typed.
watch(() => props.user, user => { text.value = user })
watch(text, () => { message.value = '' })
// The API refuses what no user ID can be, so its list could only fail: the box says why instead, as the tag search does.
function apply() {
  const value = text.value.trim()
  message.value = value && !isSubmissionUser(value) ? 'ユーザーIDは英小文字で始まる3〜20文字（英小文字・数字・_）です。' : ''
  if (!message.value && value !== props.user) emit('change', value)
}
</script>
<template>
  <form class="user-filter" @submit.prevent="apply">
    <input :id="id" v-model="text" type="text" aria-label="提出者" placeholder="ユーザーID" autocomplete="off" autocapitalize="none" spellcheck="false" enterkeyhint="search" :aria-describedby="message ? `${id}-message` : undefined">
    <button type="submit" class="editor-button" :disabled="!text.trim() && !user">絞り込む</button>
    <p v-if="message" :id="`${id}-message`" class="field-error" role="alert">{{ message }}</p>
  </form>
</template>
<style scoped>
.user-filter { display: flex; flex-wrap: wrap; gap: 8px; width: min(100%, 24rem); }
/* The reason takes a line of its own under the box and the button. */
.user-filter .field-error { flex-basis: 100%; margin: 0; }
.user-filter input { box-sizing: border-box; flex: 1; min-width: 0; min-height: 36px; padding: 6px 10px; border: 1px solid var(--color-line); border-radius: 4px; background: var(--color-paper); color: var(--color-ink); font: inherit; font-size: .875rem; }
.user-filter input:hover { border-color: var(--color-muted); }
.user-filter input:focus-visible { outline: 3px solid var(--color-accent); outline-offset: 2px; }
.user-filter .editor-button { flex-shrink: 0; min-height: 36px; }
/* 1rem keeps a phone from zooming into the box when it takes focus. */
@media (max-width: 39.999rem) { .user-filter input { font-size: 1rem; } }
</style>
