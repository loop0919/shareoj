<script setup lang="ts">
import { accountListSchema, type AccountSummary } from '~~/shared/types/account-problems'
import type { Contest, ContestProblem } from '~~/shared/types/contest'
const props = defineProps<{ contestId?: string }>()
const ready = ref(false)
const busy = ref(false)
const message = ref('')
const title = ref('')
const description = ref('<!-- ここにコンテストの概要を記載 -->\n')
const startsAt = ref('')
const endsAt = ref('')
const penaltyMinutes = ref(5)
const version = ref(0)
const unpublished = ref(true)
const available = ref<AccountSummary[]>([])
const selected = ref<ContestProblem[]>([])
const id = ref(props.contestId ?? '')
const locked = ref(false)
const timezone = ref('')
const section = ref<'description' | 'problems' | 'settings'>('description')
const sidebarExpanded = ref(false)
const form = ref<HTMLFormElement>()
const totalPoints = computed(() => selected.value.reduce((total, p) => total + (Number(p.points) || 0), 0))
const {
  mode, workspace, splitPercent, resizing, setSplit, startResize, moveResize, stopResize, resizeWithKeyboard,
  editor, syncSource, insertSnippet,
} = useMarkdownEditor()
const mathSnippet = '\n$$\na^2 + b^2 = c^2\n$$\n'
function saveWithShortcut(event: KeyboardEvent) {
  if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 's') {
    event.preventDefault()
    if (ready.value && !locked.value && !busy.value) form.value?.requestSubmit()
  }
}
onMounted(() => window.addEventListener('keydown', saveWithShortcut))
onBeforeUnmount(() => window.removeEventListener('keydown', saveWithShortcut))
function localDate(value: string) {
  const date = new Date(value)
  return new Date(date.getTime() - date.getTimezoneOffset() * 60000).toISOString().slice(0, 16)
}
async function load() {
  try {
    if (props.contestId) {
      const c = await $fetch<Contest>(`/api/my/contests/${props.contestId}`)
      if (!c.canEdit) { locked.value = true; message.value = '開始後、または作成者以外は編集できません。'; return }
      title.value = c.title; description.value = c.description; startsAt.value = localDate(c.startsAt); endsAt.value = localDate(c.endsAt)
      unpublished.value = c.status === 'draft'
      penaltyMinutes.value = c.penaltyMinutes; version.value = c.version; selected.value = c.problems
    } else id.value = crypto.randomUUID()
    let cursor = ''
    do {
      const page = accountListSchema.parse(await $fetch('/api/my/problems', { query: { cursor } }))
      available.value.push(...page.items.filter(p => !p.publishedVersion && (!p.contestId || p.contestId === id.value)))
      cursor = page.nextCursor
    } while (cursor)
    ready.value = true
  } catch { message.value = '作成情報を読み込めませんでした。ログイン状態を確認して再読み込みしてください。' }
}
onMounted(() => { timezone.value = Intl.DateTimeFormat().resolvedOptions().timeZone; void load() })
function choose(p: AccountSummary, checked: boolean) {
  if (checked) selected.value.push({ id: p.id, title: p.title, points: 100 })
  else selected.value = selected.value.filter(item => item.id !== p.id)
}
function move(index: number, delta: number) {
  const item = selected.value.splice(index, 1)[0]
  if (item) selected.value.splice(index + delta, 0, item)
}
async function save(publish = false) {
  if (busy.value || !ready.value || locked.value) return
  message.value = ''
  const invalid = form.value?.querySelector<HTMLInputElement | HTMLTextAreaElement>('input:invalid, textarea:invalid')
  if (invalid) {
    section.value = invalid.closest<HTMLElement>('[data-section]')!.dataset.section as typeof section.value
    if (section.value === 'description') mode.value = 'edit'
    await nextTick()
    invalid.reportValidity()
    return
  }
  if (!title.value.trim()) { section.value = 'description'; message.value = 'コンテストタイトルを入力してください。'; return }
  if (!selected.value.length || selected.value.length > 100) {
    section.value = 'problems'; message.value = '問題を1〜100問選んでください。'; return
  }
  const start = new Date(startsAt.value), end = new Date(endsAt.value)
  if (!Number.isFinite(start.getTime()) || !Number.isFinite(end.getTime()) || start.getTime() <= Date.now() || end <= start) {
    section.value = 'settings'; message.value = '未来の開始日時と、それより後の終了日時を指定してください。'; return
  }
  busy.value = true; message.value = ''
  try {
    await $fetch(`/api/my/contests/${id.value}`, { method: 'PUT', body: { publish, title: title.value, description: description.value, startsAt: start.toISOString(), endsAt: end.toISOString(), penaltyMinutes: penaltyMinutes.value, version: version.value, problems: selected.value.map(({ id, points }) => ({ id, points })) } })
    await navigateTo(`/contests/${id.value}`)
  } catch (error) {
    const status = (error as { statusCode?: number }).statusCode
    message.value = status === 409 ? '保存できませんでした。問題の公開状態・他コンテストへの登録・テストケースを確認してください。開催開始や別画面での更新があった場合は再読み込みが必要です。' : status === 400 ? '入力内容を確認してください。' : '保存を確認できませんでした。作成したコンテスト一覧を確認してから再度お試しください。'
  } finally { busy.value = false }
}
</script>
<template>
  <div class="author-page contest-editor">
    <header class="editor-topbar">
      <NuxtLink class="wordmark" to="/my" aria-label="ShareOJ マイページ">Share<span>OJ</span><span class="wordmark-beta">(β)</span></NuxtLink>
      <h1 class="visually-hidden">{{ contestId ? 'コンテスト編集' : 'コンテスト作成' }}</h1>
      <div v-show="section === 'description'" class="editor-view-switch" aria-label="表示の切り替え">
        <button type="button" class="editor-button" :aria-pressed="mode === 'edit'" aria-label="編集" title="編集" @click="mode = 'edit'"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="m15 4 5 5M4 20l4-1L20 7a2 2 0 0 0-4-4L4 15Z" /></svg></button>
        <button type="button" class="editor-button split-button" :aria-pressed="mode === 'split'" aria-label="分割" title="分割" @click="mode = 'split'"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><rect x="3" y="3" width="18" height="18" rx="2" /><path d="M12 3v18" /></svg></button>
        <button type="button" class="editor-button" :aria-pressed="mode === 'preview'" aria-label="プレビュー" title="プレビュー" @click="mode = 'preview'"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12Z" /><circle cx="12" cy="12" r="3" /></svg></button>
      </div>
      <div class="author-actions">
        <button type="submit" form="contest-form" class="editor-button primary save-button" :disabled="!ready || locked || busy" :aria-busy="busy" :aria-label="busy ? '保存中' : contestId ? '変更を保存' : 'コンテストを作成'" title="保存（Ctrl+S / ⌘S）" aria-keyshortcuts="Control+s Meta+s">
          <span :class="{ 'save-label-hidden': busy }">{{ contestId ? '保存' : '作成' }}</span><span v-if="busy" class="save-spinner" aria-hidden="true" />
        </button>
        <button v-if="contestId && unpublished" type="button" class="editor-button primary" :disabled="!ready || locked || busy" @click="save(true)">投稿</button>
      </div>
    </header>
    <div class="author-body" :data-sidebar-expanded="sidebarExpanded">
      <aside class="editor-sidebar" aria-label="コンテスト作成サイドバー">
        <button type="button" class="editor-button editor-sidebar-toggle" :aria-expanded="sidebarExpanded" aria-controls="contest-section-nav" :aria-label="sidebarExpanded ? 'サイドバーを折りたたむ' : 'サイドバーを展開'" :title="sidebarExpanded ? 'サイドバーを折りたたむ' : 'サイドバーを展開'" @click="sidebarExpanded = !sidebarExpanded">
          <svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><rect x="3" y="3" width="18" height="18" rx="2" /><path d="M9 3v18" /><path :d="sidebarExpanded ? 'm15 9-3 3 3 3' : 'm13 9 3 3-3 3'" /></svg>
        </button>
        <nav id="contest-section-nav" class="editor-section-nav" aria-label="コンテスト作成メニュー">
          <button type="button" class="editor-button editor-sidebar-item" :aria-current="section === 'description' ? 'page' : undefined" aria-label="タイトル・説明" title="タイトル・説明" @click="section = 'description'"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M14 2H5v20h14V7Zm0 0v5h5M8 12h8M8 16h6" /></svg><span class="editor-sidebar-label">タイトル・説明</span></button>
          <button type="button" class="editor-button editor-sidebar-item" :aria-current="section === 'problems' ? 'page' : undefined" aria-label="問題・配点" title="問題・配点" @click="section = 'problems'"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="m3 6 2 2 3-4m-5 9 2 2 3-4m-5 9 2 2 3-4M12 6h9M12 13h9M12 20h9" /></svg><span class="editor-sidebar-label">問題・配点</span></button>
          <button type="button" class="editor-button editor-sidebar-item" :aria-current="section === 'settings' ? 'page' : undefined" aria-label="コンテスト設定" title="コンテスト設定" @click="section = 'settings'"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="9" /><path d="M12 7v5l3 2" /></svg><span class="editor-sidebar-label">コンテスト設定</span></button>
        </nav>
      </aside>
      <form id="contest-form" ref="form" class="editor-main" novalidate @submit.prevent="save()">
        <div class="editor-notices"><p v-if="unpublished" class="muted">保存したコンテストは未公開です。準備ができたら「投稿」で公開できます。</p><p v-if="message" class="editor-error" role="alert">{{ message }}</p><noscript><p class="editor-error">編集と保存には JavaScript を有効にしてください。</p></noscript></div>
        <div v-show="section === 'description'" class="author-edit-content" data-section="description">
          <div class="author-fields"><div class="title-field"><div class="field-heading"><label for="contest-title">コンテストタイトル</label></div><input id="contest-title" v-model="title" required maxlength="120" placeholder="コンテストのタイトル" :disabled="!ready || locked || busy"></div></div>
    <div ref="workspace" class="author-workspace" :class="{ 'is-resizing': resizing }" :data-mode="mode" :style="{ '--editor-left': `${splitPercent}fr`, '--editor-right': `${100 - splitPercent}fr` }">
      <section id="source-pane" class="source-pane" aria-label="Markdown 編集">
        <div class="pane-heading"><div class="source-heading-label"><label id="contest-description-label" for="contest-description" @click="editor?.focus()">説明（Markdown）</label><NuxtLink class="source-guide-link" to="/blog/markdown-guide" target="_blank" rel="noopener noreferrer" aria-label="Markdown・数式の書き方" title="Markdown・数式の書き方"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="9" /><path d="M9.5 9a2.5 2.5 0 0 1 5 .5c0 1.5-2.5 2-2.5 3.5M12 16h.01" /></svg></NuxtLink></div><span>{{ description.length.toLocaleString('en-US') }} / 100,000</span></div>
        <div class="editor-toolbar" aria-label="記法を挿入">
          <button type="button" :disabled="!ready || busy" @click="insertSnippet('\n## 見出し\n')" aria-label="見出し" title="見出し"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M5 5v14M19 5v14M5 12h14" /></svg></button>
          <button type="button" :disabled="!ready || busy" @click="insertSnippet('**強調**')" aria-label="太字" title="太字"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path stroke-width="2.4" d="M6 12h7a4 4 0 0 1 0 8H6V4h6a4 4 0 0 1 0 8" /></svg></button>
          <button type="button" :disabled="!ready || busy" @click="insertSnippet(mathSnippet)" aria-label="数式" title="数式"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M19 4H6l7 8-7 8h13" /></svg></button>
          <button type="button" :disabled="!ready || busy" @click="insertSnippet('\n```text\nコード\n```\n')" aria-label="コード" title="コード"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="m8 6-6 6 6 6m8-12 6 6-6 6m-3-14-2 16" /></svg></button>
          <button type="button" :disabled="!ready || busy" @click="insertSnippet('\n:::details タイトル\n内容\n:::\n')" aria-label="折りたたみ" title="折りたたみ"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><rect x="3" y="4" width="18" height="16" rx="2" /><path d="m7 8 3 3 3-3M7 15h10" /></svg></button>
          <button type="button" :disabled="!ready || busy || editor?.uploading" :aria-expanded="editor?.showImages ?? false" @click="editor?.toggleImages()" aria-label="画像" title="画像"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><rect x="3" y="3" width="18" height="18" rx="2" /><circle cx="8" cy="8" r="1.5" /><path d="m21 15-5-5L5 21M3 16l4-4 4 4" /></svg></button>
          <EditorSettings kind="markdown" />
        </div>
        <MarkdownSourceEditor id="contest-description" ref="editor" v-model="description" label-id="contest-description-label" :disabled="!ready || busy" />
      </section>
      <div class="split-handle" role="separator" tabindex="0" aria-label="編集欄とプレビューの幅を調整" aria-orientation="vertical" aria-controls="source-pane preview-pane" :aria-valuenow="Math.round(splitPercent)" :aria-valuetext="`編集欄 ${Math.round(splitPercent)}%、プレビュー ${100 - Math.round(splitPercent)}%`" :aria-valuemin="30" :aria-valuemax="70" title="ドラッグで幅を調整・ダブルクリックで均等に戻す" @pointerdown="startResize" @pointermove="moveResize" @pointerup="stopResize" @pointercancel="stopResize" @lostpointercapture="resizing = false" @keydown="resizeWithKeyboard" @dblclick="setSplit(50)" />
      <section id="preview-pane" class="preview-pane" aria-label="コンテストのプレビュー">
        <div class="pane-heading"><h2>プレビュー</h2><span>表示を確認</span></div>
        <div class="preview-document contest-preview">
          <p class="preview-title">{{ title || 'コンテストタイトル' }}</p>
          <ProblemMarkdown v-if="description.trim()" :source="description" />
          <p v-else class="muted">説明を書くと、ここにプレビューが表示されます。</p>
        </div>
      </section>
    </div>
    </div>
        <section v-show="section === 'problems'" class="problem-management" data-section="problems" aria-labelledby="contest-problems-title">
          <div class="management-content">
            <header><h2 id="contest-problems-title">問題・配点</h2><p class="muted">自分の未公開問題から選び、出題順と配点を設定します。</p></header>
            <fieldset :disabled="!ready || locked || busy" class="problem-columns">
              <section aria-labelledby="available-problems-title"><h3 id="available-problems-title">問題を選択</h3><p class="muted">テストケースが必要です。他のコンテストに登録済みの問題は選べません。</p>
                <p v-if="ready && !available.length"><NuxtLink to="/problems/new?fresh=1" target="_blank" rel="noopener noreferrer">問題を作成・保存 ↗</NuxtLink>してから、この画面を再読み込みしてください。</p>
                <div class="problem-picker"><label v-for="p in available" :key="p.id"><input type="checkbox" :checked="selected.some(item => item.id === p.id)" @change="choose(p, ($event.target as HTMLInputElement).checked)"><span>{{ p.title || '無題の問題' }}</span></label></div>
              </section>
              <section aria-labelledby="selected-problems-title"><h3 id="selected-problems-title">出題順・配点 <span class="muted">{{ selected.length }} 問 · 合計 {{ totalPoints }} 点</span></h3>
                <p v-if="!selected.length" class="muted">出題する問題を選択してください。</p>
                <ol class="selected"><li v-for="(p, index) in selected" :key="p.id"><div class="selected-heading"><strong>{{ p.title }}</strong><div class="move-actions"><button class="editor-button" type="button" :disabled="index === 0" :aria-label="`${p.title}を上へ`" @click="move(index, -1)">↑</button><button class="editor-button" type="button" :disabled="index === selected.length - 1" :aria-label="`${p.title}を下へ`" @click="move(index, 1)">↓</button></div></div><label :for="`points-${p.id}`">配点 <input :id="`points-${p.id}`" v-model.number="p.points" type="number" min="1" max="1000000" step="1" required> 点</label></li></ol>
              </section>
            </fieldset>
            <p class="selection-note muted">問題の保存内容は、開始後もコンテストに自動で反映されます。更新後の提出から新しい内容で採点し、受付済みの提出は再採点しません。</p>
          </div>
        </section>
        <section v-show="section === 'settings'" class="problem-management" data-section="settings" aria-labelledby="contest-settings-title">
          <div class="management-content">
            <header><h2 id="contest-settings-title">コンテスト設定</h2><p class="muted">開催時間と誤答ペナルティを設定します。</p></header>
            <fieldset :disabled="!ready || locked || busy">
              <section class="settings-section"><h3>開催時間</h3><p class="muted">日時は {{ timezone || '端末のタイムゾーン' }} で入力します。公開ページでは日本時間で表示します。</p>
                <div class="dates"><div><label for="contest-start">開始日時</label><input id="contest-start" v-model="startsAt" type="datetime-local" required></div><div><label for="contest-end">終了日時</label><input id="contest-end" v-model="endsAt" type="datetime-local" required></div></div>
              </section>
              <section class="settings-section"><h3>誤答ペナルティ</h3><p class="muted">正解した問題の、初回正解前の誤答だけに加算します。コンパイルエラーは対象外です。</p><label for="contest-penalty">誤答ペナルティ（分）</label><input id="contest-penalty" v-model.number="penaltyMinutes" type="number" required min="0" max="1440" step="1"><p class="muted penalty-note">0分でペナルティなしにできます。</p></section>
            </fieldset>
            <p class="muted">開始後は問題セット・配点・開催期間・ペナルティを変更できません。投稿済みのコンテストでは、問題と解説が終了後に自動公開されます。</p>
          </div>
        </section>
      </form>
    </div>
    <footer class="draft-status"><span role="status">{{ busy ? '保存中…' : locked ? '編集できません' : ready ? `${selected.length} 問 · 合計 ${totalPoints} 点` : '読み込み中…' }}</span><NuxtLink to="/my?tab=contests">自分のコンテスト</NuxtLink></footer>
  </div>
</template>
<style scoped>
/* Hallmark · pre-emit critique: P4 H4 E4 S5 R5 V4
 * modern-minimal · Workbench · existing Plain tokens and editor controls */
.visually-hidden { position: absolute; width: 1px; height: 1px; overflow: clip; clip-path: inset(50%); white-space: nowrap; }
.editor-topbar .author-actions { width: auto; flex-shrink: 0; }
.contest-editor .author-fields { grid-template-columns: minmax(0, 1fr); }
.draft-status > :last-child { display: inline-flex; white-space: nowrap; }
fieldset { border: 0; padding: 0; margin: 0; min-width: 0; }
.management-content > header h2 { font-size: 1.5rem; margin-bottom: 12px; }
.management-content h3 { font-size: 1rem; font-weight: 600; margin-bottom: 12px; }
.management-content h3 span { display: block; font-size: .8125rem; font-weight: 400; margin-top: 4px; }
.management-content p, .management-content label { font-size: .875rem; }
.problem-columns { display: grid; gap: 32px; }
.problem-picker label { display: flex; align-items: center; gap: 12px; min-height: 48px; padding: 10px 0; border-bottom: 1px solid var(--color-line); cursor: pointer; }
.problem-picker label:hover { background: var(--color-surface); }
.problem-picker input { flex-shrink: 0; accent-color: var(--color-accent); }
.problem-picker span, .selected strong { overflow-wrap: anywhere; min-width: 0; }
.selected { padding-left: 24px; margin: 0; }
.selected li { padding: 12px 0; border-bottom: 1px solid var(--color-line); }
.selected-heading { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 8px; margin-bottom: 8px; }
.move-actions { display: flex; gap: 4px; }
.move-actions button { width: 36px; min-height: 36px; }
.selection-note { margin-top: 24px; }
.management-content input:not([type=checkbox]) { width: 100%; min-width: 0; min-height: 44px; padding: 8px 10px; font: inherit; border: 1px solid var(--color-line); border-radius: 4px; color: var(--color-ink); background: var(--color-paper); }
.management-content input[type=number] { max-width: 128px; }
.management-content input:hover:not(:disabled) { border-color: var(--color-muted); }
.settings-section { padding-block: 24px; border-top: 1px solid var(--color-line); }
.settings-section label { display: block; margin-bottom: 8px; }
.dates { display: grid; gap: 20px; }
.penalty-note { margin-top: 8px; }
@media (min-width: 60rem) { .problem-columns, .dates { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
@media (pointer: coarse) { .move-actions button { width: 44px; min-height: 44px; } }
</style>
