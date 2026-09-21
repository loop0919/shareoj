<script setup lang="ts">
import type { Submission } from '../../shared/types/submission'

const props = defineProps<{ problemId: string, hasSamples: boolean, contestId?: string, beforeSubmit?: () => Promise<boolean>, disabled?: boolean }>()
const { user } = useAccount()
const sampleHelpId = useId()
const source = ref('')
const runtime = ref('cpp17')
const { data: catalog, error: catalogError, refresh: refreshCatalog } = await useJudgeCatalog()
const available = computed(() => catalogError.value || catalog.value?.maintenance ? [] : catalog.value?.items ?? [])
const runtimeStorageKey = 'openoj.submission-runtime'
onMounted(() => {
  try {
    const saved = localStorage.getItem(runtimeStorageKey)
    if (saved !== null && (saved === '' || available.value.some(item => item.id === saved))) runtime.value = saved
  } catch { /* Storage may be disabled by the browser. */ }
})
function rememberRuntime() {
  try { localStorage.setItem(runtimeStorageKey, runtime.value) }
  catch { /* Keep language selection usable without storage. */ }
}
watch(available, items => {
  if (runtime.value && !items.some(item => item.id === runtime.value)) runtime.value = items[0]?.id ?? ''
}, { immediate: true })
const sending = ref(false)
const runningEasyTest = ref(false)
const message = ref('')
const easyResult = ref<Submission | null>(null)
let disposed = false
onBeforeUnmount(() => { disposed = true })
watch([source, runtime, () => props.problemId], () => { easyResult.value = null })
async function submit(easyTest = false) {
  if ((easyTest && !props.hasSamples) || props.disabled || sending.value || !available.value.some(item => item.id === runtime.value)) return
  if (!source.value.trim() || new TextEncoder().encode(source.value).length > 65536) {
    message.value = 'ソースコードを1〜65,536バイトで入力してください。'
    return
  }
  runningEasyTest.value = easyTest
  sending.value = true
  message.value = ''
  easyResult.value = null
  try {
    if (props.beforeSubmit && !await props.beforeSubmit()) {
      message.value = '下書きを保存できませんでした。保存内容を確認してください。'
      return
    }
    let result = await $fetch<Submission>('/api/my/submissions', { method: 'POST', body: { contestId: props.contestId, problemId: props.problemId, runtime: runtime.value, source: source.value, easyTest: easyTest || undefined } })
    if (easyTest) {
      easyResult.value = result
      const deadline = Date.now() + 60 * 60 * 1000
      while (!disposed && result.status !== 'DONE') {
        if (Date.now() > deadline) throw new Error('timeout')
        await new Promise(resolve => setTimeout(resolve, 1500))
        if (disposed) return
        result = await $fetch<Submission>(`/api/my/submissions/${result.id}`)
        if (disposed) return
        easyResult.value = result
      }
      return
    }
    await navigateTo(`/my/submissions/${result.id}`)
  } catch (error) {
    const failure = error as { statusCode?: number, data?: { data?: { code?: string, retryAfter?: number } } }
    const code = failure.data?.data?.code
    if (code === 'judge_maintenance') { message.value = judgeMaintenanceMessage; await refreshCatalog() }
    else if (failure.statusCode === 401) message.value = '提出するにはログインしてください。入力したコードはこの画面に残っています。'
    else if (code === 'submission_rate_limited') message.value = failure.data?.data?.retryAfter
      ? `提出頻度制限に到達しました。${failure.data.data.retryAfter}秒後に再度試してください。`
      : '提出頻度制限に到達しました。しばらく待ってから再度試してください。'
    else if (code === 'profile_required') message.value = 'プロフィールを登録してから提出してください。'
    else if (code === 'tests_not_ready') message.value = easyTest ? '「サンプルケースにする」をチェックしたケースがあることと、検証コード・言語の設定を確認してください。' : 'テストケース、検証コード、利用できる言語の設定を確認してください。'
    else if (code === 'judging_unavailable') message.value = 'ジャッジが設定されていません。'
    else message.value = easyTest ? 'サンプル検証の結果を確認できませんでした。再実行するか、結果の詳細を確認してください。' : '提出を確認できませんでした。再送する前に提出履歴を確認してください。'
  } finally { sending.value = false }
}
</script>

<template>
  <section v-if="!user" class="submission-login" aria-labelledby="submission-login-title">
    <h2 id="submission-login-title">ログインして解答を提出</h2>
    <p>解答の提出や採点結果の確認には、ログインが必要です。</p>
    <NuxtLink class="editor-button primary" to="/login">ログインする</NuxtLink>
  </section>
  <section v-else class="submission-form" aria-labelledby="submission-title">
    <h2 id="submission-title">提出</h2>
    <p class="muted">ソースコードは64 KiBまで</p>
    <form @submit.prevent="submit()">
      <label for="submission-language">言語</label>
      <select id="submission-language" v-model="runtime" :disabled="sending" @change="rememberRuntime">
        <option value="">-- 未選択 --</option>
        <option v-for="item in available" :key="item.id" :value="item.id">{{ item.label }}</option>
      </select>
      <p><NuxtLink to="/blog/language-guide" target="_blank" rel="noopener noreferrer">使える言語と実行環境の仕様 ↗</NuxtLink></p>
      <p v-if="catalogError || !available.length" class="notice" role="status">現在、提出受付を停止しています。</p>
      <SourceCodeEditor v-model="source" :runtime="runtime" :disabled="sending" />
      <p v-if="message" class="notice notice-error" role="alert">{{ message }}</p>
      <div class="submission-actions">
        <div class="sample-action" :class="{ 'no-samples': !hasSamples }">
          <button class="editor-button" type="button" :aria-describedby="sampleHelpId" :disabled="!hasSamples || disabled || sending || !source.trim() || !runtime || !available.length" @click="submit(true)">{{ sending && runningEasyTest ? 'サンプル検証中…' : 'サンプル検証' }}</button>
          <span class="sample-help">
            <button class="sample-help-button" type="button" aria-label="サンプル検証の説明" :aria-describedby="sampleHelpId">?</button>
            <span :id="sampleHelpId" class="sample-tooltip" role="tooltip">{{ hasSamples ? 'サンプルケースを検証する機能です。' : 'この問題は利用可能なサンプルがありません' }}</span>
          </span>
        </div>
        <button class="editor-button primary" type="submit" :disabled="disabled || sending || !source.trim() || !runtime || !available.length" :aria-busy="sending">{{ sending && !runningEasyTest ? '提出中…' : '提出する' }}</button>
      </div>
    </form>
    <section v-if="easyResult" class="easy-result" aria-labelledby="easy-result-title">
      <h3 id="easy-result-title">サンプル検証の結果</h3>
      <p role="status"><SubmissionStatus :item="easyResult" /><template v-if="easyResult.result"> — {{ easyResult.result.passed }} / {{ easyResult.result.total }} ケース合格</template></p>
      <SampleCaseResults v-if="easyResult.result?.cases?.length" :cases="easyResult.result.cases" :interactive="easyResult.result.interactive" />
      <section v-if="easyResult.result?.interactive && easyResult.result.checkerLog && !easyResult.result.cases?.length">
        <h4>ジャッジコードの診断</h4><pre>{{ easyResult.result.checkerLog }}</pre>
      </section>
      <pre v-if="easyResult.result?.compileLog">{{ easyResult.result.compileLog }}</pre>
      <NuxtLink :to="`/my/submissions/${easyResult.id}`" target="_blank" rel="noopener noreferrer">結果の詳細 ↗</NuxtLink>
    </section>
  </section>
</template>

<style scoped>
.sample-action { display: inline-flex; align-items: flex-end; gap: 8px; }
.sample-help { position: relative; display: inline-flex; }
.sample-help-button { display: inline-flex; align-items: center; justify-content: center; flex: none; appearance: none; width: 20px; height: 20px; padding: 0; border: 1px solid var(--color-muted); border-radius: 50%; background: transparent; color: var(--color-muted); font: inherit; font-size: 12px; line-height: 1; cursor: help; }
.sample-help-button:focus-visible { outline: 2px solid var(--color-accent); outline-offset: 3px; }
.sample-tooltip { display: none; position: absolute; top: 100%; left: 50%; transform: translateX(-50%); width: max-content; max-width: 240px; padding: 6px 8px; border-radius: 4px; background: var(--color-ink); color: var(--color-paper); font-size: .75rem; z-index: 1; }
.sample-action.no-samples:hover .sample-tooltip, .sample-help:hover .sample-tooltip, .sample-help:focus-within .sample-tooltip { display: block; }
.easy-result { margin-top: 24px; }
.easy-result pre { white-space: pre-wrap; overflow-wrap: anywhere; }
.submission-form { margin-top: 40px; }
.submission-login { margin-top: 40px; padding: 24px; border: 1px solid var(--color-line); border-radius: 4px; background: var(--color-surface); }
.submission-login h2 { margin-bottom: 8px; }
.submission-login p { color: var(--color-muted); font-size: .875rem; }
.submission-login a { display: inline-flex; align-items: center; min-height: 44px; padding: 10px 24px; font-weight: 600; text-decoration: none; }
label { display: block; margin-block: 20px 8px; }
select { width: 100%; max-width: 320px; min-height: 44px; padding: 8px 12px; border: 1px solid var(--color-line); border-radius: 4px; background: var(--color-paper); color: var(--color-ink); font: inherit; cursor: pointer; }
select:focus-visible { outline: 3px solid var(--color-accent); outline-offset: 3px; }
select:disabled, .submission-actions button:disabled { opacity: .5; cursor: not-allowed; }
.submission-actions .editor-button { min-height: 44px; padding: 10px 28px; font-weight: 600; }
.submission-actions button:disabled { transform: none; text-decoration: none; }
.submission-actions { display: flex; align-items: center; flex-wrap: wrap; gap: 20px; margin-top: 16px; }
</style>
