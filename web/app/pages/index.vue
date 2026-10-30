<script setup lang="ts">
useResponseHeader('Cache-Control').value = 'no-store'
const { data: featured, error: featuredError, refresh: refreshFeatured } = await useFetch('/api/featured')
usePolling(() => refreshFeatured(), 60000)
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
// The answer path is always lit. Clicking another node toggles it, and an edge lights when both ends are lit.
const graphNodes = {
  start: { x: 64, y: 212, r: 22 }, route: { x: 284, y: 184, r: 11 }, goal: { x: 364, y: 96, r: 14 },
  upper: { x: 168, y: 104, r: 8 }, top: { x: 292, y: 72, r: 5 }, lower: { x: 184, y: 292, r: 8 }, bottom: { x: 344, y: 312, r: 5 },
}
type GraphNode = keyof typeof graphNodes
const answerNodes: GraphNode[] = ['start', 'route', 'goal']
const quietNodes: GraphNode[] = ['upper', 'top', 'lower', 'bottom']
const quietEdges: [GraphNode, GraphNode][] = [['start', 'upper'], ['upper', 'top'], ['upper', 'route'], ['start', 'lower'], ['lower', 'bottom'], ['lower', 'route']]
const quietPath = quietEdges.map(([a, b]) => `M${graphNodes[a].x} ${graphNodes[a].y} ${graphNodes[b].x} ${graphNodes[b].y}`).join('')
const litOrder = ref<GraphNode[]>([])
// 0 for the answer path, 1.. in click order, -1 while unlit.
const litRank = (node: GraphNode) => answerNodes.includes(node) ? 0 : litOrder.value.indexOf(node) + 1 || -1
const isLit = (node: GraphNode) => litRank(node) >= 0
// Each lit edge is traced from the end that lit up last.
const litEdges = computed(() => quietEdges.flatMap(([a, b]) => {
  if (!isLit(a) || !isLit(b)) return []
  const [from, to] = litRank(a) > litRank(b) ? [graphNodes[a], graphNodes[b]] : [graphNodes[b], graphNodes[a]]
  return [{ key: `${a}-${b}`, d: `M${from.x} ${from.y} ${to.x} ${to.y}` }]
}))
// The ripple remounts the dots so their animation restarts from the clicked node.
const ripple = ref<{ count: number, x: number, y: number }>()
const rippleDots = computed(() => gridDots.map((dot) => {
  const origin = ripple.value
  if (!origin) return { ...dot, className: 'grid-dot--intro', style: { '--wave-delay': dot.delay } }
  const distance = Math.hypot(dot.x - origin.x, dot.y - origin.y) || 1
  const strength = Math.max(0, 1 - distance / 320)
  if (!strength) return { ...dot, className: '', style: {} }
  return { ...dot, className: 'grid-dot--ripple', style: {
    '--ripple-delay': `${(distance / 400).toFixed(3)}s`,
    '--ripple-x': ((dot.x - origin.x) / distance * 4 * strength).toFixed(2),
    '--ripple-y': ((dot.y - origin.y) / distance * 4 * strength).toFixed(2),
    '--ripple-tint': `${Math.round(strength * 100)}%`,
  } }
}))
function touchNode(node: GraphNode) {
  if (!answerNodes.includes(node)) {
    litOrder.value = litOrder.value.includes(node) ? litOrder.value.filter(lit => lit !== node) : [...litOrder.value, node]
  }
  ripple.value = { count: (ripple.value?.count ?? 0) + 1, x: graphNodes[node].x, y: graphNodes[node].y }
}
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
          <g :key="ripple?.count ?? 0" aria-hidden="true">
            <circle
              v-for="(dot, index) in rippleDots" :key="index" :cx="dot.x" :cy="dot.y" r="1.25"
              class="grid-dot" :class="dot.className" :style="dot.style"
            />
          </g>
          <circle cx="236" cy="196" r="144" class="orbit" />
          <path :d="quietPath" class="quiet-path" />
          <path d="M64 212 284 184 364 96" class="quiet-path" />
          <path d="M64 212 284 184" pathLength="1" class="answer-path answer-path--first" />
          <path d="M284 184 364 96" pathLength="1" class="answer-path answer-path--second" />
          <path v-for="edge in litEdges" :key="edge.key" :d="edge.d" pathLength="1" class="answer-path answer-path--lit" />
          <g v-for="node in quietNodes" :key="node" class="graph-node" :class="{ 'graph-node--lit': isLit(node) }" @click="touchNode(node)">
            <circle v-if="isLit(node)" :cx="graphNodes[node].x" :cy="graphNodes[node].y" :r="graphNodes[node].r + 8" class="node-halo" />
            <circle :cx="graphNodes[node].x" :cy="graphNodes[node].y" :r="graphNodes[node].r" class="quiet-node" />
            <circle :cx="graphNodes[node].x" :cy="graphNodes[node].y" r="28" class="node-hit" />
          </g>
          <g class="graph-node" @click="touchNode('start')">
            <circle cx="64" cy="212" r="30" class="start-halo" />
            <circle cx="64" cy="212" r="22" class="start-node" />
            <text x="64" y="219" class="start-label" text-anchor="middle">?</text>
            <circle cx="64" cy="212" r="30" class="node-hit" />
          </g>
          <g class="graph-node" @click="touchNode('route')">
            <circle cx="284" cy="184" r="11" class="route-node" />
            <circle cx="284" cy="184" r="28" class="node-hit" />
          </g>
          <g class="graph-node" @click="touchNode('goal')">
            <circle cx="364" cy="96" r="25" class="end-halo" />
            <circle cx="364" cy="96" r="14" class="end-node" />
            <path d="m358 96 4 4 8-8" class="check" />
            <circle cx="364" cy="96" r="28" class="node-hit" />
          </g>
        </svg>
      </figure>
    </section>
    <FeaturedDelivery v-if="featured" :page="featured" home />
    <section v-else class="delivery-fallback" aria-label="ShareOJ定期便">
      <h2>ShareOJ定期便</h2>
      <p>毎週 月・木22時に、Easy と Hard を一問ずつ。</p>
      <p v-if="featuredError" role="alert">定期便を取得できませんでした。<button class="editor-button" @click="refreshFeatured()">再試行</button></p>
      <p v-else role="status">次の定期便を確認しています…</p>
      <NuxtLink to="/featured">定期便・新作の応募 →</NuxtLink>
    </section>
  </div>
</template>

<style scoped>
/* Hallmark · pre-emit critique: P4 H5 E4 S5 R5 V4
 * component: home hero · genre: modern-minimal · theme: existing ShareOJ tokens
 * Sequential path reveal with reduced-motion support; navigation states: default, hover, focus-visible, active.
 * Graph nodes are a pointer-only toy inside the decorative image, so they add no tab stops. */
.home { padding-bottom: 64px; }
.hero { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); align-items: center; gap: 24px; min-height: 480px; padding-block: 48px 56px; }
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
.hero-actions a:active { transform: translateY(1px); }
.hero-actions a:focus-visible { outline: 3px solid var(--color-accent); outline-offset: 5px; }
.hero-figure { margin: 0; min-width: 0; }
.path-art { display: block; width: 100%; height: auto; overflow: visible; }
.grid-dot { fill: var(--color-line); }
.orbit { fill: none; stroke: var(--color-line); stroke-dasharray: 3 7; }
.quiet-path { fill: none; stroke: var(--color-line); stroke-width: 2; }
.answer-path { fill: none; stroke: var(--color-accent); stroke-width: 3; stroke-linecap: round; stroke-linejoin: round; }
.quiet-node { fill: var(--color-paper); stroke: var(--color-muted); stroke-width: 1.5; }
.graph-node { cursor: pointer; user-select: none; -webkit-tap-highlight-color: transparent; }
.graph-node--lit .quiet-node { fill: var(--color-accent); stroke: var(--color-accent); }
.node-halo { fill: var(--color-accent-soft); }
.node-hit { fill: transparent; }
.start-halo { fill: var(--color-accent-soft); filter: blur(3px); }
.start-node { fill: var(--color-paper); stroke: var(--color-accent); stroke-width: 2; }
.start-label { fill: var(--color-accent); font: 600 22px var(--font-code); }
.route-node, .end-node { fill: var(--color-accent); }
.end-halo { fill: var(--color-accent-soft); }
.check { fill: none; stroke: var(--color-paper); stroke-width: 2; stroke-linecap: round; stroke-linejoin: round; }
@media (hover: hover) { .graph-node:hover .quiet-node { stroke: var(--color-accent); } }
.delivery-fallback { padding-block: 32px; border-top: 2px solid var(--color-accent); }
/* After the reveal, use the live theme colors instead of retaining animated fill/stroke values. */
@media (prefers-reduced-motion: no-preference) {
  .answer-path { stroke-dasharray: 1; animation: trace-path .8s cubic-bezier(.76, 0, .24, 1) .3s both; }
  .answer-path--second { animation-duration: .65s; animation-delay: 1.45s; }
  .answer-path--lit { animation: trace-path .45s ease-out both; }
  .quiet-node { transition: fill .2s ease-out, stroke .2s ease-out; }
  .node-halo { transform-box: fill-box; transform-origin: center; animation: bloom .4s ease-out both; }
  .start-node { animation: light-start .7s ease-in-out .5s backwards; }
  .start-label { animation: light-question .7s ease-in-out .5s backwards; }
  .start-halo { animation: reveal-arrival .8s ease-in-out .55s both; }
  .route-node { animation: light-node .18s ease-out 1.1s backwards; }
  .end-node { animation: light-node .18s ease-out 2.1s backwards; }
  .end-halo { animation: reveal-arrival .3s ease-out 2.1s both; }
  .check { animation: reveal-arrival .2s ease-out 2.25s both; }
  .grid-dot--intro { animation: grid-ripple 1.1s ease-in-out var(--wave-delay) backwards; }
  .grid-dot--ripple { animation: grid-push .9s ease-out var(--ripple-delay) backwards; }
}
@keyframes grid-ripple {
  0%, 100% { transform: translateY(0); fill: var(--color-line); }
  30% { transform: translateY(-5px); fill: var(--color-accent); }
  65% { transform: translateY(2px); fill: var(--color-line); }
}
/* The push and tint fade with distance from the clicked node. */
@keyframes grid-push {
  0%, 100% { transform: translate(0, 0); fill: var(--color-line); }
  30% { transform: translate(calc(var(--ripple-x) * 1px), calc(var(--ripple-y) * 1px)); fill: color-mix(in oklab, var(--color-accent) var(--ripple-tint), var(--color-line)); }
  65% { transform: translate(calc(var(--ripple-x) * -.4px), calc(var(--ripple-y) * -.4px)); fill: var(--color-line); }
}
@keyframes bloom { from { opacity: 0; transform: scale(.4); } }
@keyframes light-start { from { stroke: var(--color-muted); } to { stroke: var(--color-accent); } }
@keyframes light-question { from { fill: var(--color-muted); } to { fill: var(--color-accent); } }
@keyframes trace-path { from { stroke-dashoffset: 1; } to { stroke-dashoffset: 0; } }
@keyframes light-node { from { fill: var(--color-line); } to { fill: var(--color-accent); } }
@keyframes reveal-arrival { from { opacity: 0; } to { opacity: 1; } }
@media (max-width: 800px) {
  .hero { grid-template-columns: minmax(0, 1fr); gap: 24px; padding-block: 48px; min-height: 0; }
  .hero h1 { font-size: clamp(2rem, 6.5vw, 3.25rem); }
  .hero-figure { width: min(70%, 280px); margin-left: auto; }
}
@media (max-width: 400px) {
  .hero h1 { font-size: 1.875rem; }
  .hero-actions { gap: 12px 20px; }
  .hero-actions a { gap: 12px; }
  .hero-primary { padding-inline: 20px; }
}
</style>
