<script setup lang="ts">
import { accountError } from '~/utils/account-problems'

definePageMeta({ editorLayout: true })
useSeoMeta({ title: '問題を作成 | ShareOJ', robots: 'noindex, nofollow' })
const {
  user, draft, ready, cloudId, saving, publishing, publishedVersion, contestId, manualSaveOnly,
  publicationError, saveLocation, saveState, status, storageError, leaveDialog, leaveError,
  manageDialog, generating, deleteError, errors, saveDraft, publishProblem, finishLeave,
  openDeleteConfirmation, closeDeleteConfirmation, removeProblem,
} = useProblemDraft()
const testerLink = ref('')
const testerLinkBusy = ref(false)
const testerLinkMessage = ref('')
async function createTesterLink() {
  if (testerLinkBusy.value) return
  testerLinkBusy.value = true; testerLinkMessage.value = ''
  try {
    if (!await saveDraft(!manualSaveOnly.value)) return
    const result = await $fetch<{ token: string }>(`/api/my/problems/${cloudId.value}/tester-invitation`, { method: 'POST' })
    testerLink.value = new URL(`/my/tester-invitations/${result.token}`, window.location.origin).href
  } catch (error) { testerLinkMessage.value = accountError(error) }
  finally { testerLinkBusy.value = false }
}
async function copyTesterLink() {
  try { await navigator.clipboard.writeText(testerLink.value); testerLinkMessage.value = 'リンクをコピーしました。' }
  catch { testerLinkMessage.value = 'リンクを選択してコピーしてください。' }
}
const section = ref<'statement' | 'editorial' | 'tests' | 'generators' | 'checker' | 'management'>('statement')
const activeMarkdown = computed({
  get: () => section.value === 'editorial' ? draft.editorial : draft.markdown,
  set: value => { if (section.value === 'editorial') draft.editorial = value; else draft.markdown = value },
})
const timeLimitOptions = Array.from({ length: 50 }, (_, index) => (index + 1) * 100)
const memoryLimitPresets = [64, 128, 256, 512]
// Keep in-range memory limits from older drafts selectable.
const memoryLimitOptions = computed(() => [...new Set([...memoryLimitPresets, Number(draft.memoryLimitMb)])].filter(value => value >= 64 && value <= 512).sort((a, b) => a - b))
const {
  mode, workspace, splitPercent, resizing, setSplit, startResize, moveResize, stopResize, resizeWithKeyboard,
  editor, syncSource, insertSnippet,
} = useMarkdownEditor()

const managing = computed(() => section.value === 'management')
const sidebarExpanded = ref(false)
const problemSettingsExpanded = ref(false)
const showErrors = ref(false)
const touched = reactive({ title: false, markdown: false })
const renderedSource = ref(activeMarkdown.value)
let previewTimer: ReturnType<typeof setTimeout> | undefined
watch(activeMarkdown, () => {
  clearTimeout(previewTimer)
  previewTimer = setTimeout(() => { renderedSource.value = activeMarkdown.value }, 150)
})
watch(ready, () => { renderedSource.value = activeMarkdown.value; nextTick(syncSource) })
watch(section, () => {
  clearTimeout(previewTimer)
  renderedSource.value = activeMarkdown.value
  nextTick(syncSource)
})

onBeforeUnmount(() => clearTimeout(previewTimer))

function clearTestCases() {
  if (generating.value) return
  if (window.confirm('編集中のテストケースをすべて削除しますか？採点への反映には公開内容の更新が必要です。')) draft.testCases = []
}
function openManagement() {
  section.value = 'management'
}
const inputSnippet = '\n```input\n$N$\n$A_1 \\quad A_2 \\quad \\cdots \\quad A_N$\n```\n'
const mathSnippet = '\n```math\n\\sum_{i=1}^{N} A_i\n```\n'
</script>

<template>
  <div class="author-page">
    <dialog ref="leaveDialog" class="leave-dialog" aria-labelledby="leave-title" aria-describedby="leave-description" @cancel.prevent="finishLeave('stay')">
      <h2 id="leave-title">未保存の変更があります</h2>
      <p id="leave-description">変更を保存してから移動しますか？ 保存せずに移動すると、未保存の変更は失われます。</p>
      <p v-if="leaveError" role="alert" class="field-error">{{ leaveError }}</p>
      <div class="leave-dialog-actions">
        <button type="button" class="editor-button" autofocus @click="finishLeave('stay')">編集を続ける</button>
        <button type="button" class="editor-button" @click="finishLeave('discard')">保存せずに移動</button>
        <button type="button" class="editor-button primary" @click="finishLeave('save')">保存して移動</button>
      </div>
    </dialog>
    <dialog ref="manageDialog" class="leave-dialog" aria-labelledby="manage-title" @cancel.prevent="closeDeleteConfirmation">
      <h2 id="manage-title">この問題を削除しますか？</h2>
      <p class="manage-problem-title">{{ draft.title.trim() || '無題の問題' }}</p>
      <p>問題と、未保存の変更を削除します。この操作は取り消せません。</p>
      <p v-if="deleteError" role="alert" class="field-error">{{ deleteError }}</p>
      <div class="leave-dialog-actions">
        <button type="button" class="editor-button" autofocus @click="closeDeleteConfirmation">キャンセル</button>
        <button type="button" class="editor-button danger" @click="removeProblem">削除する</button>
      </div>
    </dialog>
    <header class="editor-topbar">
      <NuxtLink class="wordmark" to="/my" aria-label="ShareOJ マイページ">Share<span>OJ</span><span class="wordmark-beta">(β)</span></NuxtLink>
      <ThemeSelect />
      <div v-show="section === 'statement' || section === 'editorial'" class="editor-view-switch" aria-label="表示の切り替え">
        <button type="button" class="editor-button" :aria-pressed="mode === 'edit'" aria-label="編集" title="編集" @click="mode = 'edit'"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="m15 4 5 5M4 20l4-1L20 7a2 2 0 0 0-4-4L4 15Z" /></svg></button>
        <button type="button" class="editor-button split-button" :aria-pressed="mode === 'split'" aria-label="分割" title="分割" @click="mode = 'split'"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><rect x="3" y="3" width="18" height="18" rx="2" /><path d="M12 3v18" /></svg></button>
        <button type="button" class="editor-button" :aria-pressed="mode === 'preview'" aria-label="プレビュー" title="プレビュー" @click="mode = 'preview'"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12Z" /><circle cx="12" cy="12" r="3" /></svg></button>
      </div>
      <div class="author-actions">
        <div class="editor-save-actions">
        <button type="button" class="editor-button primary save-button" :disabled="!ready || saving || publishing" :aria-busy="saving" :aria-label="saving ? '保存中' : '保存'" title="保存（Ctrl+S / ⌘S）" aria-keyshortcuts="Control+s Meta+s" @click="saveDraft(true)">
          <span :class="{ 'save-label-hidden': saving }">保存</span>
          <span v-if="saving" class="save-spinner" aria-hidden="true" />
        </button>
        <NuxtLink v-if="!user" class="editor-button" to="/login" target="_blank" rel="noopener">ログイン</NuxtLink>
        </div>
      </div>
    </header>
    <div class="author-body" :data-sidebar-expanded="sidebarExpanded">
      <aside class="editor-sidebar" aria-label="問題作成サイドバー">
        <button type="button" class="editor-button editor-sidebar-toggle" :aria-expanded="sidebarExpanded" aria-controls="editor-section-nav" :aria-label="sidebarExpanded ? 'サイドバーを折りたたむ' : 'サイドバーを展開'" :title="sidebarExpanded ? 'サイドバーを折りたたむ' : 'サイドバーを展開'" @click="sidebarExpanded = !sidebarExpanded">
          <svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><rect x="3" y="3" width="18" height="18" rx="2" /><path d="M9 3v18" /><path :d="sidebarExpanded ? 'm15 9-3 3 3 3' : 'm13 9 3 3-3 3'" /></svg>
        </button>
        <nav id="editor-section-nav" class="editor-section-nav" aria-label="問題作成メニュー">
          <button type="button" class="editor-button editor-sidebar-item" :aria-current="section === 'statement' ? 'page' : undefined" aria-label="問題文" title="問題文" @click="section = 'statement'"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M14 2H5v20h14V7Zm0 0v5h5M8 12h8M8 16h6" /></svg><span class="editor-sidebar-label">問題文</span></button>

          <button type="button" class="editor-button editor-sidebar-item" :disabled="!ready || publishing" :aria-current="section === 'tests' ? 'page' : undefined" aria-label="テストケース" title="テストケース" @click="section = 'tests'"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="m3 6 2 2 3-4m-5 9 2 2 3-4m-5 9 2 2 3-4M12 6h9M12 13h9M12 20h9" /></svg><span class="editor-sidebar-label">テストケース</span></button>
          <button type="button" class="editor-button editor-sidebar-item" :disabled="!ready || publishing" :aria-current="section === 'generators' ? 'page' : undefined" aria-label="生成と検証" title="生成と検証" @click="section = 'generators'"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="m7 7-5 5 5 5m10-10 5 5-5 5M14 4l-4 16" /></svg><span class="editor-sidebar-label">生成と検証</span></button>
          <button type="button" class="editor-button editor-sidebar-item" :disabled="!ready || publishing" :aria-current="section === 'checker' ? 'page' : undefined" aria-label="判定方法" title="判定方法" @click="section = 'checker'"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="m4 12 5 5L20 6" /></svg><span class="editor-sidebar-label">判定方法</span></button>
          <button type="button" class="editor-button editor-sidebar-item" :disabled="!ready || publishing" :aria-current="section === 'editorial' ? 'page' : undefined" aria-label="解説" title="解説" @click="section = 'editorial'"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M12 5c-3-2-6-2-10-1v15c4-1 7-1 10 1 3-2 6-2 10-1V4c-4-1-7-1-10 1Zm0 0v15" /></svg><span class="editor-sidebar-label">解説</span></button>
          <button type="button" class="editor-button editor-sidebar-item" :disabled="!ready || publishing" :aria-current="managing ? 'page' : undefined" aria-label="問題管理" title="問題管理" @click="openManagement"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="m9 3 .6-2h4.8l.6 2 2 1.2 2.1-.5 2.4 4.2-1.5 1.5v2.3l1.5 1.5-2.4 4.2-2.1-.5-2 1.2-.6 2H9l-.6-2-2-1.2-2.1.5-2.4-4.2 1.5-1.5V9.4L1.9 7.9l2.4-4.2 2.1.5Z" transform="translate(0 1)" /><circle cx="12" cy="12" r="3" /></svg><span class="editor-sidebar-label">問題管理</span></button>
        </nav>
      </aside>
      <div class="editor-main">
    <div class="editor-notices">
      <p v-if="storageError" class="editor-error" role="alert">{{ storageError }}</p>
      <p v-if="manualSaveOnly">自動保存はオフです。変更を反映するには「保存」を押してください。<template v-if="contestId">保存するとコンテストの出題・採点内容に反映されます。</template></p>
      <noscript><p class="editor-error">編集と保存には JavaScript を有効にしてください。</p></noscript>
    </div>
    <div v-show="section === 'statement' || section === 'editorial'" class="author-edit-content">
    <div v-if="section === 'statement'" class="author-fields">
      <div class="title-field">
        <div class="field-heading"><label for="problem-title">問題のタイトル</label><span id="title-error" class="field-error inline-field-error" aria-live="polite">{{ showErrors || touched.title ? errors.title : '' }}</span></div>
        <input id="problem-title" v-model="draft.title" maxlength="120" placeholder="例：A + B" :disabled="!ready || publishing" :aria-invalid="(showErrors || touched.title) && !!errors.title" aria-describedby="title-error" @blur="touched.title = true">
      </div>
      <button type="button" class="editor-button problem-settings-toggle" aria-label="問題設定" :aria-expanded="problemSettingsExpanded" aria-controls="problem-settings" @click="problemSettingsExpanded = !problemSettingsExpanded">
        <span>問題設定 <span aria-hidden="true">{{ problemSettingsExpanded ? '▴' : '▾' }}</span></span><span>{{ draft.timeLimitMs }} ms / {{ draft.memoryLimitMb }} MiB</span>
      </button>
      <div id="problem-settings" class="problem-settings" :data-expanded="problemSettingsExpanded">
        <div class="field"><label for="problem-difficulty">難易度（作成者設定）</label><DifficultySelect id="problem-difficulty" v-model="draft.difficulty" label="難易度（作成者設定）" :disabled="!ready || publishing" /></div>
        <div>
          <div class="field-heading"><span class="limit-field-label" id="time-limit-label">実行時間制限 <span>ms</span></span></div>
          <LimitStepper id="time-limit" v-model="draft.timeLimitMs" :options="timeLimitOptions" :default-value="2000" :step="100" label="実行時間制限" labelledby="time-limit-label" :disabled="!ready || publishing" />
        </div>
        <div>
          <div class="field-heading"><span class="limit-field-label" id="memory-limit-label">メモリ制限 <span>MiB</span></span></div>
          <LimitStepper id="memory-limit" v-model="draft.memoryLimitMb" :options="memoryLimitOptions" :default-value="512" label="メモリ制限" labelledby="memory-limit-label" :disabled="!ready || publishing" />
        </div>
      </div>
    </div>
    <div ref="workspace" class="author-workspace" :class="{ 'is-resizing': resizing }" :data-mode="mode" :style="{ '--editor-left': `${splitPercent}fr`, '--editor-right': `${100 - splitPercent}fr` }">
      <section id="source-pane" class="source-pane" aria-label="Markdown 編集">
        <div class="pane-heading"><div class="source-heading-label"><label id="problem-source-label" for="problem-source" @click="editor?.focus()">{{ section === 'editorial' ? '解説' : '本文' }} <span>(Markdown)</span></label><NuxtLink class="source-guide-link" to="/blog/markdown-guide" target="_blank" rel="noopener noreferrer" aria-label="Markdown・数式の書き方" title="Markdown・数式の書き方"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="9" /><path d="M9.5 9a2.5 2.5 0 0 1 5 .5c0 1.5-2.5 2-2.5 3.5M12 16h.01" /></svg></NuxtLink><span v-if="section === 'statement'" id="source-error" class="field-error inline-field-error" aria-live="polite">{{ showErrors || touched.markdown ? errors.markdown : '' }}</span></div><span>{{ activeMarkdown.length.toLocaleString('en-US') }} / 100,000</span></div>
        <div class="editor-toolbar" aria-label="記法を挿入">
          <button type="button" :disabled="!ready || publishing" @click="insertSnippet('\n## 見出し\n')" aria-label="見出し" title="見出し"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M5 5v14M19 5v14M5 12h14" /></svg></button>
          <button type="button" :disabled="!ready || publishing" @click="insertSnippet('**強調**')" aria-label="太字" title="太字"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path stroke-width="2.4" d="M6 12h7a4 4 0 0 1 0 8H6V4h6a4 4 0 0 1 0 8" /></svg></button>
          <button type="button" :disabled="!ready || publishing" @click="insertSnippet(inputSnippet)" aria-label="入力形式" title="入力形式"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><rect x="3" y="4" width="18" height="16" rx="2" /><path d="m6 8 3 3-3 3M12 15h5" /></svg></button>
          <button type="button" :disabled="!ready || publishing" @click="insertSnippet(mathSnippet)" aria-label="数式" title="数式"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M19 4H6l7 8-7 8h13" /></svg></button>
          <button type="button" :disabled="!ready || publishing" @click="insertSnippet('\n```text\n3 5\n```\n')" aria-label="コード" title="コード"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="m8 6-6 6 6 6m8-12 6 6-6 6m-3-14-2 16" /></svg></button>
          <button type="button" :disabled="!ready || publishing" @click="insertSnippet('\n:::details タイトル\n内容\n:::\n')" aria-label="折りたたみ" title="折りたたみ"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><rect x="3" y="4" width="18" height="16" rx="2" /><path d="m7 8 3 3 3-3M7 15h10" /></svg></button>
          <button type="button" :disabled="!ready || publishing || editor?.uploading" :aria-expanded="editor?.showImages ?? false" @click="editor?.toggleImages()" aria-label="画像" title="画像"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><rect x="3" y="3" width="18" height="18" rx="2" /><circle cx="8" cy="8" r="1.5" /><path d="m21 15-5-5L5 21M3 16l4-4 4 4" /></svg></button>
          <EditorSettings kind="markdown" />
        </div>
        <MarkdownSourceEditor id="problem-source" ref="editor" v-model="activeMarkdown" label-id="problem-source-label" :disabled="!ready || publishing" :key="section === 'editorial' ? 'editorial' : 'statement'" :invalid="section === 'statement' && (showErrors || touched.markdown) && !!errors.markdown" :describedby="section === 'statement' ? 'source-error' : undefined" @blur="section === 'statement' && (touched.markdown = true)" />
      </section>
      <div class="split-handle" role="separator" tabindex="0" aria-label="編集欄とプレビューの幅を調整" aria-orientation="vertical" aria-controls="source-pane preview-pane" :aria-valuenow="Math.round(splitPercent)" :aria-valuetext="`編集欄 ${Math.round(splitPercent)}%、プレビュー ${100 - Math.round(splitPercent)}%`" :aria-valuemin="30" :aria-valuemax="70" title="ドラッグで幅を調整・ダブルクリックで均等に戻す" @pointerdown="startResize" @pointermove="moveResize" @pointerup="stopResize" @pointercancel="stopResize" @lostpointercapture="resizing = false" @keydown="resizeWithKeyboard" @dblclick="setSplit(50)" />
      <section id="preview-pane" class="preview-pane" :aria-label="section === 'editorial' ? '解説のプレビュー' : '問題のプレビュー'">
        <div class="pane-heading"><h2>プレビュー</h2><span>表示を確認</span></div>
        <div class="preview-document">
          <p v-if="section === 'statement'" class="preview-title">{{ draft.title || '無題の問題' }}</p>
          <p v-if="section === 'statement'" class="preview-limits">実行時間 {{ draft.timeLimitMs || '—' }} ms ／ メモリ {{ draft.memoryLimitMb || '—' }} MiB</p>
          <ProblemMarkdown v-if="renderedSource.trim()" :source="renderedSource" />
          <p v-else class="muted">{{ section === 'editorial' ? '解説' : '本文' }}を書くと、ここにプレビューが表示されます。</p>
        </div>
      </section>
    </div>
    </div>
    <ProblemChecker v-if="ready" v-show="section === 'checker'" v-model="draft.checker" v-model:interactor="draft.interactor" :disabled="publishing" :problem-id="cloudId" :save="saveDraft" :published="!!publishedVersion" :published-version="publishedVersion" :has-samples="draft.testCases.some(test => test.isSample)" />
    <TestCaseEditor v-if="section === 'tests'" v-model="draft.testCases" v-model:markdown="draft.markdown" :disabled="!ready || publishing || generating" :problem-id="cloudId" />
    <TestCaseGenerator v-if="ready" v-show="section === 'generators'" v-model="draft.testCases" v-model:config="draft.generators" v-model:busy="generating" :save="saveDraft" :disabled="publishing" :problem-id="cloudId" @show-cases="section = 'tests'" />
    <section v-if="managing" class="problem-management" aria-labelledby="management-title">
      <div class="management-content">
        <header><h1 id="management-title">問題管理</h1><p class="manage-problem-title">{{ draft.title.trim() || '無題の問題' }}</p><p class="muted">{{ saveLocation }}</p></header>
        <section class="management-row"><div><h2>公開設定</h2><p>問題文・テストケース・判定方法は「公開内容を更新」を押すまで採点に反映されません。テストケースの入出力は公開ページには表示しません。</p><p v-if="publicationError" class="editor-error" role="alert">{{ publicationError }}</p><NuxtLink v-if="publishedVersion" :to="`/problems/${cloudId}`" target="_blank">公開ページを見る</NuxtLink></div><div class="publication-actions"><button v-if="publishedVersion" class="editor-button primary" :disabled="saving || publishing || generating" @click="publishProblem(true)">公開内容を更新</button><p v-else>未公開の問題は問題一覧の「投稿」から公開できます。</p><button v-if="publishedVersion" class="editor-button" :disabled="saving || publishing || generating" @click="publishProblem(false)">非公開に戻す</button></div></section>
        <section class="management-row"><div><h2>テスターリンク</h2><p>リンクを受け取ったユーザーが「許可する」を押すと、テスターになります。テスターは問題の編集・公開・削除を含め、作者と同じ操作ができます。</p><template v-if="testerLink"><label for="tester-link">招待リンク</label><input id="tester-link" :value="testerLink" readonly @focus="($event.target as HTMLInputElement).select()"><button type="button" class="editor-button" @click="copyTesterLink">リンクをコピー</button></template><p v-if="testerLinkMessage" role="status">{{ testerLinkMessage }}</p></div><button type="button" class="editor-button" :disabled="!ready || saving || publishing || generating || testerLinkBusy" @click="createTesterLink">{{ testerLinkBusy ? '発行中…' : 'リンクを発行' }}</button></section>
        <section class="management-row"><div><h2>リジャッジ</h2><p>テストケースや採点設定の変更後に、提出を再採点します。</p></div><button type="button" class="editor-button" disabled>リジャッジ（準備中）</button></section>
        <section class="management-row"><div><h2>テストケースの一括削除</h2><p>この問題に登録したテストケースをまとめて削除します。</p></div><button type="button" class="editor-button" :disabled="!draft.testCases.length || publishing || generating" @click="clearTestCases">一括削除</button></section>
        <section class="management-row"><div><h2>問題の削除</h2><p>問題を削除します。この操作は取り消せません。</p></div><button type="button" class="editor-button danger" :disabled="generating" @click="openDeleteConfirmation">問題を削除</button></section>
      </div>
    </section>
      </div>
    </div>
    <div class="draft-status"><span role="status" :data-save-state="saveState">{{ status }}</span><span>{{ saveLocation }}</span></div>
  </div>
</template>

<style scoped>
#tester-link { display: block; box-sizing: border-box; width: 100%; margin-block: 8px; padding: 8px; font: inherit; }
.problem-settings { --problem-setting-height: 43px; display: contents; }
.problem-settings :deep(.difficulty-select > button), .problem-settings :deep(.limit-stepper) { min-height: var(--problem-setting-height); }
.problem-settings-toggle { display: none; }
@media (pointer: coarse) {
  .problem-settings { --problem-setting-height: 67px; }
}
@media (max-width: 59.999rem) {
  .problem-settings-toggle { display: flex; align-items: center; justify-content: space-between; gap: 8px; grid-column: 1 / -1; min-height: 44px; text-align: left; }
  .problem-settings-toggle > :last-child { font-size: .75rem; color: var(--color-muted); }
  .problem-settings { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 4px 12px; grid-column: 1 / -1; }
  .problem-settings[data-expanded="false"] { display: none; }
}
@media (min-width: 60rem) {
  .editor-main .author-fields { grid-template-columns: minmax(0, 1fr) 160px 140px 140px; }
}
</style>
