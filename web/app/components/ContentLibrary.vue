<script setup lang="ts">
import { folderCounts, folderPath, sortedChildren } from '~/utils/folder-tree'
import { libraryKinds, problemStatusGroup, problemStatusOptions, type LibraryEntry, type LibraryKind } from '~/utils/library-entries'
const route = useRoute()
const router = useRouter()
const { entries, folders, placed, message, load } = useLibrary()

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
// Writes what is shown, so that leaving an old tab's URL keeps its kind and drops what was not understood.
function show(changes: { folder?: string, kind?: string, status?: string }) {
  const next = { folder: selected.value, kind: kindFilter.value, status: statusFilter.value, ...changes }
  void router.replace({ query: { ...route.query, tab: 'library', folder: next.folder || undefined, kind: next.kind || undefined, status: next.status || undefined } })
}

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

// The tree waits for every list, since counts made from part of them would be too small.
const loaded = ref(false)
const failed = ref(false)
async function reload() {
  failed.value = false
  await load()
  failed.value = !!message.value
  if (!failed.value) loaded.value = true
}
onMounted(() => { void reload() })
</script>

<template>
  <section class="draft-library library">
    <header class="draft-library-heading library-heading">
      <h2>問題と記事</h2>
      <NuxtLink class="editor-button primary" to="/problems/new?fresh=1">新規問題</NuxtLink>
      <NuxtLink class="editor-button" to="/blog/new">新規記事</NuxtLink>
    </header>
    <p v-if="message" role="alert" class="editor-error">{{ message }}</p>
    <button v-if="failed" type="button" class="editor-button" @click="reload()">再試行</button>
    <LoadingSpinner v-if="!loaded && !failed" label="問題と記事を読み込み中" />
    <div v-else-if="loaded" class="library-body">
      <FolderTree v-model:selected="selected" :folders="folders" :counts="treeCounts" />
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
        <div v-if="inFolder.length" class="library-filters">
          <StatusFilter v-model="kindFilter" :options="libraryKinds" :counts="kindCounts" label="種類で絞り込む" />
          <StatusFilter v-model="statusFilter" :options="problemStatusOptions" :counts="statusCounts" />
        </div>
        <LibraryTable v-if="subfolders.length || visible.length" :entries="visible" :subfolders="subfolders" @open="selected = $event" />
        <EmptyState v-if="emptyTitle && inFolder.length" kind="problem" :title="emptyTitle"><button type="button" class="editor-button" @click="show({ kind: '', status: '' })">すべて表示</button></EmptyState>
        <EmptyState v-else-if="emptyTitle" kind="problem" :title="emptyTitle" :description="selected ? undefined : '「新規問題」「新規記事」から作成すると、ここに表示されます。'" />
      </div>
    </div>
  </section>
</template>

<style src="../assets/css/library.css"></style>
