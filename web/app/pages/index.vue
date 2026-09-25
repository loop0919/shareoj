<script setup lang="ts">
const config = useRuntimeConfig()
const canonical = new URL('/', config.public.siteUrl).href
const title = 'ShareOJ — プログラミング問題を解く・作る・共有する'
const description = 'ShareOJ（Share Online Judge）は、プログラミング問題を解く・作る・共有するオンラインジャッジです。コードを提出して自動採点し、解き方や学んだことを記事で共有できます。コンテストの開催・参加もできます。'
// Each dot follows the wave according to its distance from the goal.
const gridDots = Array.from({ length: 17 * 15 }, (_, index) => {
  const x = 25 + (index % 17) * 24
  const y = 25 + Math.floor(index / 17) * 24
  return { x, y, delay: `${(2.55 + Math.hypot(x - 364, y - 96) / 480 * .9).toFixed(3)}s` }
})
useSeoMeta({ title, description, ogTitle: title, ogDescription: description, ogUrl: canonical, ogType: 'website' })
useHead({
  link: [{ rel: 'canonical', href: canonical }],
  script: [{ key: 'website', type: 'application/ld+json', textContent: JSON.stringify({
    '@context': 'https://schema.org', '@type': 'WebSite',
    name: 'ShareOJ', alternateName: 'Share Online Judge', url: canonical, description, inLanguage: 'ja',
  }).replace(/</g, '\\u003c') }],
})
useSharePreview({ type: 'website', title: 'Share Online Judge', path: '/', description })
</script>

<template>
  <div class="home">
    <section class="hero" aria-labelledby="hero-title">
      <div class="hero-copy">
        <p class="hero-kicker"><span aria-hidden="true" />SHARE ONLINE JUDGE</p>
        <h1 id="hero-title">考える楽しさを、<br><span>次の一問へ。</span></h1>
        <p class="hero-description">ひらめきを問題に。解き方を記事に。<br>ShareOJ は、プログラミング問題を解く・作る・共有するオンラインジャッジです。</p>
        <div class="hero-actions">
          <NuxtLink class="hero-primary" to="/problems">問題を見る<span aria-hidden="true">↗</span></NuxtLink>
          <NuxtLink class="hero-secondary" to="/problems/new?fresh=1">問題をつくる<span aria-hidden="true">→</span></NuxtLink>
        </div>
      </div>
      <figure class="hero-figure">
        <svg class="path-art" viewBox="0 0 440 400" role="img" aria-labelledby="path-title">
          <title id="path-title">複数のノードと、それらを結ぶ経路のグラフ</title>
          <g aria-hidden="true">
            <circle v-for="(dot, index) in gridDots" :key="index" :cx="dot.x" :cy="dot.y" r="1.25" class="grid-dot" :style="{ '--wave-delay': dot.delay }" />
          </g>
          <circle cx="236" cy="196" r="144" class="orbit" />
          <g class="quiet-path"><path d="M64 212 168 104 292 72M168 104 284 184M64 212 184 292 344 312M184 292 284 184" /></g>
          <path d="M64 212 284 184 364 96" class="quiet-path" />
          <path d="M64 212 284 184" pathLength="1" class="answer-path answer-path--first" />
          <path d="M284 184 364 96" pathLength="1" class="answer-path answer-path--second" />
          <g class="quiet-node"><circle cx="168" cy="104" r="8" /><circle cx="292" cy="72" r="5" /><circle cx="184" cy="292" r="8" /><circle cx="344" cy="312" r="5" /></g>
          <circle cx="64" cy="212" r="30" class="start-halo" />
          <circle cx="64" cy="212" r="22" class="start-node" />
          <text x="64" y="219" class="start-label" text-anchor="middle">?</text>
          <circle cx="284" cy="184" r="11" class="route-node" />
          <circle cx="364" cy="96" r="25" class="end-halo" />
          <circle cx="364" cy="96" r="14" class="end-node" />
          <path d="m358 96 4 4 8-8" class="check" />
        </svg>
      </figure>
    </section>
    <section class="regular-delivery" aria-labelledby="delivery-title">
      <div class="delivery-intro">
        <p class="delivery-kicker">毎週 月・木 23:00</p>
        <h2 id="delivery-title">ShareOJ定期便</h2>
        <p>Easy と Hard を、一問ずつ。新作を中心に、解くきっかけを届けます。</p>
      </div>
      <div class="delivery-slots" aria-label="出題枠">
        <div class="delivery-slot"><span class="slot-index">01</span><strong>Easy</strong><span>Lv.1〜4</span></div>
        <div class="delivery-slot"><span class="slot-index">02</span><strong>Hard</strong><span>Lv.5〜10</span></div>
      </div>
      <div class="delivery-foot">
        <p><span class="delivery-status">準備中</span>新作の解説は翌22時に公開します。</p>
        <NuxtLink to="/problems">公開中の問題を見る<span aria-hidden="true">→</span></NuxtLink>
      </div>
    </section>
  </div>
</template>

<style scoped>
/* Hallmark · pre-emit critique: P4 H5 E4 S5 R5 V4
 * component: home hero · genre: modern-minimal · theme: existing ShareOJ tokens
 * Sequential path reveal with reduced-motion support; navigation states: default, hover, focus-visible, active. */
.home { padding-bottom: 64px; }
.hero { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); align-items: center; gap: 24px; min-height: 600px; padding-block: 72px 80px; }
.hero-copy { position: relative; z-index: 1; }
.hero-kicker { display: flex; align-items: center; gap: 12px; margin-bottom: 28px; font-family: var(--font-code); font-size: .75rem; letter-spacing: .12em; color: var(--color-muted); }
.hero-kicker span { width: 8px; height: 8px; background: var(--color-accent); border-radius: 50%; }
.hero h1 { margin: 0 0 28px; font-size: clamp(2.25rem, 4.5vw, 3.5rem); line-height: 1.55; letter-spacing: -.055em; font-weight: 750; }
.hero h1 span { color: var(--color-accent); }
.hero-description { margin-bottom: 36px; font-size: .9375rem; line-height: 2.1; color: var(--color-muted); word-break: keep-all; overflow-wrap: anywhere; }
.hero-actions { display: flex; align-items: center; flex-wrap: wrap; gap: 16px 28px; }
.hero-actions a { display: inline-flex; align-items: center; justify-content: space-between; gap: 24px; min-height: 52px; font-size: .875rem; font-weight: 600; white-space: nowrap; text-decoration: none; }
.hero-primary { padding: 12px 24px; border-radius: 4px; background: var(--color-accent); color: var(--color-paper); }
.hero-primary:hover { background: var(--color-ink); }
.hero-secondary { color: var(--color-ink); }
.hero-secondary:hover { text-decoration: underline; text-underline-offset: 6px; }
.hero-actions a:active, .delivery-foot a:active { transform: translateY(1px); }
.hero-actions a:focus-visible, .delivery-foot a:focus-visible { outline: 3px solid var(--color-accent); outline-offset: 5px; }
.hero-figure { margin: 0; min-width: 0; }
.path-art { display: block; width: 100%; height: auto; overflow: visible; }
.grid-dot { fill: var(--color-line); }
.orbit { fill: none; stroke: var(--color-line); stroke-dasharray: 3 7; }
.quiet-path { fill: none; stroke: var(--color-line); stroke-width: 2; }
.answer-path { fill: none; stroke: var(--color-accent); stroke-width: 3; stroke-linecap: round; stroke-linejoin: round; }
.quiet-node { fill: var(--color-paper); stroke: var(--color-muted); stroke-width: 1.5; }
.start-halo { fill: var(--color-accent-soft); filter: blur(3px); }
.start-node { fill: var(--color-paper); stroke: var(--color-accent); stroke-width: 2; }
.start-label { fill: var(--color-accent); font: 600 22px var(--font-code); }
.route-node, .end-node { fill: var(--color-accent); }
.end-halo { fill: var(--color-accent-soft); }
.check { fill: none; stroke: var(--color-paper); stroke-width: 2; stroke-linecap: round; stroke-linejoin: round; }
/* Hallmark · pre-emit critique: P4 H4 E4 S5 R5 V4 · existing ShareOJ tokens */
.regular-delivery { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: 28px 40px; padding-block: 36px 28px; border-top: 1px solid var(--color-line); }
.delivery-kicker { margin: 0 0 8px; color: var(--color-accent); font-family: var(--font-code); font-size: .75rem; letter-spacing: .08em; }
.delivery-intro h2 { margin: 0 0 8px; font-size: clamp(1.375rem, 2.4vw, 1.75rem); }
.delivery-intro > p:last-child { max-width: 30rem; margin: 0; color: var(--color-muted); font-size: .875rem; line-height: 1.8; }
.delivery-slots { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; }
.delivery-slot { display: grid; gap: 2px; min-width: 0; padding: 16px 18px; border: 1px solid var(--color-line); border-radius: 4px; background: var(--color-surface); }
.slot-index { color: var(--color-muted); font-family: var(--font-code); font-size: .6875rem; }
.delivery-slot strong { font-size: 1.125rem; font-weight: 650; }
.delivery-slot > span:last-child { color: var(--color-muted); font-size: .75rem; }
.delivery-foot { grid-column: 1 / -1; display: flex; align-items: center; flex-wrap: wrap; gap: 12px 24px; padding-top: 18px; border-top: 1px solid var(--color-line); }
.delivery-foot p { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; margin: 0; color: var(--color-muted); font-size: .8125rem; }
.delivery-status { padding: 1px 8px; border: 1px solid var(--color-line); border-radius: 3px; color: var(--color-ink); white-space: nowrap; }
.delivery-foot a { display: inline-flex; align-items: center; gap: 12px; min-height: 44px; margin-left: auto; font-size: .8125rem; text-decoration: none; white-space: nowrap; }
.delivery-foot a:hover { text-decoration: underline; }
/* After the reveal, use the live theme colors instead of retaining animated fill/stroke values. */
@media (prefers-reduced-motion: no-preference) {
  .answer-path { stroke-dasharray: 1; animation: trace-path .8s cubic-bezier(.76, 0, .24, 1) .3s both; }
  .answer-path--second { animation-duration: .65s; animation-delay: 1.45s; }
  .start-node { animation: light-start .7s ease-in-out .5s backwards; }
  .start-label { animation: light-question .7s ease-in-out .5s backwards; }
  .start-halo { animation: reveal-arrival .8s ease-in-out .55s both; }
  .route-node { animation: light-node .18s ease-out 1.1s backwards; }
  .end-node { animation: light-node .18s ease-out 2.1s backwards; }
  .end-halo { animation: reveal-arrival .3s ease-out 2.1s both; }
  .check { animation: reveal-arrival .2s ease-out 2.25s both; }
  .grid-dot { animation: grid-ripple 1.1s ease-in-out var(--wave-delay) backwards; }
}
@keyframes grid-ripple {
  0%, 100% { transform: translateY(0); fill: var(--color-line); }
  30% { transform: translateY(-5px); fill: var(--color-accent); }
  65% { transform: translateY(2px); fill: var(--color-line); }
}
@keyframes light-start { from { stroke: var(--color-muted); } to { stroke: var(--color-accent); } }
@keyframes light-question { from { fill: var(--color-muted); } to { fill: var(--color-accent); } }
@keyframes trace-path { from { stroke-dashoffset: 1; } to { stroke-dashoffset: 0; } }
@keyframes light-node { from { fill: var(--color-line); } to { fill: var(--color-accent); } }
@keyframes reveal-arrival { from { opacity: 0; } to { opacity: 1; } }
@media (max-width: 800px) {
  .hero { grid-template-columns: minmax(0, 1fr); gap: 24px; padding-block: 48px; min-height: 0; }
  .hero h1 { font-size: clamp(2rem, 6.5vw, 3.25rem); }
  .hero-figure { width: min(100%, 400px); margin-left: auto; }
  .regular-delivery { grid-template-columns: minmax(0, 1fr); gap: 20px; }
  .delivery-foot { grid-column: 1; }
}
@media (max-width: 400px) {
  .delivery-slots { gap: 8px; }
  .delivery-slot { padding: 12px; }
  .delivery-foot a { margin-left: 0; }
  .hero h1 { font-size: 1.875rem; }
  .hero-actions { gap: 12px 20px; }
  .hero-actions a { gap: 12px; }
  .hero-primary { padding-inline: 20px; }
}
</style>
