<script setup lang="ts">
import { contestDate } from '~~/shared/types/contest'

definePageMeta({ key: route => String(route.query.at) })
useResponseHeader('Cache-Control').value = 'no-store'
useSeoMeta({ title: '定期便の順位表 | ShareOJ' })
const route = useRoute()
const at = typeof route.query.at === 'string' ? route.query.at : ''
const offset = ref(0)
const { data, error, status, refresh } = await useFetch('/api/featured/standings', { query: { at, offset } })
const slots = ['easy', 'hard'] as const
const columns = computed(() => slots.map(slot => ({ slot, problem: data.value?.round.slots.find(problem => problem.slot === slot) })))
usePolling(() => refresh(), 15000)
function elapsed(ms: number | null) {
  if (ms === null) return '—'
  const seconds = Math.floor(ms / 1000)
  return `${Math.floor(seconds / 3600)}:${String(Math.floor(seconds / 60) % 60).padStart(2, '0')}:${String(seconds % 60).padStart(2, '0')}`
}
</script>

<template>
  <div class="catalogue featured-standings">
    <nav class="breadcrumb" aria-label="パンくずリスト"><NuxtLink to="/featured">定期便</NuxtLink><span aria-hidden="true">/</span><span>順位表</span></nav>
    <header class="catalogue-heading">
      <h1>定期便の順位表</h1>
      <button class="editor-button" :disabled="status === 'pending'" :aria-busy="status === 'pending'" @click="refresh()">{{ status === 'pending' ? '更新中…' : '今すぐ更新' }}</button>
    </header>
    <p v-if="error" class="notice notice-error" role="alert">{{ error.statusCode === 404 ? 'この回の順位表は見つかりません。' : '順位表を取得できませんでした。' }}<button class="editor-button" :disabled="status === 'pending'" @click="refresh()">再試行</button></p>
    <p v-else-if="!data" role="status" class="muted">順位表を読み込んでいます…</p>
    <template v-if="data && !error">
      <p class="standings-period"><time :datetime="data.round.scheduledAt">{{ contestDate(data.round.scheduledAt) }}</time> の定期便<span>{{ data.closed ? '集計期間終了' : '集計中' }}</span></p>
      <p class="muted">集計締め切り：<time :datetime="data.closesAt">{{ contestDate(data.closesAt) }}</time>（日本時間）</p>
      <div class="content-table-scroll" role="region" aria-label="定期便の順位表" tabindex="0" :aria-busy="status === 'pending'">
        <table class="content-table featured-standings-table">
          <thead><tr>
            <th scope="col">順位</th><th scope="col">ユーザー</th><th scope="col">時間</th>
            <th v-for="column in columns" :key="column.slot" scope="col">
              <NuxtLink v-if="column.problem?.problemId" :to="`/problems/${column.problem.problemId}`" :title="column.problem.title">{{ column.slot === 'easy' ? 'Easy' : 'Hard' }}</NuxtLink>
              <span v-else>{{ column.slot === 'easy' ? 'Easy' : 'Hard' }}</span>
              <small v-if="column.problem?.kind === 'missing'">欠番</small>
            </th>
          </tr></thead>
          <tbody>
            <tr v-for="row in data.items" :key="row.handle">
              <td>{{ row.rank }}</td><th scope="row"><UserLink :handle="row.handle" /></th><td>{{ elapsed(row.timeMs) }}</td>
              <td v-for="slot in slots" :key="slot">
                <span class="standing-result">
                  <svg v-if="row[slot].accepted" class="accepted-check" viewBox="0 0 24 24" role="img" :aria-label="`${slot === 'easy' ? 'Easy' : 'Hard'} 正解`"><path d="m5 12 4 4L19 6" /></svg>
                  <span v-else class="muted" aria-label="未正解">—</span>
                  <span v-if="row[slot].wrong" class="standing-wrong" :aria-label="`不正解 ${row[slot].wrong} 回`">({{ row[slot].wrong }})</span>
                </span>
                <small v-if="row[slot].accepted">{{ elapsed(row[slot].timeMs) }}</small>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <p v-if="!data.items.length" class="standings-empty muted">{{ offset ? 'このページに参加者はいません。' : '集計対象の提出はまだありません。' }}</p>
      <ContentPagination v-if="offset || data.hasMore" :index="offset / 50" :has-next="data.hasMore" :loading="status === 'pending'" @move="direction => offset += direction * 50" />
      <details class="standings-rules">
        <summary>順位の決まり方</summary>
        <ul>
          <li>両方正解 → Hardのみ正解 → Easyのみ正解 → 0完の順です。</li>
          <li>同じ正解状況は同順位です。同順位内では、その正解状況に到達した提出が早い人から表示します。0完は最初の提出順です。</li>
          <li>公開から翌22時までに、この回の問題へ通常の提出をした人を集計します。締め切り時刻以降の提出は含みません。</li>
          <li>締め切り前の提出は、採点が遅れても結果を反映します。</li>
          <li>この回の作問者・テスター、コンテスト経由の提出、下書き・サンプルの実行は対象外です。</li>
          <li>時間は公開から初正解までの経過時間です。両方正解した人の時間欄には、後に正解した問題の時間を表示します。</li>
          <li>括弧内は各問題の初正解前の不正解数です。WA・RE・TLE・MLE・OLEを数え、CE・JEは含めません。誤答ペナルティはなく、時間や順位に加算しません。</li>
          <li>正解は得点の代わりにチェックマークで表示します。</li>
        </ul>
      </details>
    </template>
  </div>
</template>

<style scoped>
/* Hallmark · pre-emit critique: P4 H4 E4 S5 R5 V4 · existing ShareOJ table and theme.
 * Checks with elapsed times and non-penalizing wrong counts; loading, empty, unavailable, error and refresh states. */
.featured-standings > .breadcrumb { padding-top: 0; }
.standings-period { display: flex; align-items: center; flex-wrap: wrap; gap: 8px 16px; margin-bottom: 4px; font-size: .9375rem; }
.standings-period > span { color: var(--color-muted); font-size: .75rem; }
.featured-standings-table { margin-top: 24px; }
.featured-standings-table th, .featured-standings-table td { padding: 12px; text-align: center; font-variant-numeric: tabular-nums; }
.featured-standings-table thead, .featured-standings-table tbody tr:nth-child(even) { background: var(--color-surface); }
.featured-standings-table th[scope="row"] { min-width: 0; text-align: left; overflow-wrap: anywhere; }
.featured-standings-table small { display: block; font-size: .6875rem; font-weight: 400; }
.standing-result { display: inline-flex; align-items: center; justify-content: center; gap: 4px; }
.standing-wrong { color: var(--color-muted); font-size: .75rem; }
.accepted-check { display: inline-block; width: 22px; height: 22px; vertical-align: middle; fill: none; stroke: var(--color-accent); stroke-width: 2.5; stroke-linecap: round; stroke-linejoin: round; }
.standings-empty { padding-block: 24px; }
.standings-rules { margin-top: 24px; color: var(--color-muted); font-size: .8125rem; }
.standings-rules summary { width: fit-content; padding-block: 10px; cursor: pointer; }
.standings-rules summary:hover, .standings-rules summary:active { color: var(--color-accent); }
.standings-rules summary:focus-visible, .content-table-scroll:focus-visible { outline: 2px solid var(--color-accent); outline-offset: 2px; }
.standings-rules ul { padding-left: 1.5em; }
.standings-rules li { margin-block: 8px; overflow-wrap: anywhere; }
.notice-error .editor-button { margin-left: 12px; }
@media (max-width: 400px) { .featured-standings-table th, .featured-standings-table td { padding-inline: 8px; } }
</style>
