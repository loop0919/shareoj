<script setup lang="ts">
import type { Contest } from '~~/shared/types/contest'
// The viewer's registration: the division when the contest has one, and in team contests the team or 個人参加
// (ADR 0024). The team page names the team in its panel instead.
defineProps<{ contest: Contest, hideTeam?: boolean }>()
</script>

<template>
  <p class="participation-state"><span class="participating" role="status">参加済み</span><span v-if="contest.onsiteDivision" class="participation-division" :data-onsite="contest.onsite || undefined">{{ contest.onsite ? 'オンサイト参加' : 'オープン参加' }}</span><template v-if="contest.teamMode && !hideTeam"><span v-if="contest.team" class="participation-team">チーム <strong>{{ contest.team.name }}</strong></span><span v-else class="participation-team">個人参加</span></template></p>
</template>
