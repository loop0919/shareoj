<script setup lang="ts">
import { maxFullScore, maxScoringGroups, type ProblemDraft, type Scoring } from '~~/shared/types/problem-draft'
import { fullScore, groupNameError, type ScoringMemory, type TestCase } from '~/utils/problem-draft'

// Partial scoring (ADR 0025): the mode, the groups with their points, and the aggregator's code. Cases join groups in
// the test case editor; this editor keeps their membership in step when a group is renamed or removed.
const scoring = defineModel<ProblemDraft['scoring']>('scoring', { required: true })
const cases = defineModel<TestCase[]>('cases', { required: true })
const props = defineProps<{ disabled: boolean, contest: boolean, memory: ScoringMemory }>()
const emit = defineEmits<{ 'show-cases': [] }>()
const { data: catalog, error: catalogError } = useJudgeCatalog()
const available = computed(() => catalogError.value || catalog.value?.maintenance ? [] : catalog.value?.items ?? [])
const specOpen = useState('code-spec-open', () => false)
type Mode = 'binary' | Scoring['mode']
const modes: { value: Mode, label: string }[] = [{ value: 'binary', label: '一括' }, { value: 'groups', label: 'グループ' }, { value: 'aggregate', label: '集計コード' }]
const modeNotes: Record<Mode, string> = {
  binary: 'すべてのケースでACなら正解です。部分点はありません。',
  groups: 'グループに属するケースがすべてACなら、そのグループの配点を与えます。提出の得点は、グループの得点の合計です。',
  aggregate: '集計コードが、ケースの判定と点からグループごとの得点を決めます。提出の得点は、グループの得点の合計です。',
}
// Switching back to 一括 clears the groups from the draft, and switching to グループ clears the aggregator. The page keeps
// them in memory, so switching again restores them even after another section was opened.
const defaultRuntime = () => available.value.find(item => item.id === 'cpp23-gcc')?.id ?? available.value[0]?.id ?? 'cpp23-gcc'
// Leaving a partial mode, or the aggregate one, takes work out of the draft, so it is confirmed first.
function confirmSwitch(current: Scoring | null, next: Mode) {
  if (!current) return true
  const code = current.mode === 'aggregate' && !!current.aggregator?.source.trim()
  if (next === 'binary' && (current.groups.length || cases.value.some(item => item.groups?.length) || code))
    return window.confirm(`一括にすると、グループと配点、ケースの所属${code ? '、集計コード' : ''}を下書きから外します。このページを開いている間は、方式を戻すと元に戻ります。一括にしますか？`)
  if (next === 'groups' && code)
    return window.confirm('グループにすると、集計コードを下書きから外します。このページを開いている間は、集計コードに戻すと元に戻ります。グループにしますか？')
  return true
}
const mode = computed<Mode>({
  get: () => scoring.value?.mode ?? 'binary',
  set: next => {
    const current = scoring.value
    if (!confirmSwitch(current, next)) {
      // The radio has already moved; put it back on the mode that stays.
      nextTick(() => { for (const input of document.querySelectorAll<HTMLInputElement>('input[name="scoring-mode"]')) input.checked = input.value === mode.value })
      return
    }
    const memory = props.memory
    if (next === 'binary') {
      if (!current) return
      memory.previous = { scoring: structuredClone(toRaw(current)), membership: new WeakMap(cases.value.map(item => [toRaw(item), [...item.groups ?? []]])) }
      for (const item of cases.value) item.groups = []
      scoring.value = null
      return
    }
    let groups = current?.groups.map(group => ({ ...group }))
    let aggregator = current?.aggregator ?? memory.aggregator
    if (!groups) {
      // A new partial problem starts with one group holding every case but the samples, which scores like 一括.
      const previous = memory.previous
      groups = previous?.scoring.groups.map(group => ({ ...group })) ?? [{ name: '小課題1', points: 100 }]
      aggregator ??= previous?.scoring.aggregator
      const names = new Set(groups.map(group => group.name))
      for (const item of cases.value) {
        const kept = previous?.membership.get(toRaw(item))
        item.groups = kept ? kept.filter(name => names.has(name)) : !previous && !item.isSample ? [groups[0]!.name] : []
      }
    }
    if (next === 'aggregate') aggregator ??= { runtime: defaultRuntime(), source: '' }
    else if (aggregator) memory.aggregator = structuredClone(toRaw(aggregator))
    // The draft's scoring is replaced at once: the model only reflects it after the parent renders.
    scoring.value = { mode: next, groups, aggregator: next === 'aggregate' ? aggregator : undefined }
  },
})
const groups = computed(() => scoring.value?.groups ?? [])
const total = computed(() => fullScore({ groups: groups.value }))
const totalError = computed(() => groups.value.length && (total.value < 1 || total.value > maxFullScore) ? '満点（配点の合計）を1〜1,000,000点にしてください。' : '')
const caseName = (item: TestCase, index: number) => item.name?.trim() || `ケース${index + 1}`
const counts = computed(() => {
  const result = new Map<string, number>()
  for (const item of cases.value) for (const name of item.groups ?? []) result.set(name, (result.get(name) ?? 0) + 1)
  return result
})
const ungrouped = computed(() => cases.value.flatMap((item, index) => !item.isSample && !item.groups?.length ? [caseName(item, index)] : []))
const nameErrors = reactive<Record<number, string>>({})
const pointErrors = reactive<Record<number, string>>({})
// A name is committed when it is valid, so cases never point at a name that is still being typed.
function rename(index: number, event: Event) {
  const input = event.target as HTMLInputElement
  const group = groups.value[index]
  if (!group) return
  const name = input.value.trim()
  const error = groupNameError(input.value, groups.value.filter((_, at) => at !== index).map(other => other.name))
  nameErrors[index] = error
  if (error || name === group.name) return
  const old = group.name
  group.name = name
  input.value = name
  for (const item of cases.value) if (item.groups?.includes(old)) item.groups = item.groups.map(value => value === old ? name : value)
}
function parsePoints(value: string) {
  return /^\d{1,7}$/.test(value.trim()) && Number(value) <= maxFullScore ? Number(value) : undefined
}
function updatePoints(index: number, event: Event) {
  const points = parsePoints((event.target as HTMLInputElement).value)
  const group = groups.value[index]
  if (!group || points === undefined) return
  pointErrors[index] = ''
  group.points = points
}
function checkPoints(index: number, event: Event) {
  pointErrors[index] = parsePoints((event.target as HTMLInputElement).value) === undefined ? '配点は0〜1,000,000点の整数で入力してください。' : ''
}
async function addGroup() {
  if (!scoring.value || groups.value.length >= maxScoringGroups) return
  const names = new Set(groups.value.map(group => group.name.trim()))
  let number = groups.value.length + 1
  while (names.has(`小課題${number}`)) number++
  // The first group carries the points so the full score stays valid; later ones start at 0 and leave it unchanged.
  scoring.value.groups.push({ name: `小課題${number}`, points: groups.value.length ? 0 : 100 })
  await nextTick()
  document.querySelector<HTMLInputElement>(`#group-name-${groups.value.length - 1}`)?.select()
}
function removeGroup(index: number) {
  const group = groups.value[index]
  if (!scoring.value || !group || !window.confirm(`「${group.name}」を削除しますか？ケースの所属からも外します。`)) return
  scoring.value.groups.splice(index, 1)
  for (const item of cases.value) if (item.groups?.includes(group.name)) item.groups = item.groups.filter(name => name !== group.name)
  for (const errors of [nameErrors, pointErrors]) for (const key of Object.keys(errors)) delete errors[Number(key)]
}
// A draft from the CLI may be in the aggregate mode without an aggregator; the first edit creates it.
const aggregating = computed(() => scoring.value?.mode === 'aggregate')
function aggregatorDraft() {
  const current = scoring.value!
  current.aggregator ??= { runtime: defaultRuntime(), source: '' }
  return current.aggregator
}
const aggregatorRuntime = computed({
  get: () => scoring.value?.aggregator?.runtime ?? defaultRuntime(),
  set: runtime => { aggregatorDraft().runtime = runtime },
})
const aggregatorSource = computed({
  get: () => scoring.value?.aggregator?.source ?? '',
  set: source => { aggregatorDraft().source = source },
})
const aggregatorError = computed(() => aggregating.value && (!aggregatorSource.value.trim() || new TextEncoder().encode(aggregatorSource.value).length > 65536 || aggregatorSource.value.includes('\0'))
  ? '公開・採点するには集計コードを1〜65,536バイトで、NUL文字を含めずに入力してください。' : '')
</script>

<template>
  <section class="problem-management scoring-editor" aria-labelledby="scoring-title">
    <div class="management-content">
      <header>
        <h1 id="scoring-title">採点</h1>
        <p class="muted">提出の得点の付け方を選びます。<NuxtLink to="/docs/authoring#scoring" target="_blank" rel="noopener noreferrer">部分点の設定 ↗</NuxtLink></p>
      </header>
      <div class="scoring-mode">
        <SegmentedChoice v-model="mode" legend="採点方式" name="scoring-mode" :options="modes" :disabled="props.disabled" />
        <p class="scoring-note">{{ modeNotes[mode] }}</p>
        <p v-if="contest" class="scoring-note">コンテストの開始後は、採点方式とグループの名前・配点・順番を変えられません。</p>
      </div>
      <section v-if="scoring" class="scoring-block" aria-labelledby="scoring-groups-title">
        <div class="scoring-heading"><h2 id="scoring-groups-title">グループ</h2><p>満点 <strong>{{ total }}</strong> 点</p></div>
        <p v-if="totalError" class="field-error" role="alert">{{ totalError }}</p>
        <p v-if="!groups.length" class="muted">グループを追加してください。満点は、グループの配点の合計です。</p>
        <table v-else class="scoring-groups">
          <thead><tr><th scope="col">グループ名</th><th scope="col">配点</th><th scope="col">ケース</th><th scope="col"><span class="visually-hidden">操作</span></th></tr></thead>
          <tbody>
            <tr v-for="(group, index) in groups" :key="index">
              <td>
                <input :id="`group-name-${index}`" :value="group.name" :aria-label="`グループ${index + 1}の名前`" :aria-invalid="!!nameErrors[index]" :aria-describedby="nameErrors[index] ? `group-name-error-${index}` : undefined" :disabled="disabled" maxlength="64" autocomplete="off" @change="rename(index, $event)" @keydown.enter.prevent="rename(index, $event)">
                <p v-if="nameErrors[index]" :id="`group-name-error-${index}`" class="field-error" role="alert">{{ nameErrors[index] }}</p>
              </td>
              <td>
                <span class="points-input"><input :value="group.points" type="number" min="0" :max="maxFullScore" step="1" inputmode="numeric" :aria-label="`グループ${index + 1}の配点`" :aria-invalid="!!pointErrors[index]" :aria-describedby="pointErrors[index] ? `group-points-error-${index}` : undefined" :disabled="disabled" @input="updatePoints(index, $event)" @change="checkPoints(index, $event)"> 点</span>
                <p v-if="pointErrors[index]" :id="`group-points-error-${index}`" class="field-error" role="alert">{{ pointErrors[index] }}</p>
              </td>
              <td class="group-count" :data-empty="!counts.get(group.name) || undefined">{{ counts.get(group.name) ?? 0 }} 件</td>
              <td><button type="button" class="editor-button group-remove" :disabled="disabled" :aria-label="`「${group.name}」を削除`" title="グループを削除" @click="removeGroup(index)"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M3 6h18M9 6V3h6v3M5 6l1 15h12l1-15M10 10v7M14 10v7" /></svg></button></td>
            </tr>
          </tbody>
        </table>
        <button type="button" class="editor-button group-add" :disabled="disabled || groups.length >= maxScoringGroups" @click="addGroup"><span aria-hidden="true">＋</span> グループを追加</button>
        <p class="scoring-note">ケースの所属は「テストケース」で選びます。1つのケースを複数のグループに入れられます。サンプルはどのグループに入れなくてもかまいません。</p>
        <div v-if="ungrouped.length" class="scoring-warning">
          <p>どのグループにも属していないケース（{{ ungrouped.length }}件）：{{ ungrouped.join('、') }}</p>
          <button type="button" class="editor-button" @click="emit('show-cases')">テストケースを開く</button>
        </div>
      </section>
      <section v-if="aggregating" class="scoring-block" aria-labelledby="aggregator-title">
        <h2 id="aggregator-title">集計コード</h2>
        <div class="aggregator-option">
          <label for="aggregator-language">集計コードの言語</label>
          <select id="aggregator-language" v-model="aggregatorRuntime" :disabled="disabled">
            <option v-if="!available.some(item => item.id === aggregatorRuntime)" :value="aggregatorRuntime">{{ aggregatorRuntime }}（現在利用できません）</option>
            <option v-for="item in available" :key="item.id" :value="item.id">{{ item.label }}</option>
          </select>
        </div>
        <details class="code-spec" :open="specOpen" @toggle="specOpen = ($event.target as HTMLDetailsElement).open">
          <summary><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="m9 6 6 6-6 6" /></svg>集計コードの仕様</summary>
          <dl>
            <div><dt>標準入力</dt><dd>1行目にケース数Nとグループ数G、続くG行に各グループの配点、続くN行に各ケースの「判定 点 サンプル k g<sub>1</sub> … g<sub>k</sub>」。判定はAC・WA・TLE・MLE・OLE・RE・SKIPPED（未実行）、点がなければ<code>-</code>、サンプルは1か0、gは0から数えたグループの番号です。</dd></div>
            <div><dt>標準出力</dt><dd>各グループの得点をG行で、0以上その配点以下の10進の小数（例：<code>37.5</code>）で書きます。小数第3位以下は切り捨てます。</dd></div>
            <div><dt>ケースの点</dt><dd>判定コードがスコアのファイルに書いた整数か、testlib形式の<code>quitp</code>に渡した整数です。<NuxtLink to="/docs/checkers#aggregator" target="_blank" rel="noopener noreferrer">コード例 ↗</NuxtLink></dd></div>
            <div><dt>制限</dt><dd>CPU 5秒・512 MiB。失敗や書式違いは、提出全体がJEになります。</dd></div>
          </dl>
        </details>
        <SourceCodeEditor v-model="aggregatorSource" :runtime="aggregatorRuntime" label="集計コード" :disabled="disabled" />
        <p v-if="aggregatorError" class="editor-error" role="alert">{{ aggregatorError }}</p>
      </section>
    </div>
  </section>
</template>

<style scoped>
/* Hallmark · component: scoring settings · genre: modern-minimal · theme: existing ShareOJ tokens
 * pre-emit critique: P4 H4 E4 S5 R5 V4 · the full score sits beside the groups that make it up */
.scoring-mode { max-width: 32rem; }
.scoring-note { margin: 8px 0 0; color: var(--color-muted); font-size: .8125rem; line-height: 1.7; }
.scoring-block { margin-top: 32px; padding-top: 24px; border-top: 1px solid var(--color-line); }
.scoring-block h2 { margin: 0 0 12px; font-size: 1rem; }
.scoring-heading { display: flex; flex-wrap: wrap; align-items: baseline; justify-content: space-between; gap: 4px 16px; }
.scoring-heading p { margin: 0 0 12px; font-size: .875rem; }
.scoring-heading strong { font-size: 1.25rem; font-variant-numeric: tabular-nums; }
.scoring-groups { width: 100%; border-collapse: collapse; font-size: .875rem; }
.scoring-groups th { padding: 6px 8px; border-bottom: 1px solid var(--color-line); color: var(--color-muted); font-size: .75rem; font-weight: 600; text-align: left; }
.scoring-groups td { padding: 6px 8px; border-bottom: 1px solid var(--color-line); vertical-align: top; }
.scoring-groups th:nth-child(2), .scoring-groups td:nth-child(2) { width: 9.5em; }
.scoring-groups th:nth-child(3), .scoring-groups td:nth-child(3) { width: 4.5em; text-align: right; }
.scoring-groups th:last-child, .scoring-groups td:last-child { width: 44px; }
.scoring-groups input { box-sizing: border-box; width: 100%; min-height: 36px; padding: 6px 8px; border: 1px solid var(--color-line); border-radius: 4px; background: var(--color-paper); color: var(--color-ink); font: inherit; }
.scoring-groups input[aria-invalid="true"] { border-color: var(--color-error); }
.points-input { display: inline-flex; align-items: center; gap: 6px; white-space: nowrap; }
.points-input input { width: 6.5em; text-align: right; font-variant-numeric: tabular-nums; }
.scoring-groups .field-error { margin: 4px 0 0; }
.group-count { padding-top: 14px !important; color: var(--color-muted); white-space: nowrap; font-variant-numeric: tabular-nums; }
.group-count[data-empty] { color: var(--color-error); }
.group-remove { display: inline-flex; align-items: center; justify-content: center; width: 36px; min-height: 36px; padding: 0; color: var(--color-error); border-color: var(--color-error); }
.group-remove:hover:not(:disabled) { background: var(--color-error); color: var(--color-paper); }
.group-add { min-height: 40px; margin-top: 12px; font-weight: 600; }
.scoring-warning { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 8px 16px; margin-top: 16px; padding: 10px 12px; border: 1px solid var(--color-error); border-radius: 4px; }
.scoring-warning p { flex: 1 1 20rem; margin: 0; color: var(--color-error); font-size: .8125rem; overflow-wrap: anywhere; }
.aggregator-option { display: flex; flex-direction: column; gap: 4px; max-width: 100%; }
.aggregator-option label { font-size: .75rem; }
.aggregator-option select { align-self: flex-start; max-width: 100%; min-height: 36px; padding: 4px 8px; border: 1px solid var(--color-line); border-radius: 4px; background: var(--color-paper); color: var(--color-ink); }
.scoring-block :deep(.source-code-editor) { margin-block: 12px; }
button:disabled { opacity: .5; cursor: not-allowed; }
@media (pointer: coarse) { .group-remove { width: 44px; min-height: 44px; } .group-add { min-height: 44px; } }
@media (max-width: 480px) {
  .scoring-groups th:nth-child(2), .scoring-groups td:nth-child(2) { width: 7.5em; }
  .points-input input { width: 5em; }
  .scoring-groups th, .scoring-groups td { padding-inline: 4px; }
}
</style>
