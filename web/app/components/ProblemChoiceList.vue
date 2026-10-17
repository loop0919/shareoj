<script setup lang="ts">
import type { ProblemChoice } from '~/utils/problem-choices'
// hideBlockedLabel adds a toggle that hides blocked problems; bounded keeps a long list from pushing the rest of the form away.
const props = defineProps<{ choices: ProblemChoice[], legend: string, name: string, disabled?: boolean, hideBlockedLabel?: string, bounded?: boolean }>()
const selected = defineModel<string>({ required: true })
const noteId = (choice: ProblemChoice) => choice.reason || choice.note ? `${props.name}-${choice.id}-note` : undefined
const hideBlocked = ref(false)
const blockedCount = computed(() => props.choices.filter(choice => choice.reason).length)
const shown = computed(() => hideBlocked.value ? props.choices.filter(choice => !choice.reason || choice.id === selected.value) : props.choices)
const list = ref<HTMLUListElement>()
// A problem chosen from a link may sit below the fold of the list; bring it into view without moving the page.
watch(selected, async () => {
  if (!props.bounded) return
  await nextTick()
  const box = list.value
  const row = box?.querySelector('input:checked')?.closest('li')
  if (!box || !(row instanceof HTMLElement)) return
  if (row.offsetTop < box.scrollTop || row.offsetTop + row.offsetHeight > box.scrollTop + box.clientHeight) box.scrollTop = row.offsetTop - (box.clientHeight - row.offsetHeight) / 2
}, { immediate: true })
</script>

<template>
  <fieldset class="problem-choices" :disabled="disabled">
    <legend>{{ legend }}</legend>
    <label v-if="hideBlockedLabel && blockedCount" class="choices-switch">
      <input v-model="hideBlocked" type="checkbox" role="switch">
      <span class="switch-track" aria-hidden="true"><span class="switch-thumb" /></span>
      {{ hideBlockedLabel }}<span class="choices-count">{{ blockedCount }}件</span>
    </label>
    <ul ref="list" :data-bounded="bounded || undefined">
      <li v-for="choice in shown" :key="choice.id">
        <label :data-blocked="!!choice.reason || undefined">
          <input v-model="selected" type="radio" :name="name" :value="choice.id" :disabled="!!choice.reason" :aria-describedby="noteId(choice)">
          <span class="choice-title">{{ choice.title }}</span>
          <span class="choice-meta"><DifficultyBadge :level="choice.difficulty" /><span v-if="choice.slot" class="choice-slot">{{ choice.slot }}</span></span>
          <span v-if="choice.reason || choice.note" :id="noteId(choice)" class="choice-note" :data-reason="!!choice.reason || undefined">{{ choice.reason || choice.note }}</span>
        </label>
      </li>
    </ul>
    <p v-if="hideBlocked" class="choices-hidden" role="status">{{ blockedCount }}件を隠しています</p>
  </fieldset>
</template>

<style scoped>
/* Hallmark · component: problem radio list · genre: modern-minimal · theme: existing ShareOJ tokens (Plain)
 * states: default · hover · focus-visible · checked · disabled (with reason) · whole list disabled while busy
 * switch: off · on · hover · focus-visible · active · disabled · reduced motion
 * One bordered list, rows split by rules; the reason sits under the title instead of a tooltip. A bounded list scrolls inside itself. */
.problem-choices { min-width: 0; margin: 0; padding: 0; border: 0; }
/* The floated legend shares its row with the toggle and still names the group. */
.problem-choices legend { float: left; margin-bottom: 8px; padding: 0; font-size: .875rem; font-weight: 600; line-height: 32px; }
/* A switch is a checkbox: the input covers the whole label, so a click toggles it without moving focus or the page. */
.choices-switch { position: relative; float: right; display: inline-flex; align-items: center; gap: 8px; min-height: 44px; margin: -6px 0 2px; font-size: .8125rem; white-space: nowrap; cursor: pointer; }
.choices-switch input { position: absolute; inset: 0; width: 100%; height: 100%; margin: 0; opacity: 0; cursor: pointer; }
.switch-track { position: relative; flex: none; width: 34px; height: 20px; border: 1px solid var(--color-muted); border-radius: 999px; background: var(--color-surface); }
.switch-thumb { position: absolute; top: 2px; left: 2px; width: 14px; height: 14px; border-radius: 50%; background: var(--color-muted); transition: transform 120ms cubic-bezier(.2, 0, 0, 1); }
.choices-switch input:checked + .switch-track { border-color: var(--color-accent); background: var(--color-accent); }
.choices-switch input:checked + .switch-track .switch-thumb { background: var(--color-paper); transform: translateX(14px); }
.choices-switch input:focus-visible + .switch-track { outline: 3px solid var(--color-accent); outline-offset: 2px; }
.choices-switch input:active + .switch-track .switch-thumb { width: 16px; }
.choices-switch:has(input:disabled) { opacity: .55; cursor: not-allowed; }
.choices-switch input:disabled { cursor: not-allowed; }
.choices-count { color: var(--color-muted); font-family: var(--font-code); font-size: .75rem; }
ul { clear: both; margin: 0; padding: 0; list-style: none; border: 1px solid var(--color-line); border-radius: 4px; }
ul[data-bounded] { position: relative; max-height: min(19rem, 42dvh); overflow-y: auto; overscroll-behavior: contain; }
li + li { border-top: 1px solid var(--color-line); }
li label { display: grid; grid-template-columns: auto minmax(0, 1fr) auto; align-items: center; gap: 2px 12px; min-height: 56px; padding: 8px 14px; cursor: pointer; }
li label:has(input:checked) { background: var(--color-accent-soft); }
li label[data-blocked] { color: var(--color-muted); cursor: not-allowed; }
li input { width: 18px; height: 18px; margin: 0; accent-color: var(--color-accent); }
li input:focus-visible { outline: 3px solid var(--color-accent); outline-offset: 3px; }
li input:disabled { cursor: not-allowed; }
.choice-title { min-width: 0; font-size: .9375rem; font-weight: 500; overflow-wrap: anywhere; }
.choice-meta { display: inline-flex; align-items: center; gap: 10px; font-size: .8125rem; white-space: nowrap; }
.choice-slot { color: var(--color-muted); font-family: var(--font-code); font-size: .75rem; }
.choice-note { grid-column: 2 / -1; font-size: .75rem; line-height: 1.6; color: var(--color-muted); overflow-wrap: anywhere; }
.choice-note[data-reason] { color: var(--color-error); }
.choices-hidden { margin: 6px 0 0; color: var(--color-muted); font-size: .75rem; }
.problem-choices:disabled li label { cursor: progress; }
@media (hover: hover) {
  li label:not([data-blocked]):not(:has(input:checked)):hover { background: var(--color-surface); }
  .choices-switch:hover .switch-track { border-color: var(--color-ink); }
  .choices-switch:hover input:checked + .switch-track { border-color: var(--color-accent); }
}
@media (prefers-reduced-motion: reduce) {
  .switch-thumb { transition: none; }
}
@media (max-width: 400px) {
  li label { padding-inline: 10px; gap: 2px 10px; }
}
</style>
