<script setup lang="ts">
import { languageOptions, languagePatch, languageValue, submissionConditionChips, submissionConditionReset, submissionSorts, submissionVerdicts, type RuntimeLanguage, type SubmissionConditionKey, type SubmissionConditions, type SubmissionSort } from '~/utils/submission-conditions'

// The sort and the filters of a submission list, in the panel that ConditionsPanel folds under a button. Each change
// goes up at once; there is no apply button.
// languages come from the judge's catalogue, and the conditions name only a language or a runtime it lists
// (useSubmissionConditions); before it has loaded, or from an older API, the 言語 select offers only すべて. The 提出者
// box comes only withUser, and the 問題 select only with problems (a contest's, value and label for each).
// The bar slot ends the button's row, for the page's own controls. focus moves the focus to the button, for a control
// of the page's that leaves the page when it takes effect.
const props = defineProps<{ conditions: SubmissionConditions, languages: RuntimeLanguage[], withUser: boolean, problems?: { value: string, label: string }[] }>()
const emit = defineEmits<{ change: [patch: Partial<SubmissionConditions>], clear: [] }>()
const open = defineModel<boolean>('open', { required: true })
const shell = ref<{ focus: () => void }>()
defineExpose({ focus: () => shell.value?.focus() })
const chips = computed(() => submissionConditionChips(props.conditions, props.languages))
const verdicts = [{ key: '', label: 'すべて' }, ...submissionVerdicts.map(verdict => ({ key: verdict, label: verdict }))]
const languageGroups = computed(() => languageOptions(props.languages).map(group => ({ label: group.label, options: group.options.map(option => ({ key: option.value, label: option.label })) })))
const problemChoices = computed(() => [{ key: '', label: 'すべて' }, ...(props.problems ?? []).map(problem => ({ key: problem.value, label: problem.label }))])
// The fields show the conditions and send each pick up.
const sort = computed({ get: () => props.conditions.sort, set: (value: string) => emit('change', { sort: value as SubmissionSort }) })
const verdict = computed({ get: () => props.conditions.verdict, set: (value: string) => emit('change', { verdict: value }) })
const language = computed({ get: () => languageValue(props.conditions), set: (value: string) => emit('change', languagePatch(value)) })
const problem = computed({ get: () => props.conditions.problem, set: (value: string) => emit('change', { problem: value }) })
// Only the condition taken off goes up: a second chip taken off before the list catches up then keeps the first one off.
function remove(key: string) {
  emit('change', submissionConditionReset(key as SubmissionConditionKey))
}
</script>
<template>
  <ConditionsPanel ref="shell" v-model:open="open" :chips="chips" @remove="remove" @clear="emit('clear')">
    <template #bar><slot name="bar" /></template>
    <FilterSelect v-model="sort" label="並び順" :options="submissionSorts" />
    <FilterSelect v-model="verdict" label="結果" :options="verdicts" />
    <FilterSelect v-model="language" label="言語" :options="[{ key: '', label: 'すべて' }]" :groups="languageGroups" />
    <div v-if="withUser" class="conditions-field">
      <span class="conditions-label" aria-hidden="true">提出者</span>
      <UserFilter :user="conditions.user" @change="user => emit('change', { user })" />
    </div>
    <FilterSelect v-if="problems" v-model="problem" label="問題" :options="problemChoices" />
  </ConditionsPanel>
</template>
