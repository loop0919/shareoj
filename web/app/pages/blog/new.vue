<script setup lang="ts">
import { postSchema } from '~~/shared/types/post'
import { accountError } from '~/utils/account-problems'
useSeoMeta({ title: '記事を書く | ShareOJ', robots: 'noindex, nofollow' })
const route = useRoute()
const router = useRouter()
const { refreshAccount } = useAccount()
const title = ref('')
const markdown = ref('<!-- ここに記事を記載 -->\n')
const id = ref('')
const version = ref(0)
const publishedVersion = ref(0)
const ready = ref(false)
const busy = ref(false)
definePageMeta({ editorLayout: true })
const managing = ref(false)
const sidebarExpanded = ref(false)
const saveLocation = computed(() => publishedVersion.value ? '公開中' : '非公開')
const {
  mode, workspace, splitPercent, resizing, setSplit, startResize, moveResize, stopResize, resizeWithKeyboard,
  editor, syncSource, insertSnippet,
} = useMarkdownEditor()

const error = ref('')
const status = ref('記事を読み込んでいます…')
let owner = ''
let saved = ''
let leaving = false
const snapshot = () => JSON.stringify({ title: title.value, markdown: markdown.value })
function failure(e: unknown) {
  if ((e as { data?: { data?: { code?: string } } }).data?.data?.code === 'creation_quota_exceeded') return accountError(e)
  const code = (e as { statusCode?: number }).statusCode
  return code === 409 ? '別の画面で更新されています。入力内容をコピーしてから再読み込みしてください。' : code === 401 ? 'ログインし直してから保存してください。' : '処理に失敗しました。入力内容を残したまま、もう一度お試しください。'
}
onMounted(async () => {
  try {
    const account = await refreshAccount()
    if (!account) { await navigateTo('/login?next=/blog/new'); return }
    owner = account.id
    if (typeof route.query.post === 'string') {
      const post = postSchema.parse(await $fetch(`/api/my/posts/${encodeURIComponent(route.query.post)}`))
      id.value = post.id; version.value = post.version; publishedVersion.value = post.publishedVersion
      title.value = post.title; markdown.value = post.markdown
    }
    saved = snapshot(); ready.value = true; status.value = id.value ? '保存済み' : '記事を書き始められます'
  } catch { error.value = '記事を読み込めませんでした。アクセス権と接続を確認してください。' }
})
async function save(): Promise<boolean> {
  if (!ready.value || busy.value) return false
  busy.value = true; error.value = ''
  try {
    const account = await refreshAccount()
    if (!account || account.id !== owner) throw { statusCode: 401 }
    id.value ||= crypto.randomUUID()
    const post = postSchema.parse(await $fetch(`/api/my/posts/${id.value}`, { method: 'PUT', body: { version: version.value, title: title.value, markdown: markdown.value } }))
    version.value = post.version; publishedVersion.value = post.publishedVersion; saved = snapshot(); status.value = '保存済み'
    if (route.query.post !== id.value) await router.replace({ path: '/blog/new', query: { post: id.value } })
    return true
  } catch (e) { error.value = failure(e); return false }
  finally { busy.value = false }
}
async function publish(value: boolean) {
  if (value && (!title.value.trim() || !markdown.value.trim())) { error.value = '公開するにはタイトルと本文を入力してください。'; return }
  if (!await save()) return
  if (!window.confirm(value ? '現在の内容を公開しますか？誰でも閲覧できるようになります。' : 'この記事を非公開に戻しますか？')) return
  busy.value = true
  try {
    const post = postSchema.parse(await $fetch(`/api/my/posts/${id.value}/publication`, { method: 'PUT', body: { version: version.value, publish: value } }))
    version.value = post.version; publishedVersion.value = post.publishedVersion; status.value = value ? '公開しました' : '非公開に戻しました'
  } catch (e) { error.value = failure(e) }
  finally { busy.value = false }
}
async function remove() {
  if (!id.value || !window.confirm('記事と公開内容を削除します。この操作は取り消せません。削除しますか？')) return
  busy.value = true
  try {
    await $fetch(`/api/my/posts/${id.value}`, { method: 'DELETE', query: { version: version.value } })
    leaving = true; busy.value = false; await navigateTo('/my?tab=posts')
  } catch (e) { error.value = failure(e) }
  finally { busy.value = false }
}
watch([title, markdown], () => { if (ready.value && snapshot() !== saved) status.value = '未保存の変更があります' })
onBeforeRouteLeave(() => leaving || (!busy.value && (!ready.value || snapshot() === saved || window.confirm('未保存の変更を破棄して移動しますか？'))))
function beforeUnload(event: BeforeUnloadEvent) { if (ready.value && snapshot() !== saved) { event.preventDefault(); event.returnValue = '' } }
onMounted(() => window.addEventListener('beforeunload', beforeUnload))
onBeforeUnmount(() => window.removeEventListener('beforeunload', beforeUnload))


const mathSnippet = '\n```math\n\\sum_{i=1}^{N} A_i\n```\n'
function saveWithShortcut(event: KeyboardEvent) {
  if (event.isComposing || event.altKey || event.shiftKey || !(event.ctrlKey || event.metaKey) || event.key.toLowerCase() !== 's') return
  event.preventDefault()
  if (!event.repeat) void save()
}
onMounted(() => {
  syncSource()
  window.addEventListener('keydown', saveWithShortcut)
})
onBeforeUnmount(() => {
  window.removeEventListener('keydown', saveWithShortcut)
})
</script>

<template>
  <div class="author-page">
    <header class="editor-topbar">
      <NuxtLink class="wordmark" to="/my" aria-label="ShareOJ マイページ">Share<span>OJ</span><span class="wordmark-beta">(β)</span></NuxtLink>
      <ThemeSelect />
      <div v-show="!managing" class="editor-view-switch" aria-label="表示の切り替え">
        <button type="button" class="editor-button" :aria-pressed="mode === 'edit'" aria-label="編集" title="編集" @click="mode = 'edit'"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="m15 4 5 5M4 20l4-1L20 7a2 2 0 0 0-4-4L4 15Z" /></svg></button>
        <button type="button" class="editor-button split-button" :aria-pressed="mode === 'split'" aria-label="分割" title="分割" @click="mode = 'split'"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><rect x="3" y="3" width="18" height="18" rx="2" /><path d="M12 3v18" /></svg></button>
        <button type="button" class="editor-button" :aria-pressed="mode === 'preview'" aria-label="プレビュー" title="プレビュー" @click="mode = 'preview'"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12Z" /><circle cx="12" cy="12" r="3" /></svg></button>
      </div>
      <div class="author-actions">
        <div class="editor-save-actions">
        <button type="button" class="editor-button primary save-button" :disabled="!ready || busy" :aria-busy="busy" :aria-label="busy ? '保存中' : '保存'" title="保存（Ctrl+S / ⌘S）" aria-keyshortcuts="Control+s Meta+s" @click="save()">
          <span :class="{ 'save-label-hidden': busy }">保存</span>
          <span v-if="busy" class="save-spinner" aria-hidden="true" />
        </button>
        </div>
      </div>
    </header>
    <div class="author-body" :data-sidebar-expanded="sidebarExpanded">
      <aside class="editor-sidebar" aria-label="記事作成サイドバー">
        <button type="button" class="editor-button editor-sidebar-toggle" :aria-expanded="sidebarExpanded" aria-controls="editor-section-nav" :aria-label="sidebarExpanded ? 'サイドバーを折りたたむ' : 'サイドバーを展開'" :title="sidebarExpanded ? 'サイドバーを折りたたむ' : 'サイドバーを展開'" @click="sidebarExpanded = !sidebarExpanded">
          <svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><rect x="3" y="3" width="18" height="18" rx="2" /><path d="M9 3v18" /><path :d="sidebarExpanded ? 'm15 9-3 3 3 3' : 'm13 9 3 3-3 3'" /></svg>
        </button>
        <nav id="editor-section-nav" class="editor-section-nav" aria-label="記事作成メニュー">
          <button type="button" class="editor-button editor-sidebar-item" :aria-current="!managing ? 'page' : undefined" aria-label="本文" title="本文" @click="managing = false"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M14 2H5v20h14V7Zm0 0v5h5M8 12h8M8 16h6" /></svg><span class="editor-sidebar-label">本文</span></button>

          <button type="button" class="editor-button editor-sidebar-item" :disabled="!ready || busy" :aria-current="managing ? 'page' : undefined" aria-label="記事管理" title="記事管理" @click="managing = true"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="m9 3 .6-2h4.8l.6 2 2 1.2 2.1-.5 2.4 4.2-1.5 1.5v2.3l1.5 1.5-2.4 4.2-2.1-.5-2 1.2-.6 2H9l-.6-2-2-1.2-2.1.5-2.4-4.2 1.5-1.5V9.4L1.9 7.9l2.4-4.2 2.1.5Z" transform="translate(0 1)" /><circle cx="12" cy="12" r="3" /></svg><span class="editor-sidebar-label">記事管理</span></button>
        </nav>
      </aside>
      <div class="editor-main">
    <div class="editor-notices">
      <p v-if="error" class="editor-error" role="alert">{{ error }}</p>
      <noscript><p class="editor-error">編集と保存には JavaScript を有効にしてください。</p></noscript>
    </div>
    <div v-show="!managing" class="author-edit-content">
    <div class="author-fields post-fields">
      <div class="title-field">
        <div class="field-heading"><label for="post-title">タイトル</label></div>
        <input id="post-title" v-model="title" maxlength="120" placeholder="記事のタイトル" :disabled="!ready || busy">
      </div>
    </div>
    <div ref="workspace" class="author-workspace" :class="{ 'is-resizing': resizing }" :data-mode="mode" :style="{ '--editor-left': `${splitPercent}fr`, '--editor-right': `${100 - splitPercent}fr` }">
      <section id="source-pane" class="source-pane" aria-label="Markdown 編集">
        <div class="pane-heading"><div class="source-heading-label"><label id="post-body-label" for="post-body" @click="editor?.focus()">本文（Markdown）</label><NuxtLink class="source-guide-link" to="/blog/markdown-guide" target="_blank" rel="noopener noreferrer" aria-label="Markdown・数式の書き方" title="Markdown・数式の書き方"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="9" /><path d="M9.5 9a2.5 2.5 0 0 1 5 .5c0 1.5-2.5 2-2.5 3.5M12 16h.01" /></svg></NuxtLink></div><span>{{ markdown.length.toLocaleString('en-US') }} / 100,000</span></div>
        <div class="editor-toolbar" aria-label="記法を挿入">
          <button type="button" :disabled="!ready || busy" @click="insertSnippet('\n## 見出し\n')" aria-label="見出し" title="見出し"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M5 5v14M19 5v14M5 12h14" /></svg></button>
          <button type="button" :disabled="!ready || busy" @click="insertSnippet('**強調**')" aria-label="太字" title="太字"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path stroke-width="2.4" d="M6 12h7a4 4 0 0 1 0 8H6V4h6a4 4 0 0 1 0 8" /></svg></button>
          <button type="button" :disabled="!ready || busy" @click="insertSnippet(mathSnippet)" aria-label="数式" title="数式"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M19 4H6l7 8-7 8h13" /></svg></button>
          <button type="button" :disabled="!ready || busy" @click="insertSnippet('\n```text\nコード\n```\n')" aria-label="コード" title="コード"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="m8 6-6 6 6 6m8-12 6 6-6 6m-3-14-2 16" /></svg></button>
          <button type="button" :disabled="!ready || busy" @click="insertSnippet('\n:::details タイトル\n内容\n:::\n')" aria-label="折りたたみ" title="折りたたみ"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><rect x="3" y="4" width="18" height="16" rx="2" /><path d="m7 8 3 3 3-3M7 15h10" /></svg></button>
          <button type="button" :disabled="!ready || busy || editor?.uploading" :aria-expanded="editor?.showImages ?? false" @click="editor?.toggleImages()" aria-label="画像" title="画像"><svg class="editor-icon" viewBox="0 0 24 24" aria-hidden="true"><rect x="3" y="3" width="18" height="18" rx="2" /><circle cx="8" cy="8" r="1.5" /><path d="m21 15-5-5L5 21M3 16l4-4 4 4" /></svg></button>
          <EditorSettings kind="markdown" />
        </div>
        <MarkdownSourceEditor id="post-body" ref="editor" v-model="markdown" label-id="post-body-label" :disabled="!ready || busy" />
      </section>
      <div class="split-handle" role="separator" tabindex="0" aria-label="編集欄とプレビューの幅を調整" aria-orientation="vertical" aria-controls="source-pane preview-pane" :aria-valuenow="Math.round(splitPercent)" :aria-valuetext="`編集欄 ${Math.round(splitPercent)}%、プレビュー ${100 - Math.round(splitPercent)}%`" :aria-valuemin="30" :aria-valuemax="70" title="ドラッグで幅を調整・ダブルクリックで均等に戻す" @pointerdown="startResize" @pointermove="moveResize" @pointerup="stopResize" @pointercancel="stopResize" @lostpointercapture="resizing = false" @keydown="resizeWithKeyboard" @dblclick="setSplit(50)" />
      <section id="preview-pane" class="preview-pane" aria-label="記事のプレビュー">
        <div class="pane-heading"><h2>プレビュー</h2><span>表示を確認</span></div>
        <div class="preview-document post-preview">
          <p class="preview-title">{{ title || '無題の記事' }}</p>
          <ProblemMarkdown v-if="markdown.trim()" :source="markdown" />
          <p v-else class="muted">本文を書くと、ここにプレビューが表示されます。</p>
        </div>
      </section>
    </div>
    </div>
    <section v-if="managing" class="problem-management" aria-labelledby="management-title">
      <div class="management-content">
        <NuxtLink to="/my?tab=posts">自分の記事</NuxtLink>
        <header><h1 id="management-title">記事管理</h1><p class="manage-problem-title">{{ title.trim() || '無題の記事' }}</p><p class="muted">{{ saveLocation }}</p></header>
        <section class="management-row"><div><h2>公開設定</h2><p>公開内容は「公開内容を更新」を押すまで変わりません。</p><NuxtLink v-if="publishedVersion" :to="`/blog/${id}`" target="_blank">公開ページを見る</NuxtLink></div><div class="publication-actions"><button class="editor-button primary" :disabled="busy" @click="publish(true)">{{ publishedVersion ? '公開内容を更新' : '公開する' }}</button><button v-if="publishedVersion" class="editor-button" :disabled="busy" @click="publish(false)">非公開に戻す</button></div></section>
        <section class="management-row"><div><h2>記事の削除</h2><p>記事を削除します。この操作は取り消せません。</p></div><button v-if="version" type="button" class="editor-button danger" :disabled="busy" @click="remove">削除</button></section>
      </div>
    </section>
      </div>
    </div>
    <div class="draft-status"><span role="status">{{ status }}</span><span>{{ saveLocation }}</span></div>
  </div>
</template>

<style scoped>
.author-fields.post-fields { grid-template-columns: minmax(0, 1fr); }
</style>
