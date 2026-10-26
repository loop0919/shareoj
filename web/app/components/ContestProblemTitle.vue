<script setup lang="ts">
import { problemLabel } from '~~/shared/types/contest'
// 「A - タイトル」, with the letter set back so the title reads first. The text is the same as contestProblemTitle.
defineProps<{ index: number, title?: string }>()
</script>

<template><span class="contest-problem-letter">{{ `${problemLabel(index)} - ` }}</span>{{ title }}</template>

<style scoped>
.contest-problem-letter { color: var(--color-muted); }
</style>
