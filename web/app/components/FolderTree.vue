<script setup lang="ts">
import type { Folder } from '~~/shared/types/folders'
import { folderPath, sortedChildren } from '~/utils/folder-tree'
// counts comes from folderCounts and is not narrowed by the filters, so a folder's number is what it holds.
const props = defineProps<{ folders: Folder[], counts: { all: number, unsorted: number, byFolder: Map<string, number> } }>()
// '' is everything and 'unsorted' what no folder holds; any other value is a folder's id.
const selected = defineModel<string>('selected', { required: true })

// The folders opened by hand. The open folder and those above it open too, so the selection stays in sight.
const expanded = ref(new Set<string>())
watch([selected, () => props.folders], () => {
  const path = folderPath(props.folders, selected.value)
  if (path.some(folder => !expanded.value.has(folder.id))) expanded.value = new Set([...expanded.value, ...path.map(folder => folder.id)])
}, { immediate: true })
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
</script>

<template>
  <div class="folder-tree">
    <nav aria-label="フォルダ">
      <ul class="folder-tree-list">
        <li class="folder-tree-row">
          <span class="folder-tree-toggle" />
          <button type="button" class="folder-tree-item" :aria-current="selected === '' ? 'true' : undefined" @click="selected = ''"><span class="folder-tree-name">すべて</span><span class="folder-tree-count">{{ counts.all }}</span></button>
        </li>
        <li v-for="row in rows" :key="row.folder.id" class="folder-tree-row" :style="{ '--folder-depth': row.depth }">
          <button v-if="row.parent" type="button" class="folder-tree-toggle" :aria-expanded="row.open" :aria-label="`${row.folder.name}の中のフォルダ`" @click="toggle(row.folder.id)">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m9 6 6 6-6 6" /></svg>
          </button>
          <span v-else class="folder-tree-toggle" />
          <button type="button" class="folder-tree-item" :aria-current="selected === row.folder.id ? 'true' : undefined" @click="selected = row.folder.id"><span class="folder-tree-name">{{ row.folder.name }}</span><span class="folder-tree-count">{{ counts.byFolder.get(row.folder.id) ?? 0 }}</span></button>
        </li>
        <li class="folder-tree-row">
          <span class="folder-tree-toggle" />
          <button type="button" class="folder-tree-item" :aria-current="selected === 'unsorted' ? 'true' : undefined" @click="selected = 'unsorted'"><span class="folder-tree-name">未整理</span><span class="folder-tree-count">{{ counts.unsorted }}</span></button>
        </li>
      </ul>
    </nav>
    <p v-if="!folders.length" class="folder-tree-hint">フォルダを作ると、問題と記事を出す先ごとに分けられます。</p>
  </div>
</template>
