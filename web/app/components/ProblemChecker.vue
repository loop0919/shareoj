<script setup lang="ts">
import type { ProblemDraft } from '~~/shared/types/problem-draft'

const checker = defineModel<ProblemDraft['checker']>({ required: true })
const interactor = defineModel<ProblemDraft['interactor']>('interactor', { required: true })
const props = defineProps<{ disabled: boolean, problemId: string, published: boolean, save: () => Promise<boolean> }>()
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
const opening = ref(false)
const openError = ref('')
// Submissions run on the problem page; saving first keeps the draft judge in sync with this editor.
async function openProblem() {
  if (opening.value || props.disabled || error.value) return
  opening.value = true
  openError.value = ''
  try {
    if (!await props.save()) { openError.value = '下書きを保存できませんでした。保存内容を確認してください。'; return }
    await nextTick()
    if (props.problemId) await navigateTo(`/problems/${props.problemId}#submission-title`)
  } finally { opening.value = false }
}
const error = computed(() => code.value && (!code.value.source.trim() || new TextEncoder().encode(code.value.source).length > 65536 || code.value.source.includes('\0'))
  ? '公開・採点するにはコードを1〜65,536バイトで、NUL文字を含めずに入力してください。' : '')
</script>

<template>
  <section class="checker-settings" aria-label="判定方法">
    <div class="checker-options">
      <div class="checker-option">
        <label for="judge-method">判定方法</label>
        <select id="judge-method" v-model="method" :disabled="disabled">
          <option value="normal">通常判定（空白区切りで比較）</option>
          <option value="special">スペシャルジャッジ</option>
          <option value="interactive">インタラクティブ（対話形式）</option>
        </select>
      </div>
      <template v-if="code">
        <div class="checker-option">
          <label for="checker-language">{{ codeLabel }}の言語</label>
          <select id="checker-language" v-model="code.runtime" :disabled="disabled">
            <option v-if="!available.some(item => item.id === code?.runtime)" :value="code.runtime">{{ code.runtime }}（現在利用できません）</option>
            <option v-for="item in available" :key="item.id" :value="item.id">{{ item.label }}</option>
          </select>
        </div>
        <div class="checker-option">
          <label for="checker-protocol">判定コードの形式</label>
          <select id="checker-protocol" v-model="protocol" :disabled="disabled">
            <option value="legacy">現行形式（標準入力と終了コード）</option>
            <option v-if="supportsTestlib" value="testlib">testlib形式（Codeforces互換）</option>
          </select>
        </div>
      </template>
    </div>
    <template v-if="code">
      <dl class="code-spec" :aria-label="`${codeLabel}の仕様`">
        <template v-if="interactor">
          <div><dt>対話</dt><dd>標準入力で提出の発言を読み、標準出力で応答します。応答を待つ前に flush してください。</dd></div>
          <div><dt>初期情報</dt><dd>テスト入力は自動で送りません。入力ファイルから読み、必要な情報を出力してください。</dd></div>
        </template>
        <div v-else-if="protocol === 'legacy'"><dt>標準入力</dt><dd>提出の出力</dd></div>
        <div v-if="protocol === 'testlib'"><dt>初期化</dt><dd><code>testlib.h</code> を include し、<code>{{ interactor ? 'registerInteraction' : 'registerTestlibCmd' }}(argc, argv)</code> を呼びます。<NuxtLink to="/blog/language-guide#testlib" target="_blank" rel="noopener noreferrer">コード例 ↗</NuxtLink></dd></div>
        <div v-if="protocol === 'legacy'"><dt>引数</dt><dd>入力・期待出力・提出ソース・スコアのファイルパス（この順）。期待出力は空でもよく、スコアは採点に使いません。</dd></div>
        <div v-else-if="interactor"><dt>引数</dt><dd>入力・tout の書き込み先・正解のファイルパス（この順）。tout は通信に使わず、あとから判定もしません。</dd></div>
        <div v-else><dt>引数</dt><dd>入力・提出出力（ouf）・正解（ans）のファイルパス（この順）</dd></div>
        <div v-if="protocol === 'legacy'"><dt>結果</dt><dd>終了コード0で正解、それ以外（assert の失敗を含む）で不正解</dd></div>
        <div v-else><dt>結果</dt><dd><code>quitf(_ok, …)</code> で正解、<code>_wa</code>・<code>_pe</code> で不正解、<code>_fail</code> でJE。部分点はありません。</dd></div>
        <div><dt>制限</dt><dd>各ケースCPU 5秒・{{ interactor ? 256 : 512 }} MiB。超えるとJEになります。</dd></div>
      </dl>
      <SourceCodeEditor v-model="code.source" :runtime="code.runtime" :label="codeLabel" :disabled="disabled" />
      <p v-if="error" class="editor-error" role="alert">{{ error }}</p>
    </template>
    <div class="checker-trial">
      <p v-if="published">提出は公開中の設定で採点します。編集内容を採点へ反映するには、問題管理から公開内容を更新してください。</p>
      <p v-else>問題ページから解答を提出すると、保存した下書きで採点します。</p>
      <button type="button" class="editor-button" :disabled="disabled || !!error || opening" @click="openProblem">{{ opening ? '保存しています…' : '問題ページで提出する' }}</button>
      <p v-if="openError" class="editor-error" role="alert">{{ openError }}</p>
    </div>
  </section>
</template>

<style scoped>
.checker-settings p { max-width: 1000px; margin: 8px 0; font-size: .8125rem; }
.checker-options { display: flex; flex-wrap: wrap; gap: 12px; margin-top: 12px; }
.checker-option { display: flex; flex-direction: column; gap: 4px; min-width: 0; max-width: 100%; }
label { font-size: .75rem; }
select { max-width: 100%; min-height: 36px; padding: 4px 8px; border: 1px solid var(--color-line); border-radius: 4px; background: var(--color-paper); color: var(--color-ink); }
.checker-settings :deep(.source-code-editor) { max-width: 1000px; margin-block: 12px; }
.checker-trial { max-width: 1000px; margin-top: 20px; }
.checker-trial button { min-height: 40px; margin-top: 4px; }
</style>
