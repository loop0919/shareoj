<script setup lang="ts">
import type { ProblemChoice } from '~/utils/problem-choices'
const props = defineProps<{ choices: ProblemChoice[], legend: string, name: string, disabled?: boolean }>()
const selected = defineModel<string>({ required: true })
const noteId = (choice: ProblemChoice) => choice.reason || choice.note ? `${props.name}-${choice.id}-note` : undefined
</script>

<template>
  <fieldset class="problem-choices" :disabled="disabled">
    <legend>{{ legend }}</legend>
    <ul>
      <li v-for="choice in choices" :key="choice.id">
        <label :data-blocked="!!choice.reason || undefined">
          <input v-model="selected" type="radio" :name="name" :value="choice.id" :disabled="!!choice.reason" :aria-describedby="noteId(choice)">
          <span class="choice-title">{{ choice.title }}</span>
          <span class="choice-meta"><DifficultyBadge :level="choice.difficulty" /><span v-if="choice.slot" class="choice-slot">{{ choice.slot }}</span></span>
          <span v-if="choice.reason || choice.note" :id="noteId(choice)" class="choice-note" :data-reason="!!choice.reason || undefined">{{ choice.reason || choice.note }}</span>
        </label>
      </li>
    </ul>
  </fieldset>
</template>

<style scoped>
/* Hallmark · component: problem radio list · genre: modern-minimal · theme: existing ShareOJ tokens (Plain)
 * states: default · hover · focus-visible · checked · disabled (with reason) · whole list disabled while busy
 * One bordered list, rows split by rules; the reason sits under the title instead of a tooltip. */
.problem-choices { min-width: 0; margin: 0; padding: 0; border: 0; }
.problem-choices legend { margin-bottom: 8px; padding: 0; font-size: .875rem; font-weight: 600; }
ul { margin: 0; padding: 0; list-style: none; border: 1px solid var(--color-line); border-radius: 4px; }
li + li { border-top: 1px solid var(--color-line); }
label { display: grid; grid-template-columns: auto minmax(0, 1fr) auto; align-items: center; gap: 2px 12px; min-height: 56px; padding: 8px 14px; cursor: pointer; }
label:has(input:checked) { background: var(--color-accent-soft); }
label[data-blocked] { color: var(--color-muted); cursor: not-allowed; }
input { width: 18px; height: 18px; margin: 0; accent-color: var(--color-accent); }
input:focus-visible { outline: 3px solid var(--color-accent); outline-offset: 3px; }
input:disabled { cursor: not-allowed; }
.choice-title { min-width: 0; font-size: .9375rem; font-weight: 500; overflow-wrap: anywhere; }
.choice-meta { display: inline-flex; align-items: center; gap: 10px; font-size: .8125rem; white-space: nowrap; }
.choice-slot { color: var(--color-muted); font-family: var(--font-code); font-size: .75rem; }
.choice-note { grid-column: 2 / -1; font-size: .75rem; line-height: 1.6; color: var(--color-muted); overflow-wrap: anywhere; }
.choice-note[data-reason] { color: var(--color-error); }
.problem-choices:disabled label { cursor: progress; }
@media (hover: hover) {
  label:not([data-blocked]):not(:has(input:checked)):hover { background: var(--color-surface); }
}
@media (max-width: 400px) {
  label { padding-inline: 10px; gap: 2px 10px; }
}
</style>
