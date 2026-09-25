<script setup lang="ts">
import { contestDate } from '~~/shared/types/contest'
useResponseHeader('Cache-Control').value = 'no-store'
useSeoMeta({ title: '定期便 | ShareOJ', description: '毎週月曜・木曜23時（日本時間）にEasyとHardを一問ずつ出題します。新作の解説は翌22時に公開。' })
const offset = ref(0)
const { data, error, status, refresh } = await useFetch('/api/featured', { query: { offset } })
const dialog = ref<{ open: () => Promise<void> }>()
usePolling(() => refresh(), 60000)
</script>

<template>
  <div class="catalogue featured-page">
    <ProblemPostDialog ref="dialog" featured @posted="refresh()" />
    <header class="catalogue-heading"><h1>定期便</h1><button class="editor-button primary" @click="dialog?.open()">新作を応募</button></header>
    <FeaturedDelivery v-if="data" :page="data" />
    <p v-if="error" role="alert">定期便を取得できませんでした。<button class="editor-button" @click="refresh()">再試行</button></p>
    <section aria-labelledby="history-title" :aria-busy="status === 'pending'">
      <h2 id="history-title">{{ offset ? '過去の出題' : '最新・過去の出題' }}</h2>
      <p v-if="data && !data.items.length" class="muted">まだ出題されていません。次回の定期便をお待ちください。</p>
      <article v-for="round in data?.items" :key="round.scheduledAt" class="round">
        <h3><time :datetime="round.scheduledAt">{{ contestDate(round.scheduledAt) }}</time></h3>
        <div class="slots">
          <section v-for="slot in round.slots" :key="slot.slot" class="slot">
            <div class="slot-heading"><h4>{{ slot.slot === 'easy' ? 'Easy' : (slot.difficulty ?? 0) >= 9 ? 'Ultimate' : 'Hard' }}</h4><span>{{ slot.kind === 'new' ? '新作' : slot.kind === 'revival' ? '復刻' : '欠番' }}</span></div>
            <p v-if="slot.kind === 'missing'" class="muted">この枠の出題はありません。</p>
            <template v-else>
              <p v-if="slot.problemId"><NuxtLink :to="`/problems/${slot.problemId}`">{{ slot.title }}</NuxtLink> <DifficultyBadge :level="slot.difficulty" /></p>
              <p v-else class="muted">現在、この問題は公開されていません。</p>
              <p v-if="slot.editorialHidden" class="muted">解説・他者の提出：{{ contestDate(slot.revealAt) }} 公開</p>
              <p v-else class="muted">解説公開済み<template v-if="slot.problemId"> · <NuxtLink :to="{ path: `/problems/${slot.problemId}`, query: { view: 'editorial' } }">解説を読む</NuxtLink></template></p>
            </template>
          </section>
        </div>
      </article>
      <ContentPagination v-if="data" :index="offset / 20" :has-next="data.hasMore" :loading="status === 'pending'" @move="direction => offset += direction * 20" />
    </section>
  </div>
</template>

<style scoped>
.round { margin-block: 28px; }
.round h3 { font-size: 1rem; font-variant-numeric: tabular-nums; }
.slots { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16px; }
.slot { padding: 20px; border: 1px solid var(--color-line); border-radius: 4px; }
.slot-heading { display: flex; align-items: baseline; gap: 12px; }
.slot h4 { margin: 0; font-size: 1.125rem; }
.slot-heading span, .slot p { font-size: .875rem; }
.slot p { line-height: 1.8; overflow-wrap: anywhere; }
@media (max-width: 600px) { .slots { grid-template-columns: minmax(0, 1fr); } }
</style>
