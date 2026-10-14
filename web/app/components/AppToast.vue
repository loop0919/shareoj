<script setup lang="ts">
const { toast, dismissToast } = useToast()
let timer: ReturnType<typeof setTimeout> | undefined
function pause() { clearTimeout(timer) }
function resume() {
  pause()
  if (toast.value) timer = setTimeout(dismissToast, 5000)
}
watch(toast, resume)
onBeforeUnmount(pause)
</script>

<template>
  <div class="toast-region" aria-live="polite" aria-atomic="true">
    <div v-if="toast" class="toast" @mouseenter="pause" @mouseleave="resume" @focusin="pause" @focusout="resume">
      <svg class="toast-icon" viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="9" /><path d="m8 12 3 3 5-6" /></svg>
      <span role="status">{{ toast.message }}</span>
      <button type="button" aria-label="通知を閉じる" @click="dismissToast"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="m6 6 12 12M18 6 6 18" /></svg></button>
    </div>
  </div>
</template>

<style scoped>
.toast-region { position: fixed; right: max(16px, env(safe-area-inset-right)); bottom: max(40px, env(safe-area-inset-bottom)); z-index: 200; max-width: min(28rem, calc(100vw - 32px)); pointer-events: none; }
.toast { display: flex; align-items: center; gap: 12px; padding: 8px 8px 8px 16px; border: 1px solid var(--color-line); border-radius: 8px; background: var(--color-paper); color: var(--color-ink); box-shadow: 0 4px 20px #0002; font-size: .875rem; overflow-wrap: anywhere; pointer-events: auto; }
svg { width: 20px; height: 20px; fill: none; stroke: currentColor; stroke-width: 1.8; }
.toast-icon { flex-shrink: 0; color: var(--color-accent); }
button { display: grid; place-items: center; flex-shrink: 0; width: 44px; height: 44px; padding: 0; border: 0; border-radius: 4px; background: transparent; color: var(--color-muted); cursor: pointer; }
button:hover { background: var(--color-surface); color: var(--color-ink); }
button:focus-visible { outline: 2px solid var(--color-accent); outline-offset: -2px; }
</style>
