<script setup lang="ts">
import { executionTime, memoryUsage } from '~/utils/submission-usage'
import { runtimeLabel } from '~/utils/runtime-label'
import { submissionProblemPath, type Submission } from '~~/shared/types/submission'
const props = defineProps<{ item: Submission }>()
const item = toRef(props, 'item')
const expanded = ref(false)
const copyMessage = ref('')
const copying = ref(false)
const codeBytes = computed(() => item.value?.source === undefined ? null : new TextEncoder().encode(item.value.source).length)
async function copySource() {
  if (copying.value || item.value?.source === undefined) return
  copying.value = true
  copyMessage.value = ''
  try { await navigator.clipboard.writeText(item.value.source); copyMessage.value = 'コピーしました。' }
  catch { copyMessage.value = 'コピーできませんでした。コードを選択してコピーしてください。' }
  finally { copying.value = false }
}
const verdicts: Record<string, string> = { AC: '正解', WA: '不正解', CE: 'コンパイルエラー', RE: '実行時エラー', TLE: '実行時間超過', MLE: 'メモリ超過', OLE: '出力超過', JE: '採点できませんでした' }
</script>

<template>
  <section class="submission-detail">
    <header class="submission-header"><h1>{{ item.easyTest ? 'サンプル検証の結果' : '提出結果' }}</h1><p class="submission-id">ID: {{ item.id }}</p></header>
    <section class="submitted-source" :class="{ expanded }" aria-labelledby="source-title">
      <div class="section-heading">
        <h2 id="source-title">ソースコード</h2>
        <div v-if="item.source !== undefined" class="source-actions">
          <button class="editor-button" :aria-expanded="expanded" aria-controls="submitted-code" @click="expanded = !expanded">{{ expanded ? '折りたたむ' : '広げる' }}</button>
          <button class="editor-button" :disabled="copying" @click="copySource">{{ copying ? 'コピー中…' : 'コピー' }}</button>
        </div>
      </div>
      <SourceCodeEditor v-if="item.source !== undefined" id="submitted-code" :model-value="item.source" :runtime="item.runtime" readonly />
      <p v-else class="muted">ソースコードを取得できませんでした。</p>
      <p v-if="copyMessage" class="copy-message" role="status">{{ copyMessage }}</p>
    </section>
    <section aria-labelledby="submission-info-title">
      <h2 id="submission-info-title">提出情報</h2>
      <div class="submission-info">
        <table aria-label="提出情報">
          <tbody>
            <tr><th scope="row">提出日時</th><td><time :datetime="item.createdAt">{{ new Date(item.createdAt).toLocaleString('ja-JP') }}</time></td></tr>
            <tr v-if="item.author"><th scope="row">提出者</th><td><UserLink :handle="item.author" /></td></tr>
            <tr><th scope="row">問題</th><td><NuxtLink :to="submissionProblemPath(item)">{{ item.problemTitle }}</NuxtLink></td></tr>
            <tr><th scope="row">言語</th><td>{{ runtimeLabel(item.runtime) }}</td></tr>
            <tr><th scope="row">コード長</th><td>{{ codeBytes === null ? '—' : `${codeBytes.toLocaleString('en-US')} bytes` }}</td></tr>
            <tr><th scope="row" title="各テストケースの最大CPU時間">実行時間</th><td>{{ executionTime(item.result?.cpuTimeMs) }}</td></tr>
            <tr><th scope="row" title="各テストケースの最大メモリ使用量">メモリ使用量</th><td>{{ memoryUsage(item.result?.memoryBytes) }}</td></tr>
            <tr><th scope="row">結果</th><td><span role="status" aria-live="polite"><SubmissionStatus :item="item" /><template v-if="item.result">：{{ verdicts[item.result.verdict] ?? '' }}</template></span></td></tr>
            <tr><th scope="row">正解したケース</th><td>{{ item.result ? `${item.result.passed} / ${item.result.total}` : '—' }}</td></tr>
          </tbody>
        </table>
      </div>
    </section>
    <section class="case-results" aria-labelledby="case-results-title">
      <h2 id="case-results-title">ジャッジ結果</h2>
      <div v-if="item.result?.cases?.length" class="submission-info">
        <table aria-label="テストケースごとの結果">
          <thead><tr><th scope="col">テストケース名</th><th scope="col">結果</th><th scope="col">CPU時間</th><th scope="col">経過時間</th><th scope="col">最大メモリ</th></tr></thead>
          <tbody>
            <tr v-for="(testCase, index) in item.result.cases" :key="index">
              <td>{{ testCase.name }}</td>
              <td><span class="verdict-badge" :data-verdict="testCase.verdict === 'SKIPPED' ? undefined : testCase.verdict" :title="verdicts[testCase.verdict]">{{ testCase.verdict === 'SKIPPED' ? '未実行' : testCase.verdict }}</span></td>
              <td>{{ testCase.cpuTimeMs == null ? '—' : `${testCase.cpuTimeMs} ms` }}</td>
              <td>{{ testCase.wallTimeMs == null ? '—' : `${testCase.wallTimeMs} ms` }}</td>
              <td>{{ memoryUsage(testCase.memoryBytes) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <p v-else class="muted">{{ item.status === 'DONE' ? 'この提出にはテストケースごとの結果が記録されていません。' : '採点が完了すると、テストケースごとの結果を表示します。' }}</p>
    </section>
    <SampleCaseResults v-if="item.easyTest && item.result?.cases?.length" :cases="item.result.cases" :interactive="item.result.interactive" />
    <section v-if="item.result?.checkerLog && !(item.easyTest && item.result.interactive && item.result.cases?.length)" class="compile-log" aria-labelledby="checker-log-title"><h2 id="checker-log-title">{{ item.easyTest && item.result.interactive ? 'ジャッジコードの診断' : 'ジャッジコードの診断（作問者のみ）' }}</h2><pre>{{ item.result.checkerLog }}</pre></section>
    <section v-if="item.result?.compileLog" class="compile-log" aria-labelledby="compile-title"><h2 id="compile-title">コンパイル診断</h2><pre>{{ item.result.compileLog }}</pre></section>
  </section>
</template>

<style scoped>
/* Existing Plain theme: source viewer followed by striped submission metadata. */
.submission-detail { padding-bottom: 64px; }
.submission-header { padding-bottom: 24px; margin-bottom: 28px; border-bottom: 1px solid var(--color-line); }
.submission-header h1 { margin: 0 0 8px; }
.submission-id { margin: 0; color: var(--color-muted); font-family: var(--font-code); font-size: .8125rem; overflow-wrap: anywhere; }
.submitted-source { margin-bottom: 28px; }
.section-heading, .source-actions { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; }
.section-heading { justify-content: space-between; margin-bottom: 12px; }
.section-heading h2 { margin: 0; }
.source-actions button { min-height: 36px; padding-inline: 12px; }
.source-actions button:disabled { opacity: .5; cursor: not-allowed; transform: none; }
.submitted-source :deep(.source-code-editor) { margin-top: 0; }
.expanded :deep(.cm-scroller) { max-height: none; }
.copy-message { margin-top: 8px; font-size: .8125rem; }
.submission-info { border: 1px solid var(--color-line); border-radius: 4px; overflow: hidden; }
.submission-info table { width: 100%; border-collapse: collapse; table-layout: fixed; font-size: .875rem; }
.submission-info th, .submission-info td { padding: 12px 16px; border-bottom: 1px solid var(--color-line); overflow-wrap: anywhere; }
.submission-info th { width: 30%; text-align: left; border-right: 1px solid var(--color-line); font-weight: 600; }
.submission-info td { text-align: center; font-variant-numeric: tabular-nums; }
.submission-info tr:nth-child(odd) { background: var(--color-surface); }
.submission-info tbody tr:last-child > * { border-bottom: 0; }
.compile-log, .case-results { margin-top: 28px; }
.case-results th { width: auto; text-align: center; }
.case-results td + td { border-left: 1px solid var(--color-line); }
.case-results thead { background: var(--color-surface); }
.compile-log pre { max-width: 100%; white-space: pre-wrap; overflow-wrap: anywhere; }
@media (max-width: 480px) { .submission-info th, .submission-info td { padding: 10px; } .submission-info th { width: 35%; } }
</style>
