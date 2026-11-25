<script setup lang="ts">
import type { FolderRef } from '~~/shared/types/folders'
import { folderCounts, folderPath, refKey, sortedChildren } from '~/utils/folder-tree'
import { entryTitle, libraryKinds, problemStatusGroup, problemStatusOptions, type LibraryEntry, type LibraryKind } from '~/utils/library-entries'
const route = useRoute()
const router = useRouter()
const { entries, folders, placed, loading, loadError, message, load, createFolder, updateFolder, deleteFolder, place } = useLibrary()

// The URL holds the open folder, kind and status, so a reload or the back button shows the same list. A value this
// page does not know counts as absent. The old tabs' URLs still land here, narrowed to their kind.
const tabKinds: Record<string, LibraryKind> = { problems: 'own', testing: 'testing', posts: 'post' }
const queryValue = (key: string) => typeof route.query[key] === 'string' ? route.query[key] : ''
const selected = computed({
  get: () => {
    const folder = queryValue('folder')
    return folder === 'unsorted' || folders.value.some(item => item.id === folder) ? folder : ''
  },
  set: folder => { show({ folder }) },
})
const kindQuery = computed({
  get: () => {
    const kind = queryValue('kind')
    return libraryKinds.some(option => option.key === kind) ? kind : tabKinds[queryValue('tab')] ?? ''
  },
  set: kind => { show({ kind }) },
})
const statusQuery = computed({ get: () => queryValue('status'), set: status => { show({ status }) } })
// Writes what is shown, so that leaving an old tab's URL keeps its kind and drops what was not understood. Another
// folder or filter starts afresh, so a failed write's message goes; the back and forward buttons do the same.
function show(changes: { folder?: string, kind?: string, status?: string }) {
  message.value = ''
  const next = { folder: selected.value, kind: kindFilter.value, status: statusFilter.value, ...changes }
  void router.replace({ query: { ...route.query, tab: 'library', folder: next.folder || undefined, kind: next.kind || undefined, status: next.status || undefined } })
}
const forget = () => { message.value = '' }
onMounted(() => window.addEventListener('popstate', forget))
onBeforeUnmount(() => window.removeEventListener('popstate', forget))

// The open folder holds only what was put in it directly; its own folders lead the table instead.
const inFolder = computed(() => {
  const folder = selected.value
  if (!folder) return entries.value
  const known = new Set(folders.value.map(item => item.id))
  return entries.value.filter(entry => {
    const placedIn = placed.value.get(entry.key)
    return folder === 'unsorted' ? placedIn === undefined || !known.has(placedIn) : placedIn === folder
  })
})
const subfolders = computed(() => selected.value && selected.value !== 'unsorted' ? sortedChildren(folders.value, selected.value) : [])
const path = computed(() => selected.value && selected.value !== 'unsorted' ? folderPath(folders.value, selected.value) : [])
const treeCounts = computed(() => folderCounts(folders.value, placed.value, entries.value.map(entry => entry.key)))

// Each filter counts within the open folder and the other filter, so a number is the rows its button shows.
// A problem's status is read again, since withdrawing from 定期便 changes it in place.
const statusOf = (entry: LibraryEntry) => entry.problem ? problemStatusGroup(entry.problem) : entry.status
const { filter: kindFilter, counts: kindCounts, visible } = useStatusFilter(libraryKinds, kindQuery, () => inFolder.value.filter(entry => !statusFilter.value || statusOf(entry) === statusFilter.value), entry => entry.kind)
const { filter: statusFilter, counts: statusCounts } = useStatusFilter(problemStatusOptions, statusQuery, () => inFolder.value.filter(entry => !kindFilter.value || entry.kind === kindFilter.value), statusOf)
const emptyTitle = computed(() => {
  if (visible.value.length) return ''
  if (inFolder.value.length) return '絞り込みに合う問題と記事はありません。'
  if (subfolders.value.length) return ''
  if (!selected.value) return '問題と記事はまだありません。'
  return selected.value === 'unsorted' ? '未整理の問題と記事はありません。' : 'このフォルダは空です。'
})

// The chosen rows, as refKeys. A row the folder or the filters no longer show is let go, so a move takes only rows in
// sight.
const chosen = ref(new Set<string>())
watch(visible, list => {
  const shown = new Set(list.map(entry => entry.key))
  if ([...chosen.value].some(key => !shown.has(key))) chosen.value = new Set([...chosen.value].filter(key => shown.has(key)))
})

// Problems and posts go where the picker says. It starts on their folder when they share one.
const root = ref<HTMLElement>()
const table = ref<{ focusMove: (index: number) => boolean }>()
const picker = ref<{ open: (current?: string | null) => Promise<string | null | undefined> }>()
const pickerTitle = ref('')
const folderOf = (key: string) => {
  const folderId = placed.value.get(key)
  return folderId !== undefined && folders.value.some(folder => folder.id === folderId) ? folderId : null
}
async function move(refs: FolderRef[]) {
  const keys = refs.map(refKey)
  const first = visible.value.find(entry => entry.key === keys[0])
  if (!first) return
  pickerTitle.value = refs.length === 1 ? `「${entryTitle(first)}」をフォルダへ移動` : `選んだ${refs.length}件をフォルダへ移動`
  const places = new Set(keys.map(folderOf))
  const trigger = document.activeElement
  const index = visible.value.indexOf(first)
  const folderId = await picker.value?.open(places.size === 1 ? [...places][0] : undefined)
  if (folderId === undefined) return
  await moveTo(refs, folderId, trigger, index)
}
// Rows dropped on the folder list need no picker; the focus stays where it was unless a moved row took it along.
function drop(refs: FolderRef[], folderId: string | null) {
  const keys = new Set(refs.map(refKey))
  void moveTo(refs, folderId, document.activeElement, Math.max(0, visible.value.findIndex(entry => keys.has(entry.key))))
}
// Moved rows leave the open folder at once and so the selection too; a refusal brings them back still chosen.
async function moveTo(refs: FolderRef[], folderId: string | null, trigger: Element | null, index: number) {
  const keys = refs.map(refKey)
  const before = chosen.value
  const moving = place(refs, folderId)
  await keepFocus(trigger, index)
  if (!await moving) {
    const shown = new Set(visible.value.map(entry => entry.key))
    chosen.value = new Set([...before].filter(key => shown.has(key)))
    return
  }
  const moved = new Set(keys)
  chosen.value = new Set([...chosen.value].filter(key => !moved.has(key)))
  await keepFocus(trigger, index)
}
// A row that moved out of the open folder takes its button with it, and so does the selection bar once the selection
// is let go. The row now in its place, or else the open folder in the list, gets the focus, unless it went elsewhere.
async function keepFocus(trigger: Element | null, index: number) {
  await nextTick()
  if (!trigger || trigger.isConnected || (document.activeElement && document.activeElement !== document.body)) return
  if (!table.value?.focusMove(index)) root.value?.querySelector<HTMLElement>('.folder-tree-item[aria-current="true"]')?.focus()
}

// The name dialog shows a refused name itself and stays open, so the refusal is not repeated above the table.
async function saveName(id: string | null, name: string, parentId: string | null) {
  let refusal = ''
  const refused = (text: string) => { refusal = text }
  if (id === null) await createFolder(name, parentId, refused)
  else await updateFolder(id, name, parentId, refused)
  return refusal
}

// The tree waits for every list, since counts made from part of them would be too small. Once shown, the list stays;
// a refused write only sets message.
const loaded = ref(false)
async function reload() {
  await load()
  if (!loadError.value) loaded.value = true
}
onMounted(() => { void reload() })
</script>

<template>
  <section ref="root" class="draft-library library">
    <header class="draft-library-heading library-heading">
      <h2>問題と記事</h2>
      <NuxtLink class="editor-button primary" to="/problems/new?fresh=1">新規問題</NuxtLink>
      <NuxtLink class="editor-button" to="/blog/new">新規記事</NuxtLink>
    </header>
    <p v-if="loadError" role="alert" class="editor-error">{{ loadError }}</p>
    <button v-if="loadError" type="button" class="editor-button" @click="reload()">再試行</button>
    <LoadingSpinner v-if="loading && !loaded" label="問題と記事を読み込み中" />
    <div v-else-if="loaded" class="library-body">
      <FolderTree v-model:selected="selected" :folders="folders" :counts="treeCounts" :placed="placed" :save-name="saveName" @update="updateFolder" @remove="deleteFolder" @place="drop" />
      <div class="library-content">
        <nav class="library-location" aria-label="今いる場所">
          <span v-if="!selected" aria-current="location">すべて</span>
          <span v-else-if="selected === 'unsorted'" aria-current="location">未整理</span>
          <template v-else>
            <template v-for="(folder, index) in path" :key="folder.id">
              <span v-if="index" aria-hidden="true"> / </span>
              <span v-if="index === path.length - 1" aria-current="location">{{ folder.name }}</span>
              <button v-else type="button" @click="selected = folder.id">{{ folder.name }}</button>
            </template>
          </template>
        </nav>
        <p v-if="message" role="alert" class="editor-error">{{ message }}</p>
        <div v-if="inFolder.length" class="library-filters">
          <StatusFilter v-model="kindFilter" :options="libraryKinds" :counts="kindCounts" label="種類で絞り込む" />
          <StatusFilter v-model="statusFilter" :options="problemStatusOptions" :counts="statusCounts" />
        </div>
        <LibraryTable v-if="subfolders.length || visible.length" ref="table" v-model:selected="chosen" :entries="visible" :subfolders="subfolders" @open="selected = $event" @move="move" />
        <EmptyState v-if="emptyTitle && inFolder.length" kind="problem" :title="emptyTitle"><button type="button" class="editor-button" @click="show({ kind: '', status: '' })">すべて表示</button></EmptyState>
        <EmptyState v-else-if="emptyTitle" kind="problem" :title="emptyTitle" :description="selected ? undefined : '「新規問題」「新規記事」から作成すると、ここに表示されます。'" />
      </div>
      <FolderPicker ref="picker" :folders="folders" :title="pickerTitle" />
    </div>
  </section>
</template>

<style src="../assets/css/library.css"></style>
