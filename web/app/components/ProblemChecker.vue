<script setup lang="ts">
import { errorJudgeEpsilons, errorJudgeModes, type ProblemDraft } from '~~/shared/types/problem-draft'
import { shortId } from '~~/shared/utils/short-id'
import { judgeCodeRuntimes, preferredRuntime, usableRuntimes } from '~/utils/judge-catalog'
import { hasSource } from '~/utils/has-source'
import { epsilonExponent, errorJudgeModeLabel, thresholdFactor, validMargin } from '~/utils/error-judge'

const checker = defineModel<ProblemDraft['checker']>({ required: true })
const interactor = defineModel<ProblemDraft['interactor']>('interactor', { required: true })
const runTwice = defineModel<ProblemDraft['runTwice']>('runTwice', { required: true })
const errorJudge = defineModel<ProblemDraft['errorJudge']>('errorJudge', { required: true })
// Why a setting typed here cannot go into the draft yet; the page refuses every save while it is set.
const settingError = defineModel<string>('settingError', { default: '' })
// In the aggregate mode the score file and testlib's quitp give each case its score (ADR 0025).
const props = defineProps<{ disabled: boolean, problemId: string, published: boolean, save: () => Promise<boolean>, scoringMode?: 'groups' | 'aggregate' }>()
const aggregate = computed(() => props.scoringMode === 'aggregate')
const { data: catalog, error: catalogError } = useJudgeCatalog()
const available = computed(() => judgeCodeRuntimes(usableRuntimes(catalog.value, catalogError.value)))
let previousCode: ProblemDraft['checker'] = null
// The margin the judge uses when the draft names none.
const defaultMargin = '0.1'
const method = computed({
  get: () => errorJudge.value ? 'errorJudge' : runTwice.value ? 'runTwice' : interactor.value ? 'interactive' : checker.value ? 'special' : 'normal',
  set: value => {
    const runtime = preferredRuntime(available.value)
    const previous = runTwice.value ?? interactor.value ?? checker.value ?? previousCode ?? { runtime, source: '', protocol: runtime === 'cpp23-gcc' ? 'testlib' : 'legacy' }
    previousCode = previous
    checker.value = value === 'special' ? previous : null
    interactor.value = value === 'interactive' ? previous : null
    runTwice.value = value === 'runTwice' ? previous : null
    // The error judge has settings and no code, so like 通常判定 it sets the code aside. It starts from its defaults.
    errorJudge.value = value === 'errorJudge' ? { mode: 'either', epsilon: '1e-6', marginPercent: defaultMargin } : null
  },
})
// The field keeps what the author types, and the draft takes only a valid margin. It follows the draft whenever the
// setting is replaced or its margin changes.
const marginText = ref(errorJudge.value?.marginPercent ?? defaultMargin)
const marginFocused = ref(false)
watch(() => [errorJudge.value, errorJudge.value?.marginPercent] as const, ([value]) => {
  marginText.value = value?.marginPercent ?? defaultMargin
  if (!value) marginFocused.value = false
})
function updateMargin(event: Event) {
  marginText.value = (event.target as HTMLInputElement).value
  if (errorJudge.value && validMargin(marginText.value)) errorJudge.value.marginPercent = marginText.value
}
// A decimal typed key by key passes through a bare trailing dot ("0."). While the field has focus that is unfinished,
// not wrong: the draft keeps the last valid margin, and the line below keeps describing it.
const marginIncomplete = computed(() => marginFocused.value && /^[0-9]{1,3}\.$/.test(marginText.value))
// ε times this factor is what the judge accepts; empty while the margin is invalid.
const factor = computed(() => validMargin(marginText.value) ? thresholdFactor(marginText.value)
  : marginIncomplete.value && errorJudge.value ? thresholdFactor(errorJudge.value.marginPercent ?? defaultMargin) : '')
const marginInvalid = computed(() => !!errorJudge.value && !factor.value)
const marginMessage = '判定の余裕は0〜100の数で、小数は6桁までです。'
watch(marginInvalid, invalid => { settingError.value = invalid ? marginMessage : '' }, { immediate: true })
// 絶対誤差 reads as plain 誤差 in the sentence.
const errorWording = computed(() => !errorJudge.value || errorJudge.value.mode === 'absolute' ? '誤差' : errorJudgeModeLabel(errorJudge.value.mode))
const code = computed(() => runTwice.value ?? interactor.value ?? checker.value)
// Both modes talk to the judge over stdin/stdout, with the same limits and testlib entry point.
const talking = computed(() => !!(interactor.value ?? runTwice.value))
const supportsTestlib = computed(() => ['cpp23-gcc', 'cpp23-clang'].includes(code.value?.runtime ?? ''))
const protocol = computed({
  get: () => code.value?.protocol ?? 'legacy',
  set: (value: 'legacy' | 'testlib') => { if (code.value) code.value.protocol = value },
})
watch(supportsTestlib, supported => { if (!supported && protocol.value === 'testlib') protocol.value = 'legacy' })
const codeLabel = computed(() => runTwice.value ? '2ステップ用ジャッジ' : interactor.value ? '対話用ジャッジ' : '検証コード')
const specOpen = useState('code-spec-open', () => false)
const opening = ref(false)
const openError = ref('')
// Submissions run on the problem page; saving first keeps the draft judge in sync with this editor.
async function openProblem() {
  if (opening.value || props.disabled || error.value || marginInvalid.value) return
  opening.value = true
  openError.value = ''
  try {
    if (!await props.save()) { openError.value = '下書きを保存できませんでした。保存内容を確認してください。'; return }
    await nextTick()
    if (props.problemId) await navigateTo(`/problems/${shortId(props.problemId)}#submission-title`)
  } finally { opening.value = false }
}
const error = computed(() => code.value && (!hasSource(code.value.runtime, code.value.source) || new TextEncoder().encode(code.value.source).length > 65536 || code.value.source.includes('\0'))
  ? '公開・採点するにはコードを1〜65,536バイトで、NUL文字を含めずに入力してください。' : '')
</script>

<template>
  <section class="checker-settings" aria-label="判定方法">
    <div class="checker-options">
      <div class="checker-option">
        <label for="judge-method">判定方法</label>
        <select id="judge-method" v-model="method" :disabled="disabled">
          <option value="normal">通常判定（空白区切りで比較）</option>
          <option value="errorJudge">誤差ジャッジ</option>
          <option value="special">スペシャルジャッジ</option>
          <option value="interactive">インタラクティブ（対話形式）</option>
          <option value="runTwice">2ステップ形式（Run-twice）</option>
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
      <details class="code-spec" :open="specOpen" @toggle="specOpen = ($event.target as HTMLDetailsElement).open">
        <summary><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="m9 6 6 6-6 6" /></svg>{{ codeLabel }}の仕様</summary>
        <dl>
          <template v-if="talking">
            <div><dt>対話</dt><dd>標準入力で提出の発言を読み、標準出力で応答します。応答を待つ前に flush してください。</dd></div>
            <div><dt>初期情報</dt><dd>テスト入力は自動で送りません。入力ファイルから読み、必要な情報を出力してください。</dd></div>
          </template>
          <div v-else-if="protocol === 'legacy'"><dt>標準入力</dt><dd>提出の出力</dd></div>
          <div v-if="protocol === 'testlib'"><dt>初期化</dt><dd><code>testlib.h</code> を include し、<code>{{ talking ? 'registerInteraction' : 'registerTestlibCmd' }}(argc, argv)</code> を呼びます。<NuxtLink :to="runTwice ? '/docs/checkers#run-twice' : '/docs/checkers#testlib'" target="_blank" rel="noopener noreferrer">コード例 ↗</NuxtLink></dd></div>
          <div v-if="protocol === 'legacy' && runTwice"><dt>引数</dt><dd>入力・正解・提出ソース・スコアのファイルパス、何回目か（<code>1</code>か<code>2</code>）、引き継ぎファイルのパス（この順）。引き継ぎファイルは1回目に書き、2回目に読みます。</dd></div>
          <div v-else-if="protocol === 'legacy'"><dt>引数</dt><dd>入力・期待出力・提出ソース・スコアのファイルパス（この順）。期待出力は空でもかまいません。</dd></div>
          <div v-if="protocol === 'legacy'"><dt>スコア</dt><dd v-if="aggregate">ACかWAのケースで、書いた整数（0〜10<sup>18</sup>）をケースの点として集計コードに渡します。空なら点なしで、それ以外の内容はJEです。<template v-if="runTwice">読むのは2回目に書いた内容だけです。</template></dd><dd v-else>「集計コードで採点する」をオンにした問題でだけ、ケースの点として読みます。<template v-if="runTwice">読むのは2回目に書いた内容だけです。</template></dd></div>
          <div v-else-if="runTwice"><dt>引数</dt><dd>入力・tout の書き込み先・正解のファイルパス（この順）。1回目に tout へ書いた内容が、2回目の入力（inf）になります。何回目かは入力の先頭の値などで見分けてください。</dd></div>
          <div v-else-if="interactor"><dt>引数</dt><dd>入力・tout の書き込み先・正解のファイルパス（この順）。tout は通信に使わず、あとから判定もしません。</dd></div>
          <div v-else><dt>引数</dt><dd>入力・提出出力（ouf）・正解（ans）のファイルパス（この順）</dd></div>
          <div v-if="protocol === 'legacy'"><dt>結果</dt><dd>終了コード0で正解、それ以外（assert の失敗を含む）で不正解</dd></div>
          <div v-else><dt>結果</dt><dd><code>quitf(_ok, …)</code> で正解、<code>_wa</code>・<code>_pe</code> で不正解、<code>_fail</code> でJE。<template v-if="aggregate"><code>quitp</code> に整数（0〜1,000,000）を渡すと、正解とし、その値をケースの点にします。小数の <code>quitp</code> と <code>_pc</code> はJEです。</template><template v-else><code>quitp</code> は、「集計コードで採点する」をオンにした問題でだけ使えます。</template></dd></div>
          <div><dt>制限</dt><dd>各ケースCPU 5秒・{{ talking ? 256 : 512 }} MiB。超えるとJEになります。</dd></div>
        </dl>
      </details>
      <SourceCodeEditor v-model="code.source" :runtime="code.runtime" :label="codeLabel" :disabled="disabled" />
      <p v-if="error" class="editor-error" role="alert">{{ error }}</p>
    </template>
    <template v-else-if="errorJudge">
      <div class="error-judge-fields">
        <div>
          <label for="error-judge-mode">種類</label>
          <select id="error-judge-mode" v-model="errorJudge.mode" :disabled="disabled">
            <option v-for="mode in errorJudgeModes" :key="mode" :value="mode">{{ errorJudgeModeLabel(mode) }}</option>
          </select>
        </div>
        <div>
          <label for="error-judge-epsilon">許容誤差</label>
          <select id="error-judge-epsilon" v-model="errorJudge.epsilon" :disabled="disabled">
            <option v-for="epsilon in errorJudgeEpsilons" :key="epsilon" :value="epsilon">10^{{ epsilonExponent(epsilon) }}</option>
          </select>
        </div>
        <div>
          <label for="error-judge-margin">判定の余裕</label>
          <span class="margin-input"><input id="error-judge-margin" :value="marginText" inputmode="decimal" autocomplete="off" :disabled="disabled" :aria-invalid="marginInvalid" aria-describedby="error-judge-threshold" @input="updateMargin" @focus="marginFocused = true" @blur="marginFocused = false"> %</span>
        </div>
      </div>
      <div class="error-judge-notes">
        <p id="error-judge-threshold" :class="{ 'field-error': marginInvalid }" aria-live="polite">
          <template v-if="marginInvalid">{{ marginMessage }}</template>
          <template v-else-if="factor === '1'">期待出力との{{ errorWording }}が 10<sup>{{ epsilonExponent(errorJudge.epsilon) }}</sup> を少しでも超えると不正解になります。</template>
          <template v-else>期待出力との{{ errorWording }}が <span class="threshold">{{ factor }} × 10<sup>{{ epsilonExponent(errorJudge.epsilon) }}</sup></span> 以下なら正解になります。</template>
        </p>
        <p v-if="factor !== '1'">想定解の出力も真の値からわずかにずれるので、その分だけ広げて比べます。</p>
        <p>数として読めるトークンだけを誤差で比べ、それ以外は完全一致で比べます。<NuxtLink to="/docs/checkers#error-judge" target="_blank" rel="noopener noreferrer">誤差ジャッジの説明 ↗</NuxtLink></p>
      </div>
    </template>
    <div class="checker-trial">
      <p v-if="published">「保存」を押すと、編集した設定が以後の提出の採点に反映されます。</p>
      <p v-else>問題ページから解答を提出すると、保存した下書きで採点します。</p>
      <button type="button" class="editor-button" :disabled="disabled || !!error || marginInvalid || opening" @click="openProblem">{{ opening ? '保存しています…' : '問題ページで提出する' }}</button>
      <p v-if="openError" class="editor-error" role="alert">{{ openError }}</p>
    </div>
  </section>
</template>

<style scoped>
.checker-settings p { max-width: 1000px; margin: 8px 0; font-size: .8125rem; }
.checker-options { display: flex; flex-wrap: wrap; gap: 12px; margin-top: 12px; }
.checker-option { display: flex; flex-direction: column; gap: 4px; min-width: 0; max-width: 100%; }
label { font-size: .75rem; }
select, input { max-width: 100%; min-height: 36px; padding: 4px 8px; border: 1px solid var(--color-line); border-radius: 4px; background: var(--color-paper); color: var(--color-ink); }
/* The settings sit in rows with their names in a column, as in the code spec. */
.error-judge-fields { display: grid; gap: 8px; margin-top: 16px; }
.error-judge-fields > div { display: grid; grid-template-columns: 5.5em minmax(0, max-content); align-items: center; gap: 12px; }
.margin-input { display: inline-flex; align-items: center; gap: 6px; font-size: .8125rem; }
.margin-input input { box-sizing: border-box; width: 7em; font: inherit; font-variant-numeric: tabular-nums; }
.margin-input input[aria-invalid="true"] { border-color: var(--color-error); }
.error-judge-notes { max-width: 1000px; margin-top: 16px; }
.error-judge-notes p { margin: 4px 0; }
.threshold { white-space: nowrap; }
@media (max-width: 39.999rem) { .margin-input input { font-size: 1rem; } }
.checker-settings :deep(.source-code-editor) { max-width: 1000px; margin-block: 12px; }
.checker-trial { max-width: 1000px; margin-top: 20px; }
.checker-trial button { min-height: 40px; margin-top: 4px; }
</style>
