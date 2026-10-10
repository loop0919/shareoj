<script setup lang="ts">
import type { Submission } from '../../shared/types/submission'
import { testFileSchema, type Generators } from '~~/shared/types/problem-draft'
import { testCaseError, persistedDraft, type TestCase } from '~/utils/problem-draft'

const cases = defineModel<TestCase[]>({ required: true })
const config = defineModel<Generators>('config', { required: true })
const busy = defineModel<boolean>('busy', { required: true })
const judging = defineModel<boolean>('judging', { default: false })
const props = defineProps<{ disabled: boolean, problemId?: string, save: () => Promise<boolean> }>()
const emit = defineEmits<{ 'show-cases': [] }>()
const mode = ref<'input' | 'output' | 'validation'>('input')
const start = ref(1)
const count = ref(1)
const message = ref('')
const failure = ref('')
const validationResults = ref<{ name: string, verdict: string }[]>([])
const modeLabel = computed(() => ({ input: '入力生成', output: '出力生成', validation: '入力検証' })[mode.value])
const { data: catalog, error: catalogError, refresh: refreshCatalog } = useJudgeCatalog()
const available = computed(() => catalogError.value || catalog.value?.maintenance ? [] : catalog.value?.items ?? [])
const program = computed(() => config.value[mode.value])
watch([mode, start, count, () => JSON.stringify(program.value), () => JSON.stringify(persistedDraft({ testCases: cases.value }))], () => {
  if (validationResults.value.length) { validationResults.value = []; message.value = '' }
}, { flush: 'sync' })
let disposed = false
onBeforeUnmount(() => { disposed = true })
watch(mode, () => { start.value = 1; count.value = 1; failure.value = ''; message.value = '' })
function selectMode(next: typeof mode.value) {
  judging.value = false
  mode.value = next
}

async function generate() {
  if (busy.value || props.disabled || !available.value.length) return
  failure.value = ''
  message.value = ''
  validationResults.value = []
  const first = mode.value === 'input' ? Number(start.value) : 1
  const amount = mode.value === 'input' ? Number(count.value) : cases.value.length
  if (mode.value !== 'input' && amount === 0) {
    failure.value = 'テストケースを追加してから実行してください。'
    return
  }
  if (!Number.isInteger(first) || !Number.isInteger(amount) || amount < 1 || amount > 100 || first < -2147483648 || first + amount - 1 > 2147483647) {
    failure.value = '開始番号は32ビット整数、件数は1〜100の整数で指定してください。'
    return
  }
  if (mode.value === 'input' && cases.value.length + amount > 100) {
    failure.value = '追加後のテストケースは100件以内にしてください。'
    return
  }
  if (!program.value.source.trim() || new TextEncoder().encode(program.value.source).length > 65536 || program.value.source.includes('\0')) {
    failure.value = 'ソースコードを1〜65,536バイトで入力してください。'
    return
  }
  if (!available.value.some(item => item.id === program.value.runtime)) {
    failure.value = '利用できる言語を選択してください。'
    return
  }
  if (mode.value === 'output' && !window.confirm(`全${amount}件のテストケースの出力を生成結果で上書きしますか？`)) return
  busy.value = true
  try {
    message.value = '下書きを保存しています…'
    if (!await props.save() || !props.problemId) throw new Error('下書きを保存してから再実行してください。')
    const fingerprint = () => JSON.stringify(persistedDraft({ testCases: cases.value }))
    const snapshot = fingerprint()
    let result = await $fetch<Submission>('/api/my/submissions', {
      method: 'POST', body: { problemId: props.problemId, ...program.value, generation: { mode: mode.value, start: first, count: amount } },
    })
    const deadline = Date.now() + 60 * 60 * 1000
    while (!disposed && result.status !== 'DONE') {
      message.value = result.status === 'QUEUED' ? '実行待ちです…' : `${mode.value === 'validation' ? '検証' : '生成'}中… ${result.progress?.completed ?? 0} / ${amount}件`
      if (Date.now() > deadline) throw new Error('結果の確認がタイムアウトしました。')
      await new Promise(resolve => setTimeout(resolve, 1500))
      if (disposed) return
      result = await $fetch<Submission>(`/api/my/submissions/${result.id}`)
    }
    if (disposed) return
    if (mode.value === 'validation') {
      if (fingerprint() !== snapshot) throw new Error('実行中にテストケースが変更されました。再実行してください。')
      if (!result.result || ['CE', 'JE'].includes(result.result.verdict)) throw new Error(`検証を完了できませんでした（${result.result?.verdict ?? 'JE'}）。${result.result?.compileLog ?? ''}`)
      const results = result.result.cases
      if (results?.length !== amount) throw new Error('検証結果が不完全です。')
      validationResults.value = results.map((item, index) => ({ name: cases.value[first - 1 + index]?.name || `ケース ${first + index}`, verdict: item.verdict }))
      message.value = `${amount}件中${results.filter(item => item.verdict === 'AC').length}件が合格しました。`
      return
    }
    if (result.result?.verdict !== 'AC') throw new Error(`生成に失敗しました（${result.result?.verdict ?? 'JE'}）。${result.result?.compileLog ?? ''}`)
    const outputs = result.result.cases
    if (outputs?.length !== amount || outputs.some(item => item.outputFile ? item.output !== undefined : typeof item.output !== 'string')) throw new Error('生成結果が不完全です。')
    if (fingerprint() !== snapshot) throw new Error('実行中にテストケースが変更されたため、結果は反映していません。再実行してください。')
    const updated = cases.value.map(item => ({ ...item }))
    if (mode.value === 'input') {
      const names = new Set(updated.map(item => item.name?.trim()))
      for (const [index, generated] of outputs.entries()) {
        const base = `generated_${first + index}`
        let name = `${base}.txt`, suffix = 1
        while (names.has(name)) name = `${base}_${suffix++}.txt`
        names.add(name)
        updated.push({ name, input: generated.output ?? '', inputFile: generated.outputFile ? testFileSchema.parse(generated.outputFile) : undefined, output: '' })
      }
    } else {
      outputs.forEach((generated, index) => {
        const item = updated[first - 1 + index]!
        item.output = generated.output ?? ''
        item.outputFile = generated.outputFile ? testFileSchema.parse(generated.outputFile) : undefined
        item._outputDirty = !item.outputFile
      })
    }
    const error = testCaseError(updated)
    if (error) throw new Error(error)
    message.value = '生成ファイルを検証しています…'
    for (const generated of outputs) {
      if (!generated.outputFile) continue
      const file = testFileSchema.parse(generated.outputFile)
      const completed = testFileSchema.parse(await $fetch(`/api/my/problems/${props.problemId}/test-files/${file.id}/complete`, { method: 'POST' }))
      if (completed.id !== file.id || completed.size !== file.size || completed.sha256 !== file.sha256) throw new Error('生成ファイルが一致しません。')
      if (disposed) return
    }
    if (fingerprint() !== snapshot) throw new Error('実行中にテストケースが変更されたため、結果は反映していません。再実行してください。')
    cases.value = updated
    message.value = `${amount}件の${mode.value === 'input' ? '入力を追加' : '出力を更新'}しました。`
  } catch (error) {
    message.value = ''
    const detail = (error as { data?: { data?: { code?: string, retryAfter?: number } } }).data?.data
    if (detail?.code === 'judge_maintenance') { failure.value = judgeMaintenanceMessage; await refreshCatalog() }
    else if (detail?.code === 'submission_rate_limited') {
      failure.value = detail.retryAfter
        ? `提出頻度制限に到達しました。${detail.retryAfter}秒後に再度試してください。`
        : '提出頻度制限に到達しました。しばらく待ってから再度試してください。'
    } else failure.value = error instanceof Error && !('statusCode' in error) ? error.message : '実行できませんでした。ログイン状態と実行環境を確認してください。'
  } finally { busy.value = false }
}
</script>

<template>
  <section class="generator" aria-labelledby="generator-title">
    <header class="generator-heading">
      <div><h1 id="generator-title">生成・検証・判定</h1><p>コードでテストケースの入出力を生成・検証し、提出の判定方法を設定します。</p></div>
      <NuxtLink to="/blog/generator-guide" target="_blank" rel="noopener noreferrer">入出力生成と入力検証の使い方 ↗</NuxtLink>
    </header>
    <div class="generator-panel">
      <div class="problem-menu" role="group" aria-label="生成・検証・判定の種類">
        <button type="button" :aria-pressed="!judging && mode === 'input'" :disabled="disabled || (busy && mode !== 'input')" @click="selectMode('input')">入力生成</button>
        <button type="button" :aria-pressed="!judging && mode === 'output'" :disabled="disabled || (busy && mode !== 'output')" @click="selectMode('output')">出力生成</button>
        <button type="button" :aria-pressed="!judging && mode === 'validation'" :disabled="disabled || (busy && mode !== 'validation')" @click="selectMode('validation')">入力検証</button>
        <button type="button" :aria-pressed="judging" :disabled="disabled" @click="judging = true">判定方法</button>
      </div>
      <div v-show="judging"><slot name="checker" /></div>
      <div v-show="!judging">
        <div class="generator-options">
          <label>言語<select v-model="program.runtime" :disabled="disabled || busy"><option v-for="item in available" :key="item.id" :value="item.id">{{ item.label }}</option></select></label>
          <label v-if="mode === 'input'">開始ケース番号<input v-model="start" type="number" step="1" :disabled="disabled || busy"></label>
          <label v-if="mode === 'input'">生成件数<input v-model="count" type="number" min="1" max="100" step="1" :disabled="disabled || busy"></label>
        </div>
        <dl class="code-spec" :aria-label="`${modeLabel}の仕様`">
          <template v-if="mode === 'input'">
            <div><dt>標準入力</dt><dd>ケース番号（整数1個）</dd></div>
            <div><dt>標準出力</dt><dd>新しいテストケースの入力として追加します</dd></div>
          </template>
          <template v-else>
            <div><dt>標準入力</dt><dd>各テストケースの入力（全{{ cases.length }}件）</dd></div>
            <div v-if="mode === 'output'"><dt>標準出力</dt><dd>そのケースの出力を置き換えます</dd></div>
            <template v-else>
              <div><dt>標準出力</dt><dd>保存しません。テストケースは変更しません。</dd></div>
              <div><dt>結果</dt><dd>終了コード0で合格、それ以外で不合格</dd></div>
            </template>
          </template>
          <div v-if="mode !== 'validation'"><dt>制限</dt><dd>生成するファイルは各16 MiB、全体で512 MiBまで</dd></div>
        </dl>
        <SourceCodeEditor v-model="program.source" :runtime="program.runtime" :label="`${modeLabel}のコード`" :disabled="disabled || busy" :key="mode" />
        <p v-if="failure" class="notice notice-error" role="alert">{{ failure }}</p>
        <div class="generator-actions"><button type="button" class="editor-button primary" :disabled="disabled || busy || !available.length" @click="generate">{{ mode === 'validation' ? (busy ? '検証中…' : '検証する') : (busy ? '生成中…' : '生成する') }}</button><button type="button" class="editor-button" @click="emit('show-cases')">テストケースを確認</button></div>
        <p v-if="message" role="status">{{ message }}</p>
        <table v-if="validationResults.length" class="validation-results" aria-label="入力検証の結果">
          <thead><tr><th>テストケース</th><th>検証結果</th></tr></thead>
          <tbody><tr v-for="(item, index) in validationResults" :key="index"><td>{{ item.name }}</td><td>{{ item.verdict === 'AC' ? '合格' : item.verdict === 'RE' ? '不合格（終了コード・異常終了）' : `検証未完了（${item.verdict}）` }}</td></tr></tbody>
        </table>
      </div>
    </div>
  </section>
</template>

<style scoped>
.validation-results { width: 100%; max-width: 1000px; border-collapse: collapse; font-size: .8125rem; overflow-wrap: anywhere; table-layout: fixed; }
.validation-results th, .validation-results td { text-align: left; padding: 10px; border-bottom: 1px solid var(--color-line); }
.generator { flex: 1; min-height: 0; min-width: 0; overflow-y: auto; padding: 24px; }
.generator-heading { display: flex; flex-wrap: wrap; align-items: start; justify-content: space-between; gap: 16px; }
.generator-heading h1 { margin: 0; font-size: 1.25rem; }
.generator-heading a { font-size: .8125rem; padding-block: 4px; }
.generator-panel { max-width: 1000px; margin-top: 20px; }
.generator-actions { display: flex; flex-wrap: wrap; gap: 12px; }
.generator-actions button { min-height: 40px; }
@media (max-width: 600px) { .generator { padding: 16px 12px; } }
.generator p { margin: 8px 0; font-size: .8125rem; white-space: pre-wrap; overflow-wrap: anywhere; }
.generator-options { display: flex; flex-wrap: wrap; gap: 12px; margin-top: 12px; }
.generator-options label { display: flex; flex-direction: column; gap: 4px; font-size: .75rem; max-width: 100%; }
.generator-options input, .generator-options select { box-sizing: border-box; width: 100%; max-width: 240px; min-height: 36px; padding: 4px 8px; border: 1px solid var(--color-line); border-radius: 4px; background: var(--color-paper); color: var(--color-ink); }
.generator-panel :deep(.source-code-editor) { margin: 12px 0; }
.generator-panel :deep(.code-surface) { height: clamp(240px, 42vh, 520px); }
button:disabled { opacity: .5; cursor: not-allowed; }
</style>
