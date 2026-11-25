<script setup lang="ts">
import type { Folder } from '~~/shared/types/folders'
import { vDialogDismiss } from '~/utils/dialog-dismiss'
import { canNest, descendantIds, folderPath, sortedChildren } from '~/utils/folder-tree'
// counts comes from folderCounts and is not narrowed by the filters, so a folder's number is what it holds. saveName
// makes a folder (id null) or renames one and resolves to '' once saved or else to why it was refused, which the name
// dialog shows while it stays open.
const props = defineProps<{ folders: Folder[], counts: { all: number, unsorted: number, byFolder: Map<string, number> }, saveName: (id: string | null, name: string, parentId: string | null) => Promise<string> }>()
// '' is everything and 'unsorted' what no folder holds; any other value is a folder's id.
const selected = defineModel<string>('selected', { required: true })
// Moves and deletions the list asks for; ContentLibrary saves them. A null parentId is the top.
const emit = defineEmits<{ update: [id: string, name: string, parentId: string | null], remove: [id: string] }>()
const root = ref<HTMLElement>()

// The folders opened by hand. The open folder and those above it open too, so the selection stays in sight.
const expanded = ref(new Set<string>())
const expand = (ids: string[]) => {
  if (ids.some(id => !expanded.value.has(id))) expanded.value = new Set([...expanded.value, ...ids])
}
watch([selected, () => props.folders], () => { expand(folderPath(props.folders, selected.value).map(folder => folder.id)) }, { immediate: true })
// A folder made in or moved into a closed folder opens the folders above it, so it can be seen where it went.
watch(() => props.folders, (now, before) => {
  const parents = new Map(before.map(folder => [folder.id, folder.parentId]))
  expand(now.filter(folder => folder.parentId !== null && parents.get(folder.id) !== folder.parentId).flatMap(folder => folderPath(now, folder.parentId!).map(above => above.id)))
})
function toggle(id: string) {
  const next = new Set(expanded.value)
  if (!next.delete(id)) next.add(id)
  expanded.value = next
}

// One flat list in display order, indented by depth; a closed folder leaves out what is below it.
const rows = computed(() => {
  const list: { folder: Folder, depth: number, parent: boolean, open: boolean }[] = []
  const add = (parentId: string | null, depth: number) => {
    for (const folder of sortedChildren(props.folders, parentId)) {
      const parent = props.folders.some(child => child.parentId === folder.id)
      const open = parent && expanded.value.has(folder.id)
      list.push({ folder, depth, parent, open })
      if (open) add(folder.id, depth + 1)
    }
  }
  add(null, 0)
  return list
})

// A folder's actions open under its row. Escape, a click elsewhere or tabbing away closes them.
const menuId = useId()
const menu = ref('')
const menuButtons = new Map<string, HTMLElement>()
function closeMenu(refocus = false) {
  const id = menu.value
  menu.value = ''
  if (refocus) menuButtons.get(id)?.focus()
}
function leaveMenu(event: FocusEvent) {
  const actions = event.currentTarget as HTMLElement
  if (!(event.relatedTarget instanceof Node && actions.contains(event.relatedTarget))) closeMenu()
}
function outside(event: PointerEvent) {
  if (menu.value && event.target instanceof Element && !event.target.closest('.folder-tree-actions')) closeMenu()
}
onMounted(() => document.addEventListener('pointerdown', outside))
onBeforeUnmount(() => document.removeEventListener('pointerdown', outside))
// After a change the row may have moved or gone; its actions button, or else the open folder, keeps the focus.
async function keepFocus(id: string) {
  await nextTick()
  const button = menuButtons.get(id)
  if (button?.isConnected) button.focus()
  else root.value?.querySelector<HTMLElement>('.folder-tree-item[aria-current="true"]')?.focus()
}

// New folders go into the open folder; すべて and 未整理 make them at the top.
const parentId = computed(() => selected.value && selected.value !== 'unsorted' ? selected.value : null)
const canCreate = computed(() => canNest(props.folders, null, parentId.value))
const depthReason = 'フォルダの入れ子は3段までです。このフォルダの中には作れません。'
const reasonId = useId()
const nameDialog = ref<{ open: (name: string, save: (name: string) => Promise<string>) => Promise<string | undefined> }>()
const naming = ref({ title: '', action: '', description: undefined as string | undefined })
async function create() {
  const parent = parentId.value
  const above = parent && props.folders.find(folder => folder.id === parent)
  naming.value = { title: 'フォルダを作る', action: '作る', description: above ? `「${above.name}」の中に作ります。` : '最上位に作ります。' }
  await nameDialog.value?.open('', name => props.saveName(null, name, parent))
}
async function rename(folder: Folder) {
  closeMenu(true)
  naming.value = { title: 'フォルダの名前を変更', action: '変更する', description: undefined }
  const saved = await nameDialog.value?.open(folder.name, name => {
    const current = props.folders.find(item => item.id === folder.id)
    // The same name, or a folder gone meanwhile, has nothing to save.
    return !current || name === current.name ? Promise.resolve('') : props.saveName(current.id, name, current.parentId)
  })
  // Folders sort by name, so the row may have moved and taken the focus off its button.
  if (saved !== undefined) await keepFocus(folder.id)
}

// A folder can go only where it and the folders below it fit, and never into itself or below itself.
const picker = ref<{ open: (current?: string | null) => Promise<string | null | undefined> }>()
const moving = ref<Folder>()
const blocked = computed(() => moving.value ? props.folders.filter(folder => !canNest(props.folders, moving.value!.id, folder.id)).map(folder => folder.id) : [])
async function move(folder: Folder) {
  closeMenu(true)
  moving.value = folder
  const parent = await picker.value?.open(folder.parentId)
  const current = props.folders.find(item => item.id === folder.id)
  if (parent === undefined || !current || parent === current.parentId) return
  emit('update', current.id, current.name, parent)
  await keepFocus(current.id)
}

// The confirmation says how many folders go with it and how many problems and posts it gives back to 未整理.
const removeId = useId()
const removeDialog = ref<HTMLDialogElement>()
const removing = ref<Folder>()
const removeText = computed(() => {
  if (!removing.value) return ''
  const below = descendantIds(props.folders, removing.value.id).length
  const held = props.counts.byFolder.get(removing.value.id) ?? 0
  return `${below ? `中のフォルダ${below}個も削除されます。` : ''}問題と記事${held}件は「未整理」に戻り、削除されません。`
})
async function confirmRemove(folder: Folder) {
  closeMenu(true)
  removing.value = folder
  await nextTick()
  removeDialog.value?.showModal()
}
async function remove() {
  const folder = removing.value
  if (!folder) return
  removeDialog.value?.close()
  // The open folder goes with it, so the list shows everything instead.
  if ([folder.id, ...descendantIds(props.folders, folder.id)].includes(selected.value)) selected.value = ''
  emit('remove', folder.id)
  await keepFocus(folder.id)
}
// Cancelling goes back to the folder's actions button; after a deletion remove() has placed the focus already.
function removeClosed() {
  const button = removing.value && menuButtons.get(removing.value.id)
  removing.value = undefined
  if (button?.isConnected && document.activeElement === document.body) button.focus()
}
</script>

<template>
  <div ref="root" class="folder-tree">
    <nav aria-label="フォルダ">
      <ul class="folder-tree-list">
        <li class="folder-tree-row">
          <span class="folder-tree-toggle" />
          <button type="button" class="folder-tree-item" :aria-current="selected === '' ? 'true' : undefined" @click="selected = ''"><span class="folder-tree-name">すべて</span><span class="folder-tree-count">{{ counts.all }}</span></button>
          <span class="folder-tree-menu-button" />
        </li>
        <li v-for="row in rows" :key="row.folder.id" class="folder-tree-row" :style="{ '--folder-depth': row.depth }">
          <button v-if="row.parent" type="button" class="folder-tree-toggle" :aria-expanded="row.open" :aria-label="`${row.folder.name}の中のフォルダ`" @click="toggle(row.folder.id)">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m9 6 6 6-6 6" /></svg>
          </button>
          <span v-else class="folder-tree-toggle" />
          <button type="button" class="folder-tree-item" :aria-current="selected === row.folder.id ? 'true' : undefined" @click="selected = row.folder.id"><span class="folder-tree-name">{{ row.folder.name }}</span><span class="folder-tree-count">{{ counts.byFolder.get(row.folder.id) ?? 0 }}</span></button>
          <span class="folder-tree-actions" @focusout="leaveMenu" @keydown.esc="closeMenu(true)">
            <button :ref="el => { if (el) menuButtons.set(row.folder.id, el as HTMLElement); else menuButtons.delete(row.folder.id) }" type="button" class="folder-tree-menu-button" :aria-label="`${row.folder.name}の操作`" :aria-expanded="menu === row.folder.id" :aria-controls="menu === row.folder.id ? menuId : undefined" @click="menu = menu === row.folder.id ? '' : row.folder.id">
              <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><circle cx="5" cy="12" r="1.8" /><circle cx="12" cy="12" r="1.8" /><circle cx="19" cy="12" r="1.8" /></svg>
            </button>
            <span v-if="menu === row.folder.id" :id="menuId" class="folder-tree-menu" role="group" :aria-label="`${row.folder.name}の操作`">
              <button type="button" @click="rename(row.folder)">名前を変更</button>
              <button type="button" @click="move(row.folder)">移動</button>
              <button type="button" class="folder-tree-delete" @click="confirmRemove(row.folder)">削除</button>
            </span>
          </span>
        </li>
        <li class="folder-tree-row">
          <span class="folder-tree-toggle" />
          <button type="button" class="folder-tree-item" :aria-current="selected === 'unsorted' ? 'true' : undefined" @click="selected = 'unsorted'"><span class="folder-tree-name">未整理</span><span class="folder-tree-count">{{ counts.unsorted }}</span></button>
          <span class="folder-tree-menu-button" />
        </li>
      </ul>
    </nav>
    <p v-if="!folders.length" class="folder-tree-hint">「フォルダを作る」でフォルダを作り、行の「フォルダへ移動」で問題や記事を入れられます。</p>
    <button type="button" class="editor-button folder-tree-create" :disabled="!canCreate" :title="canCreate ? undefined : depthReason" :aria-describedby="canCreate ? undefined : reasonId" @click="create()">
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true"><path d="M12 5v14M5 12h14" /></svg>フォルダを作る
    </button>
    <p v-if="!canCreate" :id="reasonId" class="folder-tree-hint">{{ depthReason }}</p>
    <FolderNameDialog ref="nameDialog" :title="naming.title" :action="naming.action" :description="naming.description" />
    <FolderPicker ref="picker" :folders="folders" :title="moving ? `「${moving.name}」を移動` : ''" :disabled-ids="blocked" none-label="最上位" />
    <Teleport v-if="removing" to="body">
      <dialog v-dialog-dismiss ref="removeDialog" class="leave-dialog" :aria-labelledby="`${removeId}-title`" :aria-describedby="`${removeId}-description`" @close="removeClosed()">
        <h2 :id="`${removeId}-title`">「{{ removing.name }}」を削除しますか？</h2>
        <p :id="`${removeId}-description`">{{ removeText }}</p>
        <div class="leave-dialog-actions">
          <button type="button" class="editor-button" autofocus @click="removeDialog?.close()">キャンセル</button>
          <button type="button" class="editor-button danger" @click="remove()">削除する</button>
        </div>
      </dialog>
    </Teleport>
  </div>
</template>
