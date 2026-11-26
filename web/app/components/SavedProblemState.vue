<script setup lang="ts">
import type { AccountSummary } from '~~/shared/types/account-problems'
import { japanMinute } from '~/utils/japan-time'
// The reserved time reads as the list's update time does, to the minute. Both lists are in JST throughout, so the
// zone is not repeated.
defineProps<{ entry: AccountSummary }>()
const tooltipId = useId()
const preferenceLabel = (preference: string) => preference === 'soon' ? '早めに出したい' : 'ゆっくりで良い'
</script>

<template>
  <span class="problem-state">
    <ProblemStatus :problem="entry" />
    <time v-if="entry.scheduledAt" :datetime="entry.scheduledAt">{{ japanMinute(entry.scheduledAt) }}</time>
    <span v-if="!entry.publishedVersion && !entry.contestScheduled && entry.featuredPreference" class="featured-preference" tabindex="0" role="img" :aria-label="preferenceLabel(entry.featuredPreference)" :aria-describedby="`${tooltipId}-${entry.id}`">
      <FeaturedPreferenceIcon :preference="entry.featuredPreference === 'soon' ? 'soon' : 'later'" />
      <span :id="`${tooltipId}-${entry.id}`" class="preference-tooltip" role="tooltip">{{ preferenceLabel(entry.featuredPreference) }}</span>
    </span>
  </span>
</template>

<style scoped>
.problem-state { display: inline-flex; flex-wrap: wrap; align-items: flex-start; gap: 6px; }
.featured-preference { position: relative; display: inline-flex; align-items: center; justify-content: center; width: 32px; height: 32px; color: var(--color-muted); cursor: help; }
.featured-preference:focus-visible { outline: 2px solid var(--color-accent); outline-offset: 2px; border-radius: 4px; }
.preference-tooltip { display: none; position: absolute; bottom: 100%; left: 50%; transform: translateX(-50%); padding: 4px 8px; border-radius: 4px; background: var(--color-ink); color: var(--color-paper); font-size: .75rem; font-weight: 400; z-index: 1; }
.featured-preference:hover .preference-tooltip, .featured-preference:focus .preference-tooltip { display: block; }
</style>
