<script setup lang="ts">
const { stale } = useStreamingMode()
const dialog = ref<HTMLDialogElement>()
// A modal dialog makes everything else inert and unreachable, whichever layout is showing. Removing it lifts the cover.
watch(dialog, element => { if (element && !element.open) element.showModal() }, { flush: 'post' })
function reload() { location.reload() }
</script>

<template>
  <Teleport to="body">
    <dialog v-if="stale" ref="dialog" class="streaming-cover" aria-labelledby="streaming-cover-title" @cancel.prevent>
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
