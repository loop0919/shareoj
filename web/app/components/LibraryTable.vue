<script setup lang="ts">
import type { Folder, FolderRef } from '~~/shared/types/folders'
import { shortId } from '~~/shared/utils/short-id'
import { entryTitle, type LibraryEntry } from '~/utils/library-entries'
// subfolders are the open folder's own folders. They lead the table, and pressing one opens it.
const props = defineProps<{ entries: LibraryEntry[], subfolders: Folder[] }>()
// The refKeys of the chosen rows. ContentLibrary lets go of rows that leave the table.
const selected = defineModel<Set<string>>('selected', { required: true })
const emit = defineEmits<{ open: [id: string], move: [refs: FolderRef[]] }>()
const updatedLabel = (date: string) => new Date(date).toLocaleString('ja-JP', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', timeZone: 'Asia/Tokyo' })

const chosen = computed(() => props.entries.filter(entry => selected.value.has(entry.key)))
const allChosen = computed(() => props.entries.length > 0 && chosen.value.length === props.entries.length)
function choose(key: string, on: boolean) {
  const next = new Set(selected.value)
  if (on) next.add(key)
  else next.delete(key)
  selected.value = next
}
const chooseAll = (on: boolean) => { selected.value = new Set(on ? props.entries.map(entry => entry.key) : []) }
const allBox = ref<HTMLInputElement>()
function release() {
  selected.value = new Set()
  allBox.value?.focus()
}

// A chosen row carries every chosen row onto the folder list; any other row carries itself alone. A link inside a row
// drags its address as usual.
const { start, end } = useLibraryDrag()
const canDrag = useCanDrag()
function grab(entry: LibraryEntry, event: DragEvent) {
  if (event.target !== event.currentTarget) return
  start({ kind: 'items', refs: selected.value.has(entry.key) ? chosen.value.map(item => item.ref) : [entry.ref] }, event)
}

// When a moved row leaves the table, the row that takes its place, or else the last, gets the focus.
const scroll = ref<HTMLElement>()
function focusMove(index: number) {
  const buttons = scroll.value?.querySelectorAll<HTMLElement>('.library-move') ?? []
  const button = buttons[Math.min(index, buttons.length - 1)]
  button?.focus()
  return !!button
}
defineExpose({ focusMove })
</script>

<template>
  <div v-if="chosen.length" class="library-selection">
    <button type="button" class="editor-button primary" @click="emit('move', chosen.map(entry => entry.ref))">選んだ{{ chosen.length }}件をフォルダへ移動</button>
    <button type="button" class="editor-button" @click="release()">選択を解除</button>
  </div>
  <div ref="scroll" class="content-table-scroll">
    <table class="content-table content-table--cards library-table" aria-label="問題と記事">
      <thead><tr><th scope="col" class="select-cell"><input ref="allBox" type="checkbox" :checked="allChosen" :indeterminate="chosen.length > 0 && !allChosen" :disabled="!entries.length" aria-label="表示している問題と記事をすべて選択" @change="chooseAll(($event.target as HTMLInputElement).checked)"></th><th scope="col">種類</th><th scope="col">タイトル</th><th scope="col">状態</th><th scope="col">更新日時</th><th scope="col">難易度</th><th scope="col">操作</th></tr></thead>
      <tbody>
        <tr v-for="folder in subfolders" :key="folder.id" class="library-folder">
          <td class="select-cell" />
          <td class="lead-cell library-kind"><LibraryKind kind="folder" /></td>
          <th scope="row"><button type="button" class="library-folder-open" @click="emit('open', folder.id)">{{ folder.name }}</button></th>
          <td /><td /><td /><td />
        </tr>
        <tr v-for="entry in entries" :key="entry.key" :draggable="canDrag ? 'true' : undefined" @dragstart="grab(entry, $event)" @dragend="end()">
          <td class="select-cell"><input type="checkbox" :checked="selected.has(entry.key)" :aria-label="`${entryTitle(entry)}を選択`" @change="choose(entry.key, ($event.target as HTMLInputElement).checked)"></td>
          <td class="lead-cell library-kind"><LibraryKind :kind="entry.kind" /></td>
          <template v-if="entry.problem">
            <th scope="row">{{ entryTitle(entry) }}<span v-if="entry.kind === 'testing' && entry.problem.author" class="library-note">作者: {{ entry.problem.author }}</span><span v-if="entry.problem.viaContest" class="library-note">コンテスト「{{ entry.problem.viaContest.title.trim() || '無題のコンテスト' }}」の作成者として</span></th>
            <td class="line-cell" data-label="状態"><SavedProblemState :entry="entry.problem" /></td>
            <td class="corner-cell"><time :datetime="entry.problem.updatedAt">{{ updatedLabel(entry.problem.updatedAt) }}</time></td>
            <td class="lead-cell"><DifficultyBadge :level="entry.problem.difficulty" /></td>
          </template>
          <template v-else-if="entry.post">
            <th scope="row">{{ entryTitle(entry) }}</th>
            <td class="line-cell" data-label="状態">{{ entry.post.publishedVersion ? '公開中' : '下書き' }}</td>
            <td class="corner-cell"><time :datetime="entry.post.updatedAt">{{ updatedLabel(entry.post.updatedAt) }}</time></td>
            <td />
          </template>
          <td class="actions-cell">
            <div class="library-actions">
              <SavedProblemActions v-if="entry.problem" :entry="entry.problem" :testing="entry.kind === 'testing'" />
              <ContentActions v-else-if="entry.post" :title="entryTitle(entry)" :edit-to="{ path: '/blog/new', query: { post: entry.post.id } }" :view-to="`/blog/${shortId(entry.post.id)}`" :published="!!entry.post.publishedVersion" />
              <button type="button" class="editor-button library-move" :aria-label="`${entryTitle(entry)}をフォルダへ移動`" title="フォルダへ移動" @click="emit('move', [entry.ref])">
                <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2Z" /><path d="M10 13h6m-2-2.5 2.5 2.5-2.5 2.5" /></svg>
              </button>
            </div>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
