<script setup lang="ts">
import type { ProblemDraft } from '~~/shared/types/problem-draft'

const checker = defineModel<ProblemDraft['checker']>({ required: true })
const interactor = defineModel<ProblemDraft['interactor']>('interactor', { required: true })
const props = defineProps<{ disabled: boolean, problemId: string, published: boolean, publishedVersion: number, hasSamples: boolean, save: () => Promise<boolean> }>()
const { data: publishedProblem } = await useAsyncData(
  () => `checker-samples-${props.problemId}-${props.publishedVersion}`,
  () => props.published && props.problemId ? $fetch(`/api/problems/${props.problemId}`) : Promise.resolve(null),
)
const { data: catalog, error: catalogError } = useJudgeCatalog()
const available = computed(() => catalogError.value || catalog.value?.maintenance ? [] : catalog.value?.items ?? [])
let previousCode: ProblemDraft['checker'] = null
const method = computed({
  get: () => interactor.value ? 'interactive' : checker.value ? 'special' : 'normal',
  set: value => {
    const runtime = available.value.find(item => item.id === 'cpp23-gcc')?.id ?? available.value[0]?.id ?? 'cpp17'
    const previous = interactor.value ?? checker.value ?? previousCode ?? { runtime, source: '', protocol: runtime === 'cpp23-gcc' ? 'testlib' : 'legacy' }
    previousCode = previous
    checker.value = value === 'special' ? previous : null
    interactor.value = value === 'interactive' ? previous : null
  },
})
const code = computed(() => interactor.value ?? checker.value)
const supportsTestlib = computed(() => ['cpp23-gcc', 'cpp23-clang'].includes(code.value?.runtime ?? ''))
const protocol = computed({
  get: () => code.value?.protocol ?? 'legacy',
  set: (value: 'legacy' | 'testlib') => { if (code.value) code.value.protocol = value },
})
watch(supportsTestlib, supported => { if (!supported && protocol.value === 'testlib') protocol.value = 'legacy' })
const codeLabel = computed(() => interactor.value ? '対話用ジャッジ' : '検証コード')
const error = computed(() => code.value && (!code.value.source.trim() || new TextEncoder().encode(code.value.source).length > 65536 || code.value.source.includes('\0'))
  ? '公開・採点するにはコードを1〜65,536バイトで、NUL文字を含めずに入力してください。' : '')
</script>

<template>
  <section class="checker-settings" aria-labelledby="checker-title">
    <h1 id="checker-title">判定方法</h1>
    <label for="judge-method">判定方法</label>
    <select id="judge-method" v-model="method" :disabled="disabled">
      <option value="normal">通常判定（空白区切りで比較）</option>
      <option value="special">スペシャルジャッジ</option>
      <option value="interactive">インタラクティブ（対話形式）</option>
    </select>
    <template v-if="code">
      <p v-if="!interactor && protocol === 'legacy'">提出の出力を標準入力で読み、終了コード0で正解、0以外で不正解とします。assertも使用できます。</p>
      <p v-if="interactor">標準入力で提出の発言を読み、標準出力で応答します。応答を待つ前にflushしてください。</p>
      <p v-if="interactor && protocol === 'legacy'">終了コード0で正解、0以外やassertの失敗で不正解とします。</p>
      <p v-if="interactor">テスト入力は自動送信しません。入力ファイルから読み取り、必要な初期情報を出力してください。</p>
      <p v-if="protocol === 'legacy'">引数は順に、入力・期待出力・提出ソース・スコアのファイルパスです。期待出力は空でも構いません。スコアファイルへの書き込みは採点に使いません。</p>
      <p v-else>testlib.hをincludeし、{{ interactor ? 'registerInteraction' : 'registerTestlibCmd' }}(argc, argv)で初期化してください。quitf(_ok, ...)で正解、_waや_peで不正解、_failでJEとします。部分点には対応していません。</p>
      <p v-if="protocol === 'testlib'">引数は順に、入力・{{ interactor ? 'toutの書き込み先' : '提出出力' }}・正解のファイルパスです。{{ interactor ? 'toutは通信には使わず、内容の後段判定も行いません。' : '提出出力はouf、正解はansから読みます。' }}</p>
      <p v-if="protocol === 'testlib'"><NuxtLink to="/blog/language-guide#testlib" target="_blank" rel="noopener noreferrer">testlibのコード例を見る</NuxtLink></p>
      <label for="checker-language">{{ codeLabel }}の言語</label>
      <select id="checker-language" v-model="code.runtime" :disabled="disabled">
        <option v-if="!available.some(item => item.id === code?.runtime)" :value="code.runtime">{{ code.runtime }}（現在利用できません）</option>
        <option v-for="item in available" :key="item.id" :value="item.id">{{ item.label }}</option>
      </select>
      <label for="checker-protocol">判定コードの形式</label>
      <select id="checker-protocol" v-model="protocol" :disabled="disabled">
        <option value="legacy">現行形式（標準入力と終了コード）</option>
        <option v-if="supportsTestlib" value="testlib">testlib形式（Codeforces互換）</option>
      </select>
      <p class="muted">コードは自動保存。64 KiBまで。ジャッジ側は各ケースCPU 5秒・{{ interactor ? 256 : 512 }} MiBで実行し、制限超過はJEになります。</p>
      <SourceCodeEditor v-model="code.source" :runtime="code.runtime" :label="codeLabel" :disabled="disabled" />
      <p v-if="error" class="editor-error" role="alert">{{ error }}</p>
    </template>
    <p v-if="published">提出は公開中の設定で採点します。編集内容を採点へ反映するには、問題管理から公開内容を更新してください。</p>
    <p v-else>テストケースを登録して解答を提出すると、保存した下書きで採点できます。正解例と不正解例の両方を試してください。</p>
    <SubmissionForm v-if="problemId" class="checker-submission" :has-samples="published ? !!publishedProblem?.hasSamples : hasSamples" :problem-id="problemId" :before-submit="save" :disabled="disabled || !!error" />
  </section>
</template>

<style scoped>
.checker-settings { flex: 1; min-height: 0; overflow-y: auto; padding: 24px; }
.checker-settings h1 { font-size: 1.25rem; }
.checker-settings p { max-width: 1000px; font-size: .875rem; }
label { display: block; margin-block: 16px 8px; }
select { max-width: 100%; min-height: 44px; padding: 8px 12px; border: 1px solid var(--color-line); border-radius: 4px; background: var(--color-paper); color: var(--color-ink); }
.checker-settings :deep(.source-code-editor) { max-width: 1000px; margin-block: 12px; }
.checker-submission { max-width: 1000px; padding: 16px; border: 1px solid var(--color-line); border-radius: 4px; }
.checker-submission :deep(h2:first-child) { margin-top: 0; }
@media (max-width: 600px) { .checker-settings { padding: 16px 12px; } }
</style>
