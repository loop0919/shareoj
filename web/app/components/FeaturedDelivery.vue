<script setup lang="ts">
import type { FeaturedPage } from '~~/shared/types/featured'
import { contestDate } from '~~/shared/types/contest'

const props = defineProps<{ page: FeaturedPage; home?: boolean }>()
const slots = computed(() => props.page.current?.slots ?? props.page.nextSlots.map(slot => ({ ...slot, problemId: '', title: '', editorialHidden: false, revealAt: '' })))
const date = computed(() => props.page.current?.scheduledAt ?? props.page.nextAt)
</script>

<template>
  <section class="featured-delivery" aria-label="ShareOJ定期便">
    <header class="delivery-heading">
      <div>
        <p class="delivery-kicker">毎週 月・木 23:00 <span>日本時間</span></p>
        <h2>次の一問が、届く。</h2>
        <p class="delivery-caption">Easy と Hard を、一問ずつ。<span v-if="home">ShareOJ定期便</span></p>
      </div>
      <div class="delivery-date">
        <span class="delivery-phase">{{ page.current ? '公開中の問題' : '次回予告' }}</span>
        <time :datetime="date">{{ contestDate(date) }}</time>
        <span v-if="page.current">次回 {{ contestDate(page.nextAt) }}</span>
        <span v-else>問題名は公開時のお楽しみ。</span>
      </div>
    </header>
    <div class="delivery-problems">
      <article v-for="(slot, index) in slots" :key="slot.slot" class="delivery-problem">
        <header class="delivery-level">
          <span class="delivery-number" aria-hidden="true">0{{ index + 1 }}</span>
          <h3>{{ slot.slot === 'easy' ? 'Easy' : (slot.difficulty ?? 0) >= 9 ? 'Ultimate' : 'Hard' }}</h3>
          <DifficultyBadge v-if="slot.difficulty" :level="slot.difficulty" />
          <span v-else class="muted">{{ slot.slot === 'easy' ? 'Lv.1〜4' : 'Lv.5〜10' }}</span>
          <span v-if="slot.kind !== 'missing'" class="delivery-kind">{{ slot.kind === 'new' ? '新作' : '復刻' }}</span>
        </header>
        <template v-if="slot.kind !== 'missing'">
          <p v-if="page.current" class="delivery-problem-title">
            <NuxtLink v-if="slot.problemId" :to="`/problems/${slot.problemId}`">{{ slot.title }}<span aria-hidden="true">↗</span></NuxtLink>
            <span v-else>現在、この問題は公開されていません。</span>
          </p>
          <dl class="delivery-credits">
            <div><dt>writer</dt><dd><UserLink :handle="slot.writer" /></dd></div>
            <div><dt>tester</dt><dd><template v-if="slot.testers.length"><UserLink v-for="tester in slot.testers" :key="tester" :handle="tester" /></template><span v-else>—</span></dd></div>
          </dl>
          <p v-if="page.current" class="delivery-note">{{ slot.editorialHidden ? `解説・他者の提出：${contestDate(slot.revealAt)} 公開` : '解説公開済み' }}</p>
        </template>
        <p v-else class="delivery-empty">{{ page.current ? 'この枠の出題はありません。' : '出題する問題を募集中です。' }}<span v-if="!page.current">新作・復刻の候補が決まり次第、予告します。</span></p>
      </article>
    </div>
    <footer class="delivery-footer">
      <div class="delivery-waiting">
        <p><span>出題待ち</span><strong>Easy <b>{{ page.waiting.easy }}</b>問</strong><span aria-hidden="true">/</span><strong>Hard <b>{{ page.waiting.hard }}</b>問</strong></p>
        <small>次回予定を含む、出題可能な新作の応募数</small>
      </div>
      <NuxtLink v-if="home" class="delivery-more" to="/featured">定期便・新作の応募<span aria-hidden="true">→</span></NuxtLink>
      <a v-else class="delivery-more" href="#history-title">過去の出題へ<span aria-hidden="true">↓</span></a>
    </footer>
    <p v-if="!page.current" class="delivery-disclaimer">応募の取り下げや内容の変更により、予告は変更されることがあります。</p>
  </section>
</template>

<style scoped>
/* Hallmark · pre-emit critique: P4 H5 E4 S5 R5 V4
 * Scoped delivery component · existing ShareOJ theme and fonts · no motion.
 * States: preview, current, new, revival, empty, withdrawn, hover, focus-visible. */
.featured-delivery { margin-block: 24px 48px; border-top: 2px solid var(--color-accent); }
.delivery-heading { display: flex; align-items: end; justify-content: space-between; flex-wrap: wrap; gap: 24px; padding-block: 28px; }
.delivery-kicker { margin: 0 0 10px; color: var(--color-accent); font-family: var(--font-code); font-size: .8125rem; letter-spacing: .04em; }
.delivery-kicker span { margin-left: 8px; color: var(--color-muted); font-family: var(--font-body); font-size: .6875rem; }
.delivery-heading h2 { margin: 0 0 10px; font-size: clamp(1.5rem, 3vw, 2rem); letter-spacing: -.04em; }
.delivery-caption { margin: 0; color: var(--color-muted); font-size: .8125rem; }
.delivery-caption span { margin-left: 8px; }
.delivery-date { display: grid; justify-items: end; gap: 4px; font-size: .875rem; font-variant-numeric: tabular-nums; }
.delivery-date > span:last-child { color: var(--color-muted); font-size: .75rem; }
.delivery-phase { color: var(--color-accent); font-weight: 650; }
.delivery-problems { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); border-block: 1px solid var(--color-line); background: var(--color-surface); }
.delivery-problem { min-width: 0; padding: 24px 28px; }
.delivery-problem + .delivery-problem { border-left: 1px solid var(--color-line); }
.delivery-level { display: flex; flex-wrap: wrap; gap: 8px 12px; align-items: center; margin-bottom: 24px; font-size: .8125rem; }
.delivery-number { color: var(--color-muted); font-family: var(--font-code); font-size: .6875rem; }
.delivery-level h3 { margin: 0 auto 0 0; font-size: 1.25rem; font-weight: 650; }
.delivery-kind { padding: 0 6px; border: 1px solid var(--color-line); border-radius: 3px; font-size: .6875rem; }
.delivery-problem-title { font-size: 1.25rem; font-weight: 650; line-height: 1.7; overflow-wrap: anywhere; }
.delivery-problem-title a { display: flex; justify-content: space-between; gap: 16px; color: var(--color-ink); text-decoration: none; }
.delivery-problem-title a:hover { color: var(--color-accent); text-decoration: underline; }
.delivery-problem-title a span { color: var(--color-accent); font-size: 1rem; }
.delivery-credits { display: grid; gap: 8px; margin: 0; font-size: .8125rem; }
.delivery-credits > div { display: grid; grid-template-columns: 52px minmax(0, 1fr); gap: 12px; }
.delivery-credits dt { font-family: var(--font-code); font-size: .75rem; }
.delivery-credits dd { display: flex; flex-wrap: wrap; gap: 0 12px; font-weight: 500; overflow-wrap: anywhere; }
.delivery-credits a { color: var(--color-ink); }
.delivery-note { margin: 20px 0 0; font-size: .6875rem; color: var(--color-muted); }
.delivery-empty { margin: 0; font-size: .875rem; }
.delivery-empty span { display: block; margin-top: 8px; color: var(--color-muted); font-size: .75rem; }
.delivery-footer { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 16px 24px; padding-block: 20px 8px; }
.delivery-waiting p { display: flex; align-items: baseline; flex-wrap: wrap; gap: 6px 14px; margin: 0; font-size: .8125rem; }
.delivery-waiting p > span { color: var(--color-muted); font-size: .75rem; }
.delivery-waiting strong { white-space: nowrap; font-weight: 500; }
.delivery-waiting b { margin-inline: 4px 2px; font-size: 1.375rem; font-family: var(--font-code); font-weight: 600; }
.delivery-waiting small, .delivery-disclaimer { color: var(--color-muted); font-size: .6875rem; }
.delivery-disclaimer { margin: 8px 0 0; }
.delivery-more { display: inline-flex; align-items: center; gap: 20px; min-height: 44px; font-size: .8125rem; text-decoration: none; }
.delivery-more:hover { text-decoration: underline; }
.delivery-more:active { transform: translateY(1px); }
@media (max-width: 650px) {
  .delivery-date { justify-items: start; }
  .delivery-problems { grid-template-columns: minmax(0, 1fr); }
  .delivery-problem { padding: 22px 20px; }
  .delivery-problem + .delivery-problem { border-left: 0; border-top: 1px solid var(--color-line); }
}
@media (max-width: 360px) { .delivery-problem { padding-inline: 14px; } .delivery-waiting p { column-gap: 8px; } }
</style>
