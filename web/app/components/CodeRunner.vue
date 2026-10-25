<script setup lang="ts">
import { codeRunInputLimit, type CodeRun } from '~~/shared/types/code-run'
import { usableRuntimes } from '~/utils/judge-catalog'
import { hasSource } from '~/utils/has-source'
import { executionTime, memoryUsage } from '~/utils/submission-usage'

const { user } = useAccount()
const route = useRoute()
const source = ref('')
const stdin = ref('')
// The largest source the API accepts, in UTF-8 bytes.
const sourceLimit = 131072
const stdinBytes = computed(() => new TextEncoder().encode(stdin.value).length)
const runtime = ref('cpp23-gcc')
const { data: catalog, error: catalogError, refresh: refreshCatalog } = await useJudgeCatalog()
const available = computed(() => usableRuntimes(catalog.value, catalogError.value))
// One draft shared by /run and the contest page; the runtime falls back to the submission form's choice.
const draftKey = 'openoj.code-run-draft'
const submissionRuntimeKey = 'openoj.submission-runtime'
let restored = false
// The language the user chose, kept even while no language is offered (maintenance), so the draft never loses it.
let savedRuntime = ''
onMounted(() => {
  try {
    const draft = JSON.parse(localStorage.getItem(draftKey) ?? 'null') as { runtime?: unknown, source?: unknown, stdin?: unknown } | null
    if (typeof draft?.source === 'string') source.value = draft.source
    if (typeof draft?.stdin === 'string') stdin.value = draft.stdin
    const draftRuntime = typeof draft?.runtime === 'string' ? draft.runtime : ''
    savedRuntime = draftRuntime
    const usable = [draftRuntime, localStorage.getItem(submissionRuntimeKey) ?? ''].find(id => id && available.value.some(item => item.id === id))
    if (usable) { runtime.value = usable; savedRuntime = usable }
  } catch { /* Storage may be disabled by the browser, or hold a broken draft. */ }
  restored = true
  // The layout resolves the account after hydration, so wait for it before reading the last run.
  const stop = watch(user, account => {
    if (!account) return
    void nextTick(() => stop())
    void restoreLatest()
  }, { immediate: true })
})
watch([runtime, source, stdin], () => {
  if (!restored) return
  if (runtime.value) savedRuntime = runtime.value
  try { localStorage.setItem(draftKey, JSON.stringify({ runtime: savedRuntime, source: source.value, stdin: stdin.value })) }
  catch { /* Keep running usable without storage. */ }
})
watch(available, items => {
  if (items.some(item => item.id === runtime.value)) return
  // Coming back from maintenance, return to the saved language before falling back to the first one.
  if (runtime.value || items.length) runtime.value = items.find(item => item.id === savedRuntime)?.id ?? items[0]?.id ?? ''
}, { immediate: true })

const run = ref<CodeRun | null>(null)
const sending = ref(false)
const message = ref('')
const stalled = ref(false)
const pending = computed(() => sending.value || (run.value !== null && run.value.status !== 'DONE' && !stalled.value))
let disposed = false
// Each follow() call takes a new number, so an older loop stops when a newer run starts.
let following = 0
onBeforeUnmount(() => { disposed = true })

// Polls the latest run every 1.5 s until it is DONE or an hour passes.
async function follow() {
  const mine = ++following
  const deadline = Date.now() + 60 * 60 * 1000
  while (!disposed && mine === following && run.value && run.value.status !== 'DONE') {
    if (Date.now() > deadline) throw new Error('timeout')
    await new Promise(resolve => setTimeout(resolve, 1500))
    if (disposed || mine !== following) return
    const latest = await $fetch<CodeRun>('/api/my/code-runs/latest')
    if (disposed || mine !== following) return
    run.value = latest
  }
}
// Shows the last run when the screen opens, e.g. after coming back while it was still running.
async function restoreLatest() {
  const seen = following
  try {
    const latest = await $fetch<CodeRun>('/api/my/code-runs/latest')
    // A run started meanwhile is newer than this answer, and its polling must not be stopped.
    if (disposed || sending.value || seen !== following) return
    run.value = latest
  } catch { return /* No run yet, or it could not be read; running again still works. */ }
  try { await follow() }
  catch { stalled.value = true; message.value = failedMessage }
}

async function execute() {
  if (sending.value || pending.value || !available.value.some(item => item.id === runtime.value)) return
  if (!hasSource(runtime.value, source.value) || new TextEncoder().encode(source.value).length > sourceLimit) {
    message.value = `ソースコードを1〜${sourceLimit.toLocaleString('en-US')}バイトで入力してください。`
    return
  }
  if (new TextEncoder().encode(stdin.value).length > codeRunInputLimit) {
    message.value = `標準入力は${codeRunInputLimit.toLocaleString('en-US')}バイト以内で入力してください。`
    return
  }
  sending.value = true
  message.value = ''
  stalled.value = false
  ++following
  try {
    run.value = await $fetch<CodeRun>('/api/my/code-runs', { method: 'POST', body: { runtime: runtime.value, source: source.value, stdin: stdin.value } })
    sending.value = false
    await follow()
  } catch (error) {
    const failure = error as { statusCode?: number, data?: { data?: { code?: string, retryAfter?: number } } }
    const code = failure.data?.data?.code
    if (code === 'judge_maintenance') { message.value = judgeMaintenanceMessage; await refreshCatalog() }
    else if (failure.statusCode === 401) message.value = 'コードを実行するにはログインしてください。入力したコードはこの画面に残っています。'
    else if (code === 'code_run_rate_limited') message.value = failure.data?.data?.retryAfter
      ? `コード実行の頻度制限に到達しました。${failure.data.data.retryAfter}秒後に再度試してください。`
      : 'コード実行の頻度制限に到達しました。しばらく待ってから再度試してください。'
    else if (code === 'profile_required') message.value = 'プロフィールを登録してからコードを実行してください。'
    else if (code === 'judging_unavailable') message.value = 'ジャッジが設定されていません。'
    else if (code === 'invalid_code_run') message.value = '実行内容を受け付けられませんでした。言語を選び直し、ソースコードと標準入力にNUL文字などの使えない文字がないか確認してください。'
    else message.value = failedMessage
    // A failed read leaves the run unfinished; stop treating it as running so the button works again.
    stalled.value = true
  } finally { sending.value = false }
}

const failedMessage = '実行結果を確認できませんでした。時間をおいてもう一度実行してください。'
const ran = ['AC', 'RE', 'TLE', 'MLE', 'OLE']
const result = computed(() => run.value?.status === 'DONE' ? run.value.result : null)
const executed = computed(() => !!result.value && !result.value.expired && ran.includes(result.value.verdict))
const compileError = computed(() => result.value?.verdict === 'CE')
</script>

<template>
  <section v-if="!user" class="code-run-login" aria-labelledby="code-run-login-title">
    <h2 id="code-run-login-title">ログインしてコードを実行</h2>
    <p>コード実行には、ログインが必要です。</p>
    <NuxtLink class="editor-button primary" :to="{ path: '/login', query: { next: route.fullPath } }">ログインする</NuxtLink>
  </section>
  <form v-else class="code-runner" @submit.prevent="execute">
    <!-- The language and the run button share the top row, so input and output can stay together below (as on Codeforces). -->
    <div class="code-runner-toolbar">
      <div class="code-runner-language">
        <label for="code-run-language">言語</label>
        <div class="code-runner-language-row">
          <select id="code-run-language" v-model="runtime" :disabled="pending">
            <option value="">-- 未選択 --</option>
            <option v-for="item in available" :key="item.id" :value="item.id">{{ item.label }}</option>
          </select>
          <NuxtLink to="/docs/languages" target="_blank" rel="noopener noreferrer">使える言語と実行環境の仕様 ↗</NuxtLink>
        </div>
      </div>
      <button class="editor-button primary code-run-button" type="submit" :disabled="pending || !hasSource(runtime, source) || !runtime || !available.length" :aria-busy="pending"><LoadingSpinner v-if="pending" inline label="実行中" />実行</button>
    </div>
    <div v-if="catalogError || !available.length || message" class="code-runner-notices">
      <p v-if="catalogError || !available.length" class="notice" role="status">現在、コード実行の受付を停止しています。</p>
      <p v-if="message" class="notice notice-error" role="alert">{{ message }}</p>
    </div>
    <div class="code-runner-source">
      <SourceCodeEditor v-model="source" :runtime="runtime" :limit="sourceLimit" :disabled="sending" />
    </div>
    <div class="code-runner-side">
      <!-- Framed like the source editor, so the two boxes start on the same line. -->
      <div class="io-box stdin-box" :class="{ 'is-disabled': sending }">
        <div class="io-heading">
          <label for="code-run-stdin">標準入力</label>
          <span class="byte-count">{{ stdinBytes.toLocaleString('en-US') }} / {{ codeRunInputLimit.toLocaleString('en-US') }} bytes</span>
        </div>
        <textarea id="code-run-stdin" v-model="stdin" rows="8" spellcheck="false" :disabled="sending" />
      </div>
      <section class="io-box" aria-labelledby="code-run-stdout-title">
        <div class="io-heading">
          <h2 id="code-run-stdout-title">標準出力</h2>
          <!-- The verdict and the measurements stay one line, in the heading of the output they describe. -->
          <span v-if="run && !result?.expired" class="run-status" role="status">
            <SubmissionStatus :item="run" />
            <span v-if="executed">終了コード {{ result!.exitCode ?? '—' }} ・ {{ executionTime(result!.cpuTimeMs) }} ・ {{ memoryUsage(result!.memoryBytes) }}</span>
          </span>
        </div>
        <div class="io-body">
          <p v-if="!run" class="muted">実行すると、ここに出力が表示されます。</p>
          <p v-else-if="result?.expired" role="status">混雑のため実行できませんでした。時間をおいてもう一度実行してください。</p>
          <p v-else-if="result?.verdict === 'JE'">ジャッジで問題が起きたため、実行できませんでした。時間をおいてもう一度実行してください。</p>
          <p v-else-if="compileError" class="muted">コンパイルエラーのため実行していません。</p>
          <template v-else-if="executed">
            <pre v-if="result!.stdout?.text">{{ result!.stdout.text }}</pre>
            <p v-else class="muted">（出力なし）</p>
          </template>
        </div>
      </section>
      <p v-if="executed && result!.stdout?.truncated" class="muted io-note">長いため、先頭の64 KiBだけを表示しています。</p>
      <!-- A compile error runs nothing, so this box shows the compiler's message instead of the program's errors. -->
      <section class="io-box" aria-labelledby="code-run-stderr-title">
        <div class="io-heading"><h2 id="code-run-stderr-title">{{ compileError ? 'コンパイルのメッセージ' : '標準エラー' }}</h2></div>
        <div class="io-body">
          <template v-if="compileError">
            <pre v-if="result!.compileLog">{{ result!.compileLog }}</pre>
            <p v-else class="muted">コンパイルのメッセージはありません。</p>
          </template>
          <template v-else-if="executed">
            <pre v-if="result!.stderr?.text">{{ result!.stderr.text }}</pre>
            <p v-else class="muted">（出力なし）</p>
          </template>
        </div>
      </section>
      <p v-if="executed && result!.stderr?.truncated" class="muted io-note">長いため、先頭の64 KiBだけを表示しています。</p>
    </div>
  </form>
</template>

<style scoped>
.code-run-login { margin-top: 40px; padding: 24px; border: 1px solid var(--color-line); border-radius: 4px; background: var(--color-surface); }
.code-run-login h2 { margin-bottom: 8px; }
.code-run-login p { color: var(--color-muted); font-size: .875rem; }
.code-run-login a { display: inline-flex; align-items: center; min-height: 44px; padding: 10px 24px; font-weight: 600; text-decoration: none; }
.code-runner { display: grid; grid-template-columns: minmax(0, 1fr); gap: 0 32px; margin-top: 24px; }
/* Wide screens: the source on the left; input, output and errors on the right (ADR 0031). */
@media (min-width: 60rem) { .code-runner { grid-template-columns: minmax(0, 3fr) minmax(0, 2fr); align-items: start; } }
/* The top row spans both columns, so the source and input boxes below it start on the same line. */
.code-runner-toolbar, .code-runner-notices { grid-column: 1 / -1; }
.code-runner-toolbar { display: flex; flex-wrap: wrap; align-items: flex-end; justify-content: space-between; gap: 12px 24px; }
.code-runner-toolbar, .code-runner-language, .code-runner-source, .code-runner-side { min-width: 0; }
.code-runner-language > label { display: block; margin-bottom: 8px; }
.code-runner-language-row { display: flex; flex-wrap: wrap; align-items: center; gap: 8px 16px; }
.code-runner-notices { margin-top: 16px; }
select { width: 320px; max-width: 100%; min-height: 44px; padding: 8px 12px; border: 1px solid var(--color-line); border-radius: 4px; background: var(--color-paper); color: var(--color-ink); font: inherit; cursor: pointer; }
select:focus-visible { outline: 3px solid var(--color-accent); outline-offset: 3px; }
select:disabled, button:disabled { opacity: .5; cursor: not-allowed; }
.code-run-button { display: inline-flex; align-items: center; gap: 8px; min-height: 44px; margin-left: auto; padding: 10px 28px; font-weight: 600; }
.code-run-button:disabled { transform: none; text-decoration: none; }
/* The same frame and heading height as SourceCodeEditor, whose heading holds a 32px settings button. */
.io-box { margin-top: 20px; border: 1px solid var(--color-line); border-radius: 4px; overflow: hidden; }
.io-box + .io-box, .io-note + .io-box { margin-top: 12px; }
.stdin-box.is-disabled { opacity: .6; }
.io-heading { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 8px; min-height: 49px; padding: 8px 12px; border-bottom: 1px solid var(--color-line); font-size: .875rem; }
.io-heading label, .io-heading h2 { margin: 0; font-family: inherit; font-size: .6875rem; font-weight: 400; line-height: 1.5; letter-spacing: normal; color: var(--color-muted); }
.byte-count, .run-status { font-size: .75rem; color: var(--color-muted); }
.run-status { display: inline-flex; flex-wrap: wrap; align-items: center; gap: 4px 8px; }
textarea { display: block; width: 100%; min-height: 160px; padding: 8px 12px; border: 0; background: var(--color-paper); color: var(--color-ink); font: .875rem/1.5 var(--font-mono, ui-monospace, monospace); resize: vertical; }
/* Inside the frame, so the ring is drawn inward like the source editor's. */
textarea:focus-visible { outline: 2px solid var(--color-accent); outline-offset: -2px; }
.io-body { min-height: 64px; }
.io-body > p { margin: 0; padding: 8px 12px; font-size: .875rem; }
/* The frame already draws the box, so the output itself needs no second border. */
.io-body pre { max-height: 20rem; overflow: auto; padding: 8px 12px; border: 0; border-radius: 0; background: none; font-size: .875rem; line-height: 1.5; white-space: pre-wrap; overflow-wrap: anywhere; }
.io-note { margin: 4px 0 0; font-size: .75rem; }
</style>
