<script setup lang="ts">
import { maxFolderName } from '~~/shared/types/folders'
import { vDialogDismiss } from '~/utils/dialog-dismiss'
// Asks for a folder's name. title heads the dialog, description says where the folder goes and action names the
// button that saves. open() gives the trimmed name, or undefined when cancelled; the caller saves it.
defineProps<{ title: string, description?: string, action: string }>()
const dialogId = useId()
// The dialog exists only while it is open, like the other confirmations on the my page.
const isOpen = ref(false)
const dialog = ref<HTMLDialogElement>()
const input = ref<HTMLInputElement>()
const name = ref('')
let answer: string | undefined
let done: ((name: string | undefined) => void) | undefined
let trigger: HTMLElement | undefined

async function open(initial = ''): Promise<string | undefined> {
  done?.(undefined)
  name.value = initial
  answer = undefined
  trigger = document.activeElement instanceof HTMLElement ? document.activeElement : undefined
  isOpen.value = true
  await nextTick()
  dialog.value?.showModal()
  input.value?.select()
  return new Promise(resolve => { done = resolve })
}
function save() {
  if (!name.value.trim()) return
  answer = name.value.trim()
  dialog.value?.close()
}
// Focus goes back to what opened the dialog, then the caller hears the answer.
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
    <dialog v-dialog-dismiss ref="dialog" class="leave-dialog" :aria-labelledby="`${dialogId}-title`" :aria-describedby="description ? `${dialogId}-description` : undefined" @close="closed">
      <h2 :id="`${dialogId}-title`">{{ title }}</h2>
      <p v-if="description" :id="`${dialogId}-description`">{{ description }}</p>
      <form class="folder-name-form" @submit.prevent="save">
        <label :for="`${dialogId}-name`">フォルダの名前</label>
        <input :id="`${dialogId}-name`" ref="input" v-model="name" type="text" :maxlength="maxFolderName" required autocomplete="off">
        <div class="leave-dialog-actions">
          <button type="button" class="editor-button" @click="dialog?.close()">キャンセル</button>
          <button type="submit" class="editor-button primary" :disabled="!name.trim()">{{ action }}</button>
        </div>
      </form>
    </dialog>
  </Teleport>
</template>
