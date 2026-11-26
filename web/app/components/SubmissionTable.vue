<script setup lang="ts">
import { contestDate } from '~~/shared/types/contest'
import type { Submission } from '~~/shared/types/submission'
import { submissionUsage } from '~/utils/submission-usage'
import { runtimeLabel } from '~/utils/runtime-label'
import { formatScore } from '~/utils/score'
// The table of a problem's list and of a contest's lists. select adds the column that chooses rows for a rejudge, where
// only the rows in selectable can be chosen (useRejudgeSelection). showScore adds the score column of partially scored
// results (ADR 0025). A contest names each row's problem through the problem slot, which adds the 問題 column, and marks
// a row after its time through the after-time slot.
const props = defineProps<{ items: Submission[], detail: (item: Submission) => string, showScore: boolean, select: boolean, selectable: string[] }>()
const selected = defineModel<string[]>('selected', { required: true })
const allSelected = computed(() => props.selectable.length > 0 && props.selectable.every(id => selected.value.includes(id)))
</script>
<template>
  <div class="content-table-scroll" role="region" aria-label="提出一覧のスクロール領域" tabindex="0">
    <table class="content-table content-table--cards"><thead><tr><th v-if="select" scope="col" class="select-cell"><input type="checkbox" :checked="allSelected" :disabled="!selectable.length" aria-label="このページの提出をすべて選択" @change="selected = ($event.target as HTMLInputElement).checked ? [...selectable] : []"></th><th scope="col">提出日時 (JST)</th><th v-if="$slots.problem" scope="col">問題</th><th scope="col">ユーザー</th><th scope="col">言語</th><th scope="col">コード長</th><th scope="col" title="各テストケースの最大CPU時間・最大メモリ使用量">実行時間・メモリ</th><th v-if="showScore" scope="col">得点</th><th scope="col">結果</th><th scope="col">詳細</th></tr></thead>
      <tbody><tr v-for="item in items" :key="item.id"><td v-if="select" class="select-cell"><input v-model="selected" type="checkbox" :value="item.id" :disabled="!selectable.includes(item.id)" :aria-label="`${contestDate(item.createdAt)}の提出を選択`"></td><td class="corner-cell"><time :datetime="item.createdAt">{{ contestDate(item.createdAt) }}</time><slot name="after-time" :item="item" /></td><td v-if="$slots.problem" class="submission-problem title-cell"><slot name="problem" :item="item" /></td><td class="inline-cell"><UserLink :handle="item.author" /></td><td class="inline-cell">{{ runtimeLabel(item.runtime) }}</td><td class="submission-usage stat-cell wide-cell" data-label="コード長">{{ item.sourceBytes == null ? '—' : `${item.sourceBytes.toLocaleString('en-US')} bytes` }}</td><td class="submission-usage stat-cell wide-cell" data-label="実行時間・メモリ">{{ submissionUsage(item.result) }}</td><td v-if="showScore" class="submission-score stat-cell" data-label="得点" :data-full="(item.result?.scoring && item.result.scoring.score === item.result.scoring.maxScore) || undefined">{{ item.result?.scoring ? formatScore(item.result.scoring.score) : '—' }}</td><td class="lead-cell"><SubmissionStatus :item="item" /></td><td class="end-cell"><NuxtLink :to="detail(item)">詳細</NuxtLink></td></tr></tbody>
    </table>
  </div>
</template>
<style scoped>
.submission-usage { white-space: nowrap; font-variant-numeric: tabular-nums; }
.submission-problem { min-width: 200px; overflow-wrap: anywhere; }
.submission-score { text-align: right; white-space: nowrap; font-variant-numeric: tabular-nums; }
.submission-score[data-full] { color: var(--color-accent); font-weight: 600; }
.select-cell { width: 44px; text-align: center; }
.select-cell input { width: 18px; height: 18px; margin: 0; accent-color: var(--color-accent); cursor: pointer; }
.select-cell input:disabled { cursor: not-allowed; }
</style>
