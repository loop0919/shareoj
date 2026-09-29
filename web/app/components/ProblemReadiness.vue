<script setup lang="ts">
import { contentIssues, destinationNames, issueMessage, issueSection, problemStatus, type EditorSection, type ProblemStatusInput } from '~/utils/problem-readiness'
const props = defineProps<{ problem: ProblemStatusInput, saved: boolean }>()
const emit = defineEmits<{ jump: [section: EditorSection] }>()
const sectionNames: Record<EditorSection, string> = { statement: '問題文', tests: 'テストケース', checker: '判定方法', editorial: '解説' }
const status = computed(() => problemStatus(props.problem))
// What the author can fix, whatever the problem's state: publishing an update needs it too.
const issues = computed(() => status.value.kind === 'featured' ? status.value.issues : contentIssues(props.problem.readiness))
const stateCodes = new Set(['published', 'ever_published', 'in_contest', 'featured_limit'])
// Where the problem already is limits other destinations; say so once per message.
const notes = computed(() => {
  const readiness = props.problem.readiness
  if (!readiness) return []
  return [...new Set((['publish', 'contest', 'featured'] as const).flatMap(destination =>
    readiness[destination].filter(code => stateCodes.has(code)).map(code => `${destinationNames[destination]}：${issueMessage(code, destination)}`)))]
})
</script>

<template>
  <div class="problem-readiness">
    <p v-if="!saved" class="muted">保存すると、公開やコンテスト、定期便に出せるかを確認できます。</p>
    <p v-else-if="!problem.readiness" class="muted">出せるかの確認は、問題の作成者だけに表示されます。</p>
    <template v-else>
      <div class="readiness-heading"><ProblemStatus :problem="problem" :expandable="false" /><span class="muted">保存済みの内容で判定しています</span></div>
      <p v-if="status.kind === 'featured' && issues.length" class="readiness-warning">定期便に応募中ですが、次の項目を直すまで選出されません。</p>
      <ul v-if="issues.length" class="readiness-issues" aria-label="足りない項目">
        <li v-for="code in issues" :key="code">
          <span>{{ issueMessage(code, status.kind === 'featured' ? 'featured' : undefined) }}</span>
          <button v-if="issueSection(code)" type="button" class="editor-button" @click="emit('jump', issueSection(code)!)">{{ sectionNames[issueSection(code)!] }}を開く</button>
        </li>
      </ul>
      <p v-else-if="status.kind === 'ready'">コンテストにも定期便にも出せます。</p>
      <ul v-if="notes.length" class="readiness-notes" aria-label="提出先の制限">
        <li v-for="note in notes" :key="note">{{ note }}</li>
      </ul>
    </template>
  </div>
</template>

<style scoped>
/* Hallmark · pre-emit critique: P4 H4 E4 S5 R5 V4 · existing ShareOJ tokens; each missing item sits next to the button that opens its section */
.problem-readiness { display: grid; gap: 8px; }
.readiness-heading { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
.readiness-heading .muted { font-size: .8125rem; }
.readiness-warning { margin: 0; color: var(--color-error); }
.readiness-issues { display: grid; gap: 6px; margin: 0; padding: 0; list-style: none; }
.readiness-issues li { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 8px; padding: 6px 10px; border: 1px solid var(--color-line); border-radius: 4px; min-width: 0; }
.readiness-issues li > span { min-width: 0; overflow-wrap: anywhere; }
.readiness-notes { margin: 0; padding-left: 1.2em; color: var(--color-muted); font-size: .8125rem; }
</style>
