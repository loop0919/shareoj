<script setup lang="ts">
import { vDialogDismiss } from '~/utils/dialog-dismiss'
const { offOpen, turnOff } = useStreamingMode()
const dialog = ref<HTMLDialogElement>()
// The dialog is a native modal: opening and closing follow the shared flag.
watch(offOpen, open => {
  if (open && !dialog.value?.open) dialog.value?.showModal()
  else if (!open && dialog.value?.open) dialog.value.close()
})
</script>

<template>
  <Teleport to="body">
    <dialog v-dialog-dismiss ref="dialog" class="leave-dialog" aria-labelledby="streaming-off-title" aria-describedby="streaming-off-description" @close="offOpen = false">
      <h2 id="streaming-off-title">配信モードを切りますか？</h2>
      <p id="streaming-off-description">マイページやスタッフ向けの画面が、このブラウザで見えるようになります。</p>
      <div class="leave-dialog-actions">
        <button type="button" class="editor-button" autofocus @click="dialog?.close()">キャンセル</button>
        <button type="button" class="editor-button danger" @click="turnOff">切る</button>
      </div>
    </dialog>
  </Teleport>
</template>
