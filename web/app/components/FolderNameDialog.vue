<script setup lang="ts">
import { maxFolderName } from '~~/shared/types/folders'
import { vDialogDismiss } from '~/utils/dialog-dismiss'
// Asks for a folder's name and saves it. title heads the dialog, description says where the folder goes and action
// names the button that saves. open() calls save with the trimmed name, which resolves to '' once saved or else to why
// it was refused; a refusal shows here and keeps the name for another try. open() gives the saved name, or undefined
// when cancelled.
defineProps<{ title: string, description?: string, action: string }>()
const dialogId = useId()
// The dialog exists only while it is open, like the other confirmations on the my page.
const isOpen = ref(false)
const dialog = ref<HTMLDialogElement>()
const input = ref<HTMLInputElement>()
const name = ref('')
const saving = ref(false)
const refusal = ref('')
let save: (name: string) => Promise<string> = async () => ''
let answer: string | undefined
let done: ((name: string | undefined) => void) | undefined
let trigger: HTMLElement | undefined

async function open(initial: string, saveName: (name: string) => Promise<string>): Promise<string | undefined> {
  done?.(undefined)
  name.value = initial
  save = saveName
  refusal.value = ''
  answer = undefined
  trigger = document.activeElement instanceof HTMLElement ? document.activeElement : undefined
  isOpen.value = true
  await nextTick()
  dialog.value?.showModal()
  input.value?.select()
  return new Promise(resolve => { done = resolve })
}
async function submit() {
  const trimmed = name.value.trim()
  if (!trimmed || saving.value) return
  saving.value = true
  refusal.value = ''
  try { refusal.value = await save(trimmed) }
  finally { saving.value = false }
  if (refusal.value) {
    await nextTick()
    input.value?.focus()
    return
  }
  answer = trimmed
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
    <dialog v-dialog-dismiss ref="dialog" class="leave-dialog" :aria-labelledby="`${dialogId}-title`" :aria-describedby="description ? `${dialogId}-description` : undefined" @cancel="saving && $event.preventDefault()" @close="closed">
      <h2 :id="`${dialogId}-title`">{{ title }}</h2>
      <p v-if="description" :id="`${dialogId}-description`">{{ description }}</p>
      <form class="folder-name-form" @submit.prevent="submit()">
        <label :for="`${dialogId}-name`">フォルダの名前</label>
        <input :id="`${dialogId}-name`" ref="input" v-model="name" type="text" :maxlength="maxFolderName" required autocomplete="off" :readonly="saving">
        <p v-if="refusal" role="alert" class="editor-error">{{ refusal }}</p>
        <div class="leave-dialog-actions">
          <button type="button" class="editor-button" :disabled="saving" @click="dialog?.close()">キャンセル</button>
          <button type="submit" class="editor-button primary" :disabled="saving || !name.trim()" :aria-busy="saving">{{ saving ? '保存中…' : action }}</button>
        </div>
      </form>
    </dialog>
  </Teleport>
</template>
