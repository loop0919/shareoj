<script setup lang="ts">
// One line about what the button will do, then the actions. Status and errors take that line's place.
defineProps<{ status?: string, error?: string, message?: string, placeholder?: string }>()
</script>

<template>
  <div class="action-bar">
    <div class="bar-main">
      <p v-if="status" class="bar-status" role="status"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="m5 12 5 5 9-10" /></svg>{{ status }}</p>
      <p v-else-if="error" class="editor-error" role="alert">{{ error }}</p>
      <p v-else-if="message" class="bar-message">{{ message }}</p>
      <p v-else class="bar-message bar-placeholder">{{ placeholder }}</p>
      <slot />
    </div>
    <div class="bar-actions"><slot name="actions" /></div>
  </div>
</template>

<style scoped>
/* Hallmark · component: pinned action bar · genre: modern-minimal · theme: existing ShareOJ tokens (Plain)
 * Sticks to the bottom of the screen while its page runs past it, so a long list never hides the button.
 * From 60rem it follows the pages' two columns (26rem, then the rest): the choice under the list, the button at the bottom right.
 * states: placeholder · message · status (success) · error · actions disabled · hover/focus/active via editor-button */
.action-bar { position: sticky; bottom: 0; z-index: 1; display: grid; grid-template-columns: minmax(0, 1fr); gap: 10px; padding-block: 14px 16px; border-top: 1px solid var(--color-line); background: var(--color-paper); }
.bar-main { display: grid; gap: 10px; min-width: 0; }
.bar-main > p { margin: 0; }
.bar-message { min-height: 1lh; font-size: .9375rem; font-weight: 500; overflow-wrap: anywhere; }
.bar-placeholder { color: var(--color-muted); font-size: .8125rem; font-weight: 400; }
.bar-status { display: flex; align-items: flex-start; gap: 8px; color: var(--color-accent); font-size: .9375rem; font-weight: 500; overflow-wrap: anywhere; }
.bar-status svg { flex: none; width: 20px; height: 20px; margin-top: 4px; fill: none; stroke: currentColor; stroke-width: 2.2; stroke-linecap: round; stroke-linejoin: round; }
.bar-actions { display: flex; flex-wrap: wrap; align-items: center; justify-content: flex-end; gap: 12px; }
.bar-actions :slotted(a) { display: inline-flex; align-items: center; min-height: 44px; margin-right: auto; font-size: .875rem; white-space: nowrap; }
.bar-actions :slotted(.editor-button) { min-height: 44px; padding: 8px 22px; font-size: .9375rem; }
.bar-actions :slotted(.editor-button.primary) { padding-inline: 28px; font-weight: 600; }
.bar-actions :slotted(.editor-button:disabled) { opacity: .55; cursor: not-allowed; text-decoration: none; }
@media (min-width: 60rem) {
  .action-bar { grid-template-columns: minmax(0, 26rem) minmax(0, 1fr); column-gap: 56px; align-items: end; }
  /* The link joins the button at the right instead of floating mid-page. */
  .bar-actions { gap: 24px; }
  .bar-actions :slotted(a) { margin-right: 0; }
}
</style>
