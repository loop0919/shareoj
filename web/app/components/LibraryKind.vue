<script setup lang="ts">
import { libraryKinds, type LibraryKind } from '~/utils/library-entries'
// A row's kind as a tinted mark. Three kinds share one table, and the mark's shape and colour tell them apart without
// reading. Its name shows beside the mark while the pointer is on the cell. A screen that cannot hover, or a card on
// a phone, shows the name all the time; a screen reader reads it either way.
const props = defineProps<{ kind: LibraryKind | 'folder' }>()
const label = computed(() => props.kind === 'folder' ? 'フォルダ' : libraryKinds.find(kind => kind.key === props.kind)!.label)
</script>
<template>
  <span class="library-kind-name" :class="`library-kind-name--${kind}`">
    <svg width="30" height="30" viewBox="-5 -5 34 34" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
      <template v-if="kind === 'own'"><path d="m8 8-4 4 4 4M16 8l4 4-4 4M13.5 6l-3 12" /></template>
      <template v-else-if="kind === 'testing'"><path d="M9 3h6M10 3v6l-5 9a2 2 0 0 0 1.8 3h10.4A2 2 0 0 0 19 18l-5-9V3" /><path d="M7.5 15h9" /></template>
      <template v-else-if="kind === 'post'"><path d="M7 3h7l5 5v11a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2Z" /><path d="M14 3v5h5M9 13h6M9 17h6" /></template>
      <path v-else d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2Z" />
    </svg>
    <span class="library-kind-label">{{ label }}</span>
  </span>
</template>
<style scoped>
.library-kind-name { position: relative; display: inline-flex; align-items: center; gap: 8px; white-space: nowrap; vertical-align: middle; }
.library-kind-name svg { flex: none; border-radius: 6px; background: var(--kind-soft); color: var(--kind-ink); }
.library-kind-name--own { --kind-ink: var(--color-accent); --kind-soft: var(--color-accent-soft); }
.library-kind-name--testing { --kind-ink: light-dark(#8f5400, #f0b860); --kind-soft: light-dark(#fdf1dc, #40321a); }
.library-kind-name--post { --kind-ink: light-dark(#1f5fbf, #8ab8ff); --kind-soft: light-dark(#e8f1fd, #1d3050); }
.library-kind-name--folder { --kind-ink: var(--color-muted); --kind-soft: var(--color-surface); }
/* The name stays in the page and only fades in, so it is read aloud without the pointer. */
@media (hover: hover) and (min-width: 40rem) {
  .library-kind-label { position: absolute; left: calc(100% + 8px); top: 50%; translate: 0 -50%; z-index: 2; padding: 4px 8px; border-radius: 4px; background: var(--color-ink); color: var(--color-paper); font-size: .75rem; font-weight: 600; opacity: 0; pointer-events: none; }
  .library-kind:hover .library-kind-label { opacity: 1; }
}
</style>
