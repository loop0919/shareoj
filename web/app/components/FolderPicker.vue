<script setup lang="ts">
import type { Folder } from '~~/shared/types/folders'
import { vDialogDismiss } from '~/utils/dialog-dismiss'
import { sortedChildren } from '~/utils/folder-tree'
// Picks where something goes. The first choice is no folder, which reads 未整理 for problems and posts and 最上位 for a
// folder; disabledIds are the folders that cannot take it. open() gives the folder's id, null for the first choice, or
// undefined when cancelled; current, when given, is chosen to start with.
const props = withDefaults(defineProps<{ folders: Folder[], title: string, disabledIds?: string[], noneLabel?: string }>(), { disabledIds: () => [], noneLabel: '未整理' })
const dialogId = useId()
const isOpen = ref(false)
const dialog = ref<HTMLDialogElement>()
const group = ref<HTMLElement>()
// The radios' value: '' is the first choice, any other value a folder's id.
const choice = ref<string>()
let answer: string | null | undefined
let done: ((folderId: string | null | undefined) => void) | undefined
let trigger: HTMLElement | undefined

// Every folder in the list's order, indented by depth. A nested folder is named by its path, since names repeat
// under different parents.
const options = computed(() => {
  const list: { value: string, label: string, name?: string, depth: number, disabled: boolean }[] = [{ value: '', label: props.noneLabel, depth: 0, disabled: false }]
  const disabled = new Set(props.disabledIds)
  const add = (parentId: string | null, path: string[]) => {
    for (const folder of sortedChildren(props.folders, parentId)) {
      const names = [...path, folder.name]
      list.push({ value: folder.id, label: folder.name, name: path.length ? names.join(' / ') : undefined, depth: path.length, disabled: disabled.has(folder.id) })
      add(folder.id, names)
    }
  }
  add(null, [])
  return list
})

async function open(current?: string | null): Promise<string | null | undefined> {
  done?.(undefined)
  const start = current === undefined ? undefined : current ?? ''
  choice.value = options.value.some(option => option.value === start && !option.disabled) ? start : undefined
  answer = undefined
  trigger = document.activeElement instanceof HTMLElement ? document.activeElement : undefined
  isOpen.value = true
  await nextTick()
  dialog.value?.showModal()
  // Like tabbing into a radio group: the chosen radio, or else the first that can be chosen.
  const radios = [...group.value?.querySelectorAll<HTMLInputElement>('input[type="radio"]') ?? []]
  ;(radios.find(radio => radio.checked) ?? radios.find(radio => !radio.disabled))?.focus()
  return new Promise(resolve => { done = resolve })
}
function confirm() {
  if (choice.value === undefined) return
  answer = choice.value || null
  dialog.value?.close()
}
// Enter takes the radio that has the focus, which is the chosen one unless nothing was chosen yet.
function take(event: KeyboardEvent) {
  const radio = event.target
  if (radio instanceof HTMLInputElement && radio.type === 'radio' && !radio.disabled) choice.value = radio.value
  confirm()
}
function closed() {
  isOpen.value = false
  if (trigger?.isConnected) trigger.focus()
  done?.(answer)
  done = undefined
}
defineExpose({ open })
</script>

<template>
  <Teleport v-if="isOpen" to="body">
    <dialog v-dialog-dismiss ref="dialog" class="leave-dialog" :aria-labelledby="`${dialogId}-title`" @close="closed">
      <h2 :id="`${dialogId}-title`">{{ title }}</h2>
      <!-- Arrow keys move between the radios, as in any radio group. -->
      <div ref="group" class="folder-picker" role="radiogroup" aria-label="移動先" @keydown.enter.prevent="take">
        <label v-for="option in options" :key="option.value" class="folder-picker-option" :style="{ '--folder-depth': option.depth }">
          <input v-model="choice" type="radio" :name="`${dialogId}-folder`" :value="option.value" :disabled="option.disabled" :aria-label="option.name">
          <span>{{ option.label }}</span>
        </label>
      </div>
      <div class="leave-dialog-actions">
        <button type="button" class="editor-button" @click="dialog?.close()">キャンセル</button>
        <button type="button" class="editor-button primary" :disabled="choice === undefined" @click="confirm()">移動する</button>
      </div>
    </dialog>
  </Teleport>
</template>
