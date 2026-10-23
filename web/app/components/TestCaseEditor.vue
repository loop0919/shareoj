<script setup lang="ts">
import type { ProblemDraft } from '~~/shared/types/problem-draft'
import { testCaseError, type TestCase } from '~/utils/problem-draft'
import { downloadTestFile } from '~/utils/test-files'
import { importTestCaseFiles, mergeTestCases } from '~/utils/import-test-cases'
const cases = defineModel<TestCase[]>({ required: true })
const markdown = defineModel<string>('markdown', { required: true })
// scoring lets each case join the groups of a partially scored problem (ADR 0025).
const props = defineProps<{ disabled: boolean, problemId?: string, scoring?: ProblemDraft['scoring'] }>()
const emit = defineEmits<{ 'show-scoring': [] }>()
const error = computed(() => testCaseError(cases.value))
const selected = ref(0)
const inputFiles = ref<HTMLInputElement>()
const outputFiles = ref<HTMLInputElement>()
const fileStatus = reactive({ input: '', output: '' })
const importing = ref(false)
const appending = ref(false)
const appendError = ref('')
const { showToast, dismissToast } = useToast()
const locked = computed(() => props.disabled || importing.value || appending.value)
async function appendSamples() {
  if (locked.value) return
  const samples = cases.value.filter(item => item.isSample).map(item => ({ ...item }))
  if (!samples.length) return
  dismissToast()
  appending.value = true
  appendError.value = ''
  try {
    for (const item of samples) {
      for (const key of ['input', 'output'] as const) {
        const file = item[`${key}File`]
        if (file && !item[`_${key}Dirty`] && !item[key]) {
          if (!props.problemId) throw new Error('サンプルの読み込みに失敗しました。もう一度お試しください。')
          item[key] = await downloadTestFile(props.problemId, file)
        }
      }
    }
    if (!mounted || props.disabled) return
    let number = 0
    for (const match of markdown.value.matchAll(/^## サンプル (\d+)\s*$/gm)) number = Math.max(number, Number(match[1]))
    const block = (value: string) => {
      let length = 3
      for (const match of value.matchAll(/`+/g)) length = Math.max(length, match[0].length + 1)
      const fence = '`'.repeat(length)
      return `${fence}text\n${value}${value && !value.endsWith('\n') ? '\n' : ''}${fence}`
    }
    const content = samples.map(item => `## サンプル ${++number}\n\n### 入力\n${block(item.input)}\n\n### 出力\n${block(item.output)}\n`).join('\n')
    const next = markdown.value + (markdown.value ? '\n\n' : '') + content
    if (next.length > 100_000) throw new Error('追加すると問題文が100,000文字を超えます。サンプルを小さくしてください。')
    markdown.value = next
    showToast(`${samples.length}件のサンプルを問題文の末尾に追加しました。`)
  } catch (error) {
    appendError.value = error instanceof Error && error.message.includes('100,000') ? error.message : 'サンプルの読み込みに失敗しました。もう一度お試しください。'
  } finally {
    appending.value = false
  }
}
const importError = ref('')
const importStatus = ref('')
const importDialog = ref<HTMLDialogElement>()
const dialogId = useId()
function openImport() {
  if (locked.value) return
  importError.value = ''
  importStatus.value = ''
  importDialog.value?.showModal()
}
let mounted = true
onBeforeUnmount(() => { mounted = false; cancelAnimationFrame(scrollFrame) })
async function selectFiles(event: Event, key: 'input' | 'output') {
  const control = event.target as HTMLInputElement
  const files = Array.from(control.files ?? [])
  control.value = ''
  if (locked.value || !files.length) return
  importing.value = true
  importError.value = ''
  importStatus.value = ''
  try {
    const imported = await importTestCaseFiles(files, key, cases.value)
    if (!mounted) return
    if (props.disabled) throw new Error('現在は取り込めません。もう一度選択してください。')
    const merged = mergeTestCases(cases.value, imported, [key])
    const validation = testCaseError(merged)
    if (validation) throw new Error(validation)
    const added = merged.length - cases.value.length
    cases.value = merged
    selected.value = merged.findIndex(item => item.name?.trim() === imported[0]!.name?.trim())
    fileStatus[key] = `${files.length}ファイル取り込み済み`
    importStatus.value = `${key === 'input' ? '入力' : '出力'}：${added}件追加、${imported.length - added}件更新しました。`
  } catch (error) {
    importError.value = error instanceof Error ? error.message : 'ファイルを読み込めませんでした。'
  } finally {
    importing.value = false
  }
}
const current = computed(() => cases.value[selected.value])
const loading = reactive({ input: false, output: false })
const loadError = reactive({ input: '', output: '' })
const fileName = (item: TestCase, index: number) => item.name?.trim() || `ケース${index + 1}`
watch(() => cases.value.length, length => { selected.value = Math.max(0, Math.min(selected.value, length - 1)) })
async function load(key: 'input' | 'output') {
  const item = current.value
  const file = item?.[`${key}File`]
  if (!item || !file || item[`_${key}Dirty`] || item[key] || !props.problemId || loading[key]) return
  loading[key] = true
  loadError[key] = ''
  try {
    const value = await downloadTestFile(props.problemId, file)
    if (cases.value.includes(item) && item[`${key}File`]?.id === file.id && !item[`_${key}Dirty`]) item[key] = value
  } catch {
    loadError[key] = 'テストデータを読み込めませんでした。'
  } finally {
    loading[key] = false
    if (current.value !== item) void load(key)
  }
}
watch([selected, () => current.value?.inputFile?.id, () => current.value?.outputFile?.id], () => { void load('input'); void load('output') }, { immediate: true })
function remove() {
  if (current.value && window.confirm(`「${fileName(current.value, selected.value)}」の入力と出力を削除しますか？`)) cases.value.splice(selected.value, 1)
}
// New cases join no group: the readiness names them until the author decides (ADR 0025).
const ungrouped = (item: TestCase) => !!props.scoring && !item.isSample && !item.groups?.length
// Membership follows the groups' order, whichever way the chips were clicked.
function toggleGroup(name: string, member: boolean) {
  const item = current.value
  if (!item || !props.scoring) return
  const groups = new Set(item.groups ?? [])
  if (member) groups.add(name)
  else groups.delete(name)
  item.groups = props.scoring.groups.map(group => group.name).filter(value => groups.has(value))
}
// The saved order is the judging order, so authors can move samples back to the top.
const list = ref<HTMLUListElement>()
const moveStatus = ref('')
function move(from: number, to: number) {
  if (locked.value || from === to) return
  const open = cases.value[selected.value]
  const [item] = cases.value.splice(from, 1)
  cases.value.splice(to, 0, item!)
  selected.value = cases.value.indexOf(open!)
  moveStatus.value = `${fileName(item!, to)}を${to + 1}番目に移動しました。`
}
async function moveByKey(event: KeyboardEvent, index: number) {
  const to = index + (event.key === 'ArrowUp' ? -1 : event.key === 'ArrowDown' ? 1 : 0)
  if (to === index) return
  event.preventDefault()
  if (to < 0 || to >= cases.value.length) return
  move(index, to)
  await nextTick()
  list.value?.querySelectorAll<HTMLButtonElement>('.case-grip')[to]?.focus()
}
// Pointer events cover mouse and touch; the handle's touch-action stops a touch drag from scrolling instead.
// slot is the gap the case drops into: 0 is above the first row and cases.length below the last.
const drag = ref<{ from: number, slot: number, y: number, offset: number, left: number, width: number }>()
let scrollFrame = 0
const dropSlot = computed(() => drag.value && drag.value.slot !== drag.value.from && drag.value.slot !== drag.value.from + 1 ? drag.value.slot : -1)
function slotAt(y: number) {
  const rows = Array.from(list.value?.children ?? [])
  const index = rows.findIndex(row => { const box = row.getBoundingClientRect(); return y < box.top + box.height / 2 })
  return index < 0 ? rows.length : index
}
function startDrag(event: PointerEvent, index: number) {
  if (locked.value || event.button !== 0) return
  event.preventDefault()
  const handle = event.currentTarget as HTMLElement
  handle.setPointerCapture(event.pointerId)
  const row = handle.parentElement!.getBoundingClientRect()
  drag.value = { from: index, slot: index, y: event.clientY, offset: event.clientY - row.top, left: row.left, width: row.width }
  scrollFrame = requestAnimationFrame(scrollEdge)
}
function moveDrag(event: PointerEvent) {
  if (!drag.value) return
  drag.value.y = event.clientY
  drag.value.slot = slotAt(event.clientY)
}
function endDrag(drop: boolean) {
  if (!drag.value) return
  const { from, slot } = drag.value
  drag.value = undefined
  cancelAnimationFrame(scrollFrame)
  if (drop) move(from, slot > from ? slot - 1 : slot)
}
// Holding a case near the top or bottom of the list scrolls it, so a short list on a phone reaches every row.
function scrollEdge() {
  const element = list.value
  if (!drag.value || !element) return
  const box = element.getBoundingClientRect()
  const step = drag.value.y < box.top + 24 ? -8 : drag.value.y > box.bottom - 24 ? 8 : 0
  if (step) {
    element.scrollTop += step
    drag.value.slot = slotAt(drag.value.y)
  }
  scrollFrame = requestAnimationFrame(scrollEdge)
}
function add() {
  if (locked.value || cases.value.length >= 100) return
  const used = new Set(cases.value.map(item => item.name?.trim()))
  let number = cases.value.length + 1
  while (used.has(`case_${String(number).padStart(3, '0')}.txt`)) number++
  cases.value.push({ name: `case_${String(number).padStart(3, '0')}.txt`, input: '', output: '' })
  selected.value = cases.value.length - 1
}
</script>

<template>
  <section class="test-case-editor" aria-labelledby="test-cases-title">
    <header class="case-toolbar">
      <div class="case-heading">
        <h1 id="test-cases-title">テストケース <span>{{ cases.length }} / 100件</span></h1>
        <p class="case-limits">入出力は各16 MiB・合計512 MiBまで
          <span class="case-info" tabindex="0" role="img" aria-label="保存と公開について" :aria-describedby="`${dialogId}-info`">
            <svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="9" /><path d="M12 11v5M12 8h.01" /></svg>
            <span :id="`${dialogId}-info`" class="case-info-tooltip" role="tooltip">公開中の問題は「保存」で変更が採点に反映されます。入出力は公開ページには表示されません。</span>
          </span>
        </p>
      </div>
      <button type="button" class="editor-button" :disabled="locked || !cases.some(item => item.isSample)" @click="appendSamples">{{ appending ? 'サンプルを読み込み中…' : 'サンプルを問題文に追加' }}</button>
    </header>
    <p v-if="error" class="editor-error" role="alert">{{ error }}</p>
    <p v-if="appendError" class="editor-error" role="alert">{{ appendError }}</p>
    <div class="case-workspace">
      <nav class="case-files" aria-label="テストケース一覧">
        <div class="file-list-heading"><span>テストケース名</span>
          <div class="file-list-actions">
            <button type="button" class="editor-button primary case-add" :disabled="locked || cases.length >= 100" aria-label="テストケースを追加" @click="add"><span aria-hidden="true">＋</span> 追加</button>
            <button type="button" class="editor-button case-import" :disabled="locked" aria-label="ファイルから一括追加" title="ファイルから一括追加" @click="openImport"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M12 15V4m0 0L8 8m4-4 4 4M4 15v3a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-3" /></svg></button>
          </div>
        </div>
        <ul ref="list">
          <li v-for="(item, index) in cases" :key="index" :class="{ 'case-dragging': drag?.from === index }">
            <span v-if="dropSlot === index" class="case-drop" aria-hidden="true" />
            <button type="button" class="case-grip" :aria-label="`${fileName(item, index)}を移動`" title="ドラッグか ↑↓ キーで並べ替え" :disabled="locked" @pointerdown="startDrag($event, index)" @pointermove="moveDrag" @pointerup="endDrag(true)" @pointercancel="endDrag(false)" @keydown="moveByKey($event, index)"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><circle cx="9" cy="6" r="1.5" /><circle cx="15" cy="6" r="1.5" /><circle cx="9" cy="12" r="1.5" /><circle cx="15" cy="12" r="1.5" /><circle cx="9" cy="18" r="1.5" /><circle cx="15" cy="18" r="1.5" /></svg></button>
            <button type="button" class="case-select" :aria-current="selected === index ? 'true' : undefined" :disabled="locked" :title="fileName(item, index)" @click="selected = index">
              <svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M14 2H5v20h14V7Zm0 0v5h5M8 12h8M8 16h6" /></svg>
              <span>{{ fileName(item, index) }}</span><span v-if="ungrouped(item)" class="case-ungrouped" title="どのグループにも属していません">無所属</span><svg v-if="item.isSample" class="sample-flag" viewBox="0 0 24 24" role="img" aria-label="サンプルケース"><path d="M5 21V3m0 1c5-4 9 4 14 0v10c-5 4-9-4-14 0" /></svg>
            </button>
            <span v-if="dropSlot === cases.length && index === cases.length - 1" class="case-drop case-drop-end" aria-hidden="true" />
          </li>
        </ul>
        <div v-if="drag" class="case-drag-ghost" aria-hidden="true" :style="{ top: `${drag.y - drag.offset}px`, left: `${drag.left + 24}px`, width: `${drag.width - 24}px` }">{{ fileName(cases[drag.from]!, drag.from) }}</div>
        <p class="visually-hidden" role="status">{{ moveStatus }}</p>
      </nav>
      <div v-if="current" class="case-detail">
        <div class="case-name">
          <label :for="`case-name-${selected}`">テストケース名</label>
          <input :id="`case-name-${selected}`" v-model="current.name" :aria-label="`テストケース名 ${selected + 1}`" :placeholder="`ケース${selected + 1}`" :disabled="locked" autocomplete="off" title="64文字まで。入力と出力に同じ名前を使用します。" />
          <label class="sample-option"><input v-model="current.isSample" type="checkbox" :disabled="locked">サンプルケースにする</label>
          <button type="button" class="editor-button case-delete" :disabled="locked" :aria-label="`ケース${selected + 1}を削除`" title="選択中のケースの入力と出力を削除" @click="remove"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M3 6h18M9 6V3h6v3M5 6l1 15h12l1-15M10 10v7M14 10v7" /></svg>削除</button>
        </div>
        <fieldset v-if="scoring" class="case-groups" :disabled="locked">
          <legend>所属するグループ</legend>
          <label v-for="group in scoring.groups" :key="group.name" class="group-chip"><input type="checkbox" :checked="current.groups?.includes(group.name) ?? false" @change="toggleGroup(group.name, ($event.target as HTMLInputElement).checked)">{{ group.name }}</label>
          <span v-if="!scoring.groups.length" class="case-groups-note">グループは「採点」で追加します。</span>
          <span v-else-if="ungrouped(current)" class="case-groups-note case-groups-missing">サンプル以外のケースは、どれかのグループに入れてください。</span>
          <button type="button" class="case-groups-link" @click="emit('show-scoring')">グループを編集</button>
        </fieldset>
        <div :key="selected" class="case-fields">
          <TestDataEditor :id="`case-input-${selected}`" v-model="current.input" label="入力" :disabled="locked" :stored-bytes="current.inputFile?.size" :loading="loading.input" :error="loadError.input" @update:model-value="current._inputDirty = true" />
          <TestDataEditor :id="`case-output-${selected}`" v-model="current.output" label="出力" :disabled="locked" :stored-bytes="current.outputFile?.size" :loading="loading.output" :error="loadError.output" @update:model-value="current._outputDirty = true" />
        </div>
      </div>
      <div v-else class="empty-editor">
        <EmptyState kind="testcase" title="テストケースはまだありません。" description="1件ずつ入力するか、.txt ファイルからまとめて取り込めます。空の入力や出力も登録できます。">
          <button type="button" class="editor-button primary" :disabled="locked" @click="add"><span aria-hidden="true">＋</span> 追加</button>
          <button type="button" class="editor-button" :disabled="locked" @click="openImport">ファイルから一括追加</button>
        </EmptyState>
      </div>
    </div>
    <dialog ref="importDialog" class="leave-dialog bulk-import" :aria-labelledby="`${dialogId}-title`" :aria-describedby="`${dialogId}-description`" @cancel="importing && $event.preventDefault()">
      <h2 :id="`${dialogId}-title`">ファイルから一括追加</h2>
      <p :id="`${dialogId}-description`">.txt を複数選択できます。ファイル名がテストケース名になり、入力と出力は同じ名前どうしで組になります。同じ名前のケースがあるときは、選んだ側だけを更新します。</p>
      <div class="import-pickers">
        <div class="import-picker">
          <span class="import-picker-label">入力</span>
          <button type="button" class="editor-button" :disabled="locked" @click="inputFiles?.click()">入力ファイルを選択</button>
          <span class="import-file-status">{{ fileStatus.input || '未選択' }}</span>
          <input ref="inputFiles" hidden type="file" accept=".txt,text/plain" multiple aria-label="入力ファイル" :disabled="locked" @change="selectFiles($event, 'input')">
        </div>
        <div class="import-picker">
          <span class="import-picker-label">出力</span>
          <button type="button" class="editor-button" :disabled="locked" @click="outputFiles?.click()">出力ファイルを選択</button>
          <span class="import-file-status">{{ fileStatus.output || '未選択' }}</span>
          <input ref="outputFiles" hidden type="file" accept=".txt,text/plain" multiple aria-label="出力ファイル" :disabled="locked" @change="selectFiles($event, 'output')">
        </div>
      </div>
      <p class="import-note">UTF-8 のテキストに限ります。入出力は各16 MiB、全体で512 MiBまでです。</p>
      <p v-if="importing || importStatus" role="status">{{ importing ? '取り込み中…' : importStatus }}</p>
      <p v-if="importError" class="editor-error" role="alert">{{ importError }}</p>
      <div class="leave-dialog-actions"><button type="button" class="editor-button" :disabled="importing" @click="importDialog?.close()">閉じる</button></div>
    </dialog>
  </section>
</template>

<style scoped>
/* Hallmark · component scope · existing ShareOJ tokens · reference: shared file list + paired editors
 * pre-emit critique: P5 H4 E4 S5 R5 V4 */
.test-case-editor { display: flex; flex-direction: column; flex: 1; min-height: 0; min-width: 0; }
.case-toolbar { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 12px; padding: 10px 16px; border-bottom: 1px solid var(--color-line); }
.case-heading { position: relative; display: flex; flex-wrap: wrap; align-items: baseline; gap: 4px 16px; min-width: 0; }
.case-toolbar h1 { margin: 0; font-size: 1rem; }
.case-toolbar h1 span { margin-left: 12px; font-size: .75rem; font-weight: normal; color: var(--color-muted); }
.case-toolbar button { white-space: nowrap; }
.case-limits { display: inline-flex; align-items: center; gap: 4px; margin: 0; color: var(--color-muted); font-size: .75rem; }
.case-info { display: inline-flex; align-items: center; justify-content: center; width: 24px; height: 24px; border-radius: 4px; cursor: help; }
.case-info .editor-icon { width: 16px; height: 16px; }
.case-info:hover, .case-info:focus-visible { color: var(--color-accent); }
.case-info:focus-visible { outline: 2px solid var(--color-accent); outline-offset: 1px; }
.case-info-tooltip { display: none; position: absolute; top: calc(100% + 6px); left: 0; right: 0; z-index: 3; max-width: 22rem; padding: 8px 12px; border-radius: 4px; background: var(--color-ink); color: var(--color-paper); font-size: .75rem; line-height: 1.7; }
.case-info:hover .case-info-tooltip, .case-info:focus .case-info-tooltip { display: block; }
.case-message { margin: 0; padding: 8px 16px; font-size: .75rem; color: var(--color-muted); border-bottom: 1px solid var(--color-line); }
.bulk-import { word-break: auto-phrase; }
.import-pickers { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; }
.import-picker { display: flex; flex-direction: column; align-items: center; gap: 8px; padding: 16px 12px; border: 1px dashed var(--color-line); border-radius: 6px; background: var(--color-surface); text-align: center; }
.import-picker-label { font-size: .8125rem; font-weight: 600; }
.import-file-status { color: var(--color-muted); font-size: .75rem; }
.bulk-import .import-note { margin: 12px 0 0; color: var(--color-muted); font-size: .75rem; }
.bulk-import [role=status] { margin: 12px 0 0; font-size: .8125rem; }
.bulk-import .editor-error { margin: 12px 0 0; }
@media (max-width: 420px) { .import-pickers { grid-template-columns: minmax(0, 1fr); } }
.case-workspace { display: grid; grid-template-columns: 220px minmax(0, 1fr); flex: 1; min-height: 0; }
.case-files { display: flex; flex-direction: column; min-height: 0; min-width: 0; border-right: 1px solid var(--color-line); }
.file-list-heading { display: flex; align-items: center; justify-content: space-between; gap: 8px; padding: 8px 10px; font-size: .75rem; border-bottom: 1px solid var(--color-line); background: var(--color-surface); }
.file-list-actions { display: flex; flex-shrink: 0; gap: 4px; }
.case-add { flex-shrink: 0; font-weight: 600; }
.case-import { display: inline-flex; align-items: center; justify-content: center; width: var(--editor-control-size); padding: 0; }
.case-files ul { list-style: none; margin: 0; padding: 4px 0; overflow-y: auto; }
.case-files li { position: relative; display: flex; margin: 0; border-left: 3px solid transparent; }
.case-files li:has(> .case-select:hover), .case-files li:has(> .case-select[aria-current]) { background: var(--color-accent-soft); }
.case-files li:has(> .case-select[aria-current]) { border-left-color: var(--color-accent); }
.case-files li:has(> .case-select:active) { background: var(--color-line); }
.case-select { display: flex; align-items: center; gap: 8px; flex: 1; min-width: 0; padding: 10px 12px 10px 4px; background: transparent; border: 0; color: var(--color-ink); text-align: left; font-family: var(--font-code); font-size: .8125rem; cursor: pointer; }
.case-select span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.case-select svg { flex-shrink: 0; }
.case-select:hover, .case-select[aria-current] { color: var(--color-accent); }
.case-select:focus-visible, .case-grip:focus-visible { outline: 2px solid var(--color-accent); outline-offset: -2px; }
.case-select:disabled, .case-grip:disabled { opacity: .5; cursor: not-allowed; }
.case-grip { display: inline-flex; align-items: center; justify-content: center; flex-shrink: 0; width: 28px; padding: 0; background: transparent; border: 0; color: var(--color-muted); cursor: grab; touch-action: none; }
.case-grip .editor-icon { fill: currentColor; stroke: none; }
.case-grip:not(:disabled):hover { color: var(--color-ink); }
.case-dragging { opacity: .4; }
.case-drop { position: absolute; z-index: 11; left: -3px; right: 0; top: -1px; height: 2px; background: var(--color-accent); pointer-events: none; }
.case-drop-end { top: auto; bottom: -1px; }
.case-drag-ghost { position: fixed; z-index: 10; box-sizing: border-box; padding: 10px 12px; border: 1px solid var(--color-line); border-radius: 4px; background: var(--color-paper); box-shadow: 0 6px 18px rgb(0 0 0 / .18); color: var(--color-ink); font-family: var(--font-code); font-size: .8125rem; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; pointer-events: none; cursor: grabbing; }
.case-detail { display: flex; flex-direction: column; min-height: 0; min-width: 0; }
.case-name { background: var(--color-surface); display: flex; align-items: center; gap: 12px; padding: 8px 12px; border-bottom: 1px solid var(--color-line); }
.case-name label { margin: 0; font-size: .75rem; white-space: nowrap; }
.case-name input:not([type="checkbox"]) { flex: 1; min-width: 0; max-width: 32rem; box-sizing: border-box; padding: 6px 8px; border: 1px solid var(--color-line); border-radius: 4px; background: var(--color-paper); color: var(--color-ink); font-family: var(--font-code); font-size: .8125rem; }
.sample-flag { --color-sample-flag: #2463d4; width: 18px; height: 18px; margin-left: auto; flex-shrink: 0; fill: var(--color-sample-flag); stroke: var(--color-sample-flag); stroke-width: 1.5; stroke-linejoin: round; }
.case-name .sample-option { display: inline-flex; align-items: center; gap: 6px; width: auto; cursor: pointer; }
.sample-option input { accent-color: var(--color-accent); width: 16px; height: 16px; }
.case-name { flex-wrap: wrap; }
.case-name button { white-space: nowrap; }
.case-delete { display: inline-flex; align-items: center; gap: 6px; flex-shrink: 0; color: var(--color-error); border-color: var(--color-error); }
.case-delete:hover:not(:disabled) { background: var(--color-error); color: var(--color-paper); }
.case-delete:focus-visible { outline-color: var(--color-error); }
.case-delete:active:not(:disabled) { transform: translateY(1px); }
.case-delete:disabled, .case-add:disabled, .case-import:disabled { opacity: .5; cursor: not-allowed; }
@media (pointer: coarse) { .case-delete, .case-add { min-height: 44px; } .case-import { width: 44px; min-height: 44px; } .case-grip { width: 44px; } }
.case-select .case-ungrouped { flex-shrink: 0; margin-left: auto; padding: 0 4px; border: 1px solid var(--color-error); border-radius: 3px; color: var(--color-error); font-size: .6875rem; line-height: 1.5; overflow: visible; }
.case-groups { display: flex; flex-wrap: wrap; align-items: center; gap: 6px 8px; margin: 0; padding: 8px 12px; border: 0; border-bottom: 1px solid var(--color-line); min-width: 0; }
.case-groups legend { float: left; margin-right: 4px; padding: 0; font-size: .75rem; }
.group-chip { position: relative; display: inline-flex; align-items: center; min-height: 32px; padding: 2px 12px; border: 1px solid var(--color-line); border-radius: 999px; background: var(--color-paper); font-size: .8125rem; cursor: pointer; overflow-wrap: anywhere; }
.group-chip input { position: absolute; inset: 0; width: 100%; height: 100%; margin: 0; opacity: 0; cursor: pointer; }
.group-chip:has(input:checked) { border-color: var(--color-accent); background: var(--color-accent-soft); color: var(--color-accent); font-weight: 600; }
.group-chip:has(input:checked)::before { content: '✓'; margin-right: 4px; }
.group-chip:has(input:focus-visible) { outline: 2px solid var(--color-accent); outline-offset: 2px; }
.case-groups:disabled .group-chip { opacity: .5; cursor: not-allowed; }
.case-groups-note { color: var(--color-muted); font-size: .75rem; }
.case-groups-missing { color: var(--color-error); }
.case-groups-link { margin-left: auto; padding: 4px 2px; border: 0; background: none; color: var(--color-accent); font: inherit; font-size: .75rem; text-decoration: underline; cursor: pointer; }
@media (hover: hover) { .group-chip:not(:has(input:checked)):hover { background: var(--color-surface); } }
@media (pointer: coarse) { .group-chip, .case-groups-link { min-height: 44px; } .case-groups-link { padding-inline: 8px; } }
.case-fields { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); flex: 1; min-height: 0; }
.empty-editor { display: flex; flex-direction: column; padding: 16px; overflow-y: auto; }
.empty-editor > * { margin-block: auto; }
@media (max-height: 32rem) { .empty-editor > * { margin-block: 0 auto; } }
@media (min-width: 601px) and (max-width: 900px) { .file-list-heading { justify-content: flex-end; } .file-list-heading > span { display: none; } }
@media (max-width: 900px) { .case-workspace { grid-template-columns: 160px minmax(0, 1fr); } .file-list-heading { padding: 8px; } .case-name { flex-wrap: wrap; gap: 6px; } .case-name label { width: 100%; } }
@media (max-width: 600px), (max-height: 32rem) {
  .test-case-editor { display: block; overflow-y: auto; }
  .case-workspace { min-height: 400px; }
}
@media (max-width: 600px) {
  .case-name input:not([type="checkbox"]) { flex-basis: 100%; max-width: none; }
  .case-name .sample-option { flex: 1; }
  .case-toolbar { padding: 8px; }
  .case-toolbar h1 span { margin-left: 4px; }
  .case-message { padding: 6px 8px; }
  .case-workspace { grid-template-columns: minmax(0, 1fr); grid-template-rows: auto auto; min-height: 0; }
  .case-files { border-right: 0; border-bottom: 1px solid var(--color-line); }
  .file-list-heading { padding: 4px 8px; }
  .case-files ul { padding: 0; max-height: 132px; }
  .case-select { padding: 6px 8px 6px 4px; min-height: 44px; }
  .case-fields { flex: none; grid-template-columns: minmax(0, 1fr); grid-template-rows: repeat(2, 320px); }
}
</style>
