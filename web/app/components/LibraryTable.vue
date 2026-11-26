<script setup lang="ts">
import type { Folder, FolderRef } from '~~/shared/types/folders'
import { shortId } from '~~/shared/utils/short-id'
import { entryTitle, libraryKinds, type LibraryEntry, type LibraryKind, type LibrarySort } from '~/utils/library-entries'
import { japanMinute } from '~/utils/japan-time'
// subfolders are the open folder's own folders. They lead the table, and pressing one opens it.
// The table is laid out as a list: the folder list beside it leaves no room for a column per fact, so a row's state,
// update time and difficulty run in a line under its title and wrap. The roles are written out, since a table that is
// not displayed as one loses them.
// sort is the order the rows came in. A row shows the time it was ordered by, so the order can be read off the list.
const props = defineProps<{ entries: LibraryEntry[], subfolders: Folder[], sort: LibrarySort }>()
// The refKeys of the chosen rows. ContentLibrary lets go of rows that leave the table.
const selected = defineModel<Set<string>>('selected', { required: true })
const emit = defineEmits<{ open: [id: string], move: [refs: FolderRef[]] }>()
const kindLabels = Object.fromEntries(libraryKinds.map(kind => [kind.key, kind.label])) as Record<LibraryKind, string>
// By publication, a row that is not published yet has no such time, and shows its last update as the rest of its
// group is ordered.
function shownTime(entry: LibraryEntry) {
  if (props.sort === 'created') return { label: '作成', at: entry.createdAt }
  if (props.sort === 'published' && entry.publishedAt) return { label: '公開', at: entry.publishedAt }
  return { label: '更新', at: entry.updatedAt }
}

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

// What a row is seldom asked for waits behind its ⋯ button, as a folder's does in the folder list: one menu is open at
// a time, and it closes on Esc, on a press outside, or when the focus leaves it.
const menuId = useId()
const menu = ref('')
const menuButtons = new Map<string, HTMLElement>()
function closeMenu(refocus = false) {
  const key = menu.value
  menu.value = ''
  if (refocus) menuButtons.get(key)?.focus()
}
function leaveMenu(event: FocusEvent) {
  const actions = event.currentTarget as HTMLElement
  if (!(event.relatedTarget instanceof Node && actions.contains(event.relatedTarget))) closeMenu()
}
function outside(event: PointerEvent) {
  if (menu.value && event.target instanceof Element && !event.target.closest('.library-menu-wrap')) closeMenu()
}
onMounted(() => document.addEventListener('pointerdown', outside))
onBeforeUnmount(() => document.removeEventListener('pointerdown', outside))
// The focus goes back to the ⋯ button first, so the move starts from a button that stays while the row does.
function moveFromMenu(entry: LibraryEntry) {
  closeMenu(true)
  emit('move', [entry.ref])
}
// The withdrawal's dialog belongs to the row's problem actions; the focus is on the ⋯ button when it opens, and comes
// back there when it closes.
const problemActions = new Map<string, { confirmWithdraw: () => Promise<void> }>()
function withdrawFromMenu(entry: LibraryEntry) {
  closeMenu(true)
  void problemActions.get(entry.key)?.confirmWithdraw()
}

// When a moved row leaves the table, the row that takes its place, or else the last, gets the focus.
const scroll = ref<HTMLElement>()
function focusMove(index: number) {
  const buttons = scroll.value?.querySelectorAll<HTMLElement>('.library-menu-button') ?? []
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
  <div ref="scroll" class="library-list">
    <table class="library-table" role="table" aria-label="問題と記事">
      <thead role="rowgroup"><tr role="row"><th role="columnheader" scope="col" class="select-cell"><input ref="allBox" type="checkbox" :checked="allChosen" :indeterminate="chosen.length > 0 && !allChosen" :disabled="!entries.length" aria-label="表示している問題と記事をすべて選択" @change="chooseAll(($event.target as HTMLInputElement).checked)"></th><th role="columnheader" scope="col" class="visually-hidden">種類</th><th role="columnheader" scope="col" class="visually-hidden">タイトル</th><th role="columnheader" scope="col" class="visually-hidden">状態と更新日時</th><th role="columnheader" scope="col" class="visually-hidden">操作</th></tr></thead>
      <tbody role="rowgroup">
        <tr v-for="folder in subfolders" :key="folder.id" role="row" class="library-folder">
          <td role="cell" class="select-cell" />
          <td role="cell" class="library-kind"><LibraryKind kind="folder" /></td>
          <th role="rowheader" scope="row"><button type="button" class="library-folder-open" @click="emit('open', folder.id)">{{ folder.name }}</button></th>
          <td role="cell" class="library-meta" /><td role="cell" class="actions-cell" />
        </tr>
        <tr v-for="entry in entries" :key="entry.key" role="row" :draggable="canDrag ? 'true' : undefined" @dragstart="grab(entry, $event)" @dragend="end()">
          <td role="cell" class="select-cell"><input type="checkbox" :checked="selected.has(entry.key)" :aria-label="`${entryTitle(entry)}を選択`" @change="choose(entry.key, ($event.target as HTMLInputElement).checked)"></td>
          <td role="cell" class="library-kind"><LibraryKind :kind="entry.kind" /></td>
          <th role="rowheader" scope="row">{{ entryTitle(entry) }}<template v-if="entry.problem"><span v-if="entry.kind === 'testing' && entry.problem.author" class="library-note">作者: {{ entry.problem.author }}</span><span v-if="entry.problem.viaContest" class="library-note">コンテスト「{{ entry.problem.viaContest.title.trim() || '無題のコンテスト' }}」の作成者として</span></template></th>
          <td role="cell" class="library-meta">
            <!-- Where nothing hovers, the kind's name has no pointer to bring it out, so it leads this line. -->
            <span class="library-meta-kind" aria-hidden="true">{{ kindLabels[entry.kind] }}</span>
            <template v-if="entry.problem">
              <SavedProblemState :entry="entry.problem" />
              <span class="library-meta-item">{{ shownTime(entry).label }} <time :datetime="shownTime(entry).at">{{ japanMinute(shownTime(entry).at) }}</time></span>
              <span class="library-meta-item">難易度 <DifficultyBadge :level="entry.problem.difficulty" /></span>
            </template>
            <template v-else-if="entry.post">
              <span class="library-post-state" :data-published="entry.post.publishedVersion ? '' : undefined">{{ entry.post.publishedVersion ? '公開中' : '下書き' }}</span>
              <span class="library-meta-item">{{ shownTime(entry).label }} <time :datetime="shownTime(entry).at">{{ japanMinute(shownTime(entry).at) }}</time></span>
            </template>
          </td>
          <td role="cell" class="actions-cell">
            <div class="library-actions">
              <SavedProblemActions v-if="entry.problem" :ref="el => { if (el) problemActions.set(entry.key, el as unknown as { confirmWithdraw: () => Promise<void> }); else problemActions.delete(entry.key) }" :entry="entry.problem" :testing="entry.kind === 'testing'" extras-elsewhere />
              <ContentActions v-else-if="entry.post" :title="entryTitle(entry)" :edit-to="{ path: '/blog/new', query: { post: entry.post.id } }" :view-to="`/blog/${shortId(entry.post.id)}`" :published="!!entry.post.publishedVersion" />
              <span class="library-menu-wrap" @focusout="leaveMenu" @keydown.esc="closeMenu(true)">
                <button :ref="el => { if (el) menuButtons.set(entry.key, el as HTMLElement); else menuButtons.delete(entry.key) }" type="button" class="editor-button library-menu-button" :aria-label="`${entryTitle(entry)}の操作`" title="その他の操作" :aria-expanded="menu === entry.key" :aria-controls="menu === entry.key ? menuId : undefined" @click="menu = menu === entry.key ? '' : entry.key">
                  <svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><circle cx="5" cy="12" r="1.8" /><circle cx="12" cy="12" r="1.8" /><circle cx="19" cy="12" r="1.8" /></svg>
                </button>
                <span v-if="menu === entry.key" :id="menuId" class="library-menu" role="group" :aria-label="`${entryTitle(entry)}の操作`">
                  <button type="button" @click="moveFromMenu(entry)">フォルダへ移動</button>
                  <NuxtLink v-if="entry.kind === 'own' && entry.problem?.scheduledAt" :to="{ path: '/problems/post', query: { problem: entry.problem.id } }">予約を変更</NuxtLink>
                  <button v-if="entry.kind === 'own' && entry.problem?.featuredPreference" type="button" @click="withdrawFromMenu(entry)">定期便の応募を取り下げる</button>
                </span>
              </span>
            </div>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
