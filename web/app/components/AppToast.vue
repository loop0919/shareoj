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
    <div v-if="toast" class="toast" :data-type="toast.type" @mouseenter="pause" @mouseleave="resume" @focusin="pause" @focusout="resume">
      <svg class="toast-icon" viewBox="0 0 24 24" aria-hidden="true">
        <template v-if="toast.type === 'warn'"><path d="m12 3 10 18H2Z" /><path d="M12 9v5m0 2v2" /></template>
        <template v-else><circle cx="12" cy="12" r="9" /><path :d="toast.type === 'error' ? 'm8 8 8 8m0-8-8 8' : 'M12 11v6m0-10v2'" /></template>
      </svg>
      <span role="status">{{ toast.message }}</span>
      <button type="button" aria-label="通知を閉じる" @click="dismissToast"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="m6 6 12 12M18 6 6 18" /></svg></button>
    </div>
  </div>
</template>

<style scoped>
.toast-region { position: fixed; top: max(16px, env(safe-area-inset-top)); left: 50%; transform: translateX(-50%); z-index: 200; width: max-content; max-width: min(28rem, calc(100vw - 32px)); pointer-events: none; }
.toast { --toast-color: light-dark(#176b43, #86e0ae); --toast-background: light-dark(#edf9f1, #183b2b); --toast-border: light-dark(#90cba7, #347d53); display: flex; align-items: center; gap: 12px; padding: 8px 8px 8px 16px; border: 1px solid var(--toast-border); border-radius: 8px; background: var(--toast-background); color: var(--toast-color); box-shadow: 0 4px 20px #0002; font-size: .875rem; overflow-wrap: anywhere; pointer-events: auto; }
.toast[data-type="warn"] { --toast-color: light-dark(#785600, #f5d76e); --toast-background: light-dark(#fff9db, #3b3214); --toast-border: light-dark(#e8ce63, #8f772e); }
.toast[data-type="error"] { --toast-color: light-dark(#a62620, #ffaaa4); --toast-background: light-dark(#fff0ee, #431f20); --toast-border: light-dark(#eba7a1, #a34a45); }
svg { width: 20px; height: 20px; fill: none; stroke: currentColor; stroke-width: 1.8; }
.toast-icon { flex-shrink: 0; }
button { display: grid; place-items: center; flex-shrink: 0; width: 44px; height: 44px; padding: 0; border: 0; border-radius: 4px; background: transparent; color: inherit; cursor: pointer; }
button:hover { background: color-mix(in srgb, currentColor 10%, transparent); }
button:focus-visible { outline: 2px solid currentColor; outline-offset: -2px; }
</style>
