<script setup lang="ts">
import { testCaseError, type TestCase } from '~/utils/problem-draft'
import { downloadTestFile } from '~/utils/test-files'
import { importTestCaseFiles, mergeTestCases } from '~/utils/import-test-cases'
const cases = defineModel<TestCase[]>({ required: true })
const markdown = defineModel<string>('markdown', { required: true })
const props = defineProps<{ disabled: boolean, problemId?: string }>()
const error = computed(() => testCaseError(cases.value))
const selected = ref(0)
const inputFiles = ref<HTMLInputElement>()
const outputFiles = ref<HTMLInputElement>()
const fileStatus = reactive({ input: '', output: '' })
const importing = ref(false)
const appending = ref(false)
const appendError = ref('')
const appendStatus = ref('')
const locked = computed(() => props.disabled || importing.value || appending.value)
async function appendSamples() {
  if (locked.value) return
  const samples = cases.value.filter(item => item.isSample).map(item => ({ ...item }))
  if (!samples.length) return
  appending.value = true
  appendError.value = ''; appendStatus.value = ''
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
    appendStatus.value = `${samples.length}件のサンプルを問題文の末尾に追加しました。`
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
onBeforeUnmount(() => { mounted = false })
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
            <span :id="`${dialogId}-info`" class="case-info-tooltip" role="tooltip">変更は自動保存され、「公開する／公開内容を更新」で採点に反映されます。入出力は公開ページには表示されません。</span>
          </span>
        </p>
      </div>
      <button type="button" class="editor-button" :disabled="locked || !cases.some(item => item.isSample)" @click="appendSamples">{{ appending ? 'サンプルを読み込み中…' : 'サンプルを問題文に追加' }}</button>
    </header>
    <p v-if="error" class="editor-error" role="alert">{{ error }}</p>
    <p v-if="appendError" class="editor-error" role="alert">{{ appendError }}</p>
    <p v-if="appendStatus" class="case-message" role="status">{{ appendStatus }}</p>
    <div class="case-workspace">
      <nav class="case-files" aria-label="テストケース一覧">
        <div class="file-list-heading"><span>テストケース名</span>
          <div class="file-list-actions">
            <button type="button" class="editor-button primary case-add" :disabled="locked || cases.length >= 100" aria-label="テストケースを追加" @click="add"><span aria-hidden="true">＋</span> 追加</button>
            <button type="button" class="editor-button case-import" :disabled="locked" aria-label="ファイルから一括追加" title="ファイルから一括追加" @click="openImport"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M12 15V4m0 0L8 8m4-4 4 4M4 15v3a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-3" /></svg></button>
          </div>
        </div>
        <ul>
          <li v-for="(item, index) in cases" :key="index">
            <button type="button" :aria-current="selected === index ? 'true' : undefined" :disabled="locked" :title="fileName(item, index)" @click="selected = index">
              <svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M14 2H5v20h14V7Zm0 0v5h5M8 12h8M8 16h6" /></svg>
              <span>{{ fileName(item, index) }}</span><svg v-if="item.isSample" class="sample-flag" viewBox="0 0 24 24" role="img" aria-label="サンプルケース"><path d="M5 21V3m0 1c5-4 9 4 14 0v10c-5 4-9-4-14 0" /></svg>
            </button>
          </li>
        </ul>
      </nav>
      <div v-if="current" class="case-detail">
        <div class="case-name">
          <label :for="`case-name-${selected}`">テストケース名</label>
          <input :id="`case-name-${selected}`" v-model="current.name" :aria-label="`テストケース名 ${selected + 1}`" :placeholder="`ケース${selected + 1}`" :disabled="locked" autocomplete="off" title="64文字まで。入力と出力に同じ名前を使用します。" />
          <label class="sample-option"><input v-model="current.isSample" type="checkbox" :disabled="locked">サンプルケースにする</label>
          <button type="button" class="editor-button case-delete" :disabled="locked" :aria-label="`ケース${selected + 1}を削除`" title="選択中のケースの入力と出力を削除" @click="remove"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M3 6h18M9 6V3h6v3M5 6l1 15h12l1-15M10 10v7M14 10v7" /></svg>削除</button>
        </div>
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
.case-files li { margin: 0; }
.case-files li button { display: flex; align-items: center; gap: 8px; width: 100%; min-width: 0; padding: 10px 12px; background: transparent; border: 0; border-left: 3px solid transparent; color: var(--color-ink); text-align: left; font-family: var(--font-code); font-size: .8125rem; cursor: pointer; }
.case-files li button span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.case-files li button svg { flex-shrink: 0; }
.case-files li button:hover, .case-files li button[aria-current] { background: var(--color-accent-soft); color: var(--color-accent); }
.case-files li button[aria-current] { border-left-color: var(--color-accent); }
.case-files li button:focus-visible { outline: 2px solid var(--color-accent); outline-offset: -2px; }
.case-files li button:active { background: var(--color-line); }
.case-files li button:disabled { opacity: .5; cursor: not-allowed; }
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
@media (pointer: coarse) { .case-delete, .case-add { min-height: 44px; } .case-import { width: 44px; min-height: 44px; } }
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
  .case-files li button { padding: 6px 8px; min-height: 44px; }
  .case-fields { flex: none; grid-template-columns: minmax(0, 1fr); grid-template-rows: repeat(2, 320px); }
}
</style>
