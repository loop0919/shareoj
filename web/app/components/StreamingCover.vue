<script setup lang="ts">
import { coveredMark } from '~/composables/useStreamingMode'
const { stale } = useStreamingMode()
const dialog = ref<HTMLDialogElement>()
// A modal dialog makes everything else inert and unreachable, whichever layout is showing. Removing it lifts the cover.
watch(dialog, element => { if (element && !element.open) element.showModal() }, { flush: 'post' })
// Whatever closes the dialog (a second Escape cannot be cancelled in Chromium), it comes back at once while stale.
// Escape is stopped before it can close the dialog at all, so the content never shows; Tab and backdrop clicks cannot move focus out of the only control.
function keepOpen() {
  if (!stale.value || !dialog.value || dialog.value.open) return
  dialog.value.showModal()
  dialog.value.querySelector('button')?.focus()
}
// While stale the app root is hidden, not the dialog (a sibling in body), so a dialog that closes for any reason exposes nothing.
// The mark follows stale alone: the app and error views each mount their own cover, and an unmount must not lift it.
if (import.meta.client) watchEffect(() => { document.documentElement.toggleAttribute(coveredMark, stale.value) })
function reload() { location.reload() }
</script>

<template>
  <Teleport to="body">
    <dialog v-if="stale" ref="dialog" class="streaming-cover" aria-labelledby="streaming-cover-title" @cancel.prevent @close="keepOpen" @keydown.esc.prevent @keydown.tab.prevent @mousedown.prevent>
      <p id="streaming-cover-title">配信モードの設定が変わりました。</p>
      <button type="button" class="editor-button" autofocus @click="reload">読み込み直す</button>
    </dialog>
  </Teleport>
</template>

<style scoped>
.streaming-cover { position: fixed; inset: 0; width: 100vw; max-width: none; height: 100dvh; max-height: none; margin: 0; border: 0; padding: 24px; background: var(--color-paper); color: var(--color-ink); }
.streaming-cover[open] { display: grid; place-content: center; justify-items: center; gap: 16px; }
.streaming-cover::backdrop { background: var(--color-paper); }
</style>
