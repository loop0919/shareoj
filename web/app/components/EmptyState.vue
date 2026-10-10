<script setup lang="ts">
defineProps<{ kind: 'problem' | 'testcase' | 'testing' | 'post' | 'contest' | 'submission' | 'event', title: string, description?: string }>()
</script>

<template>
  <div class="empty-state">
    <svg viewBox="0 0 96 96" aria-hidden="true">
      <circle class="backdrop" cx="48" cy="50" r="34" />
      <template v-if="kind === 'problem'">
        <path class="figure" d="M30 18H56L70 32V76A4 4 0 0 1 66 80H30A4 4 0 0 1 26 76V22A4 4 0 0 1 30 18Z" />
        <path class="detail" d="M56 18V28A4 4 0 0 0 60 32H70M41 50L35 56L41 62M55 50L61 56L55 62M50 47L46 65" />
      </template>
      <template v-else-if="kind === 'testcase'">
        <rect class="figure" x="19" y="18" width="36" height="46" rx="4" />
        <path class="detail" d="M27 30H47M27 38H41M27 46H44" />
        <rect class="figure" x="41" y="34" width="36" height="46" rx="4" />
        <path class="detail" d="M49 46H69M49 54H63M49 62H66" />
      </template>
      <template v-else-if="kind === 'testing'">
        <rect class="figure" x="27" y="22" width="42" height="58" rx="5" />
        <rect class="figure" x="38" y="16" width="20" height="11" rx="3" />
        <path class="detail" d="M37 53L45 61L60 45" />
      </template>
      <template v-else-if="kind === 'post'">
        <path class="figure" d="M26 18H50L62 30V76A4 4 0 0 1 58 80H26A4 4 0 0 1 22 76V22A4 4 0 0 1 26 18Z" />
        <path class="detail" d="M50 18V26A4 4 0 0 0 54 30H62M32 42H50M32 52H52M32 62H42" />
        <g transform="rotate(45 66 60)">
          <path class="figure" d="M61 38H71V70L66 78L61 70Z" />
          <path class="detail" d="M61 45H71M61 70H71" />
        </g>
      </template>
      <template v-else-if="kind === 'contest'">
        <path class="detail" d="M34 26H30A6 6 0 0 0 30 38H35M62 26H66A6 6 0 0 1 66 38H61M48 52V64" />
        <path class="figure" d="M34 22H62V38A14 14 0 0 1 34 38Z" />
        <rect class="figure" x="37" y="64" width="22" height="10" rx="2" />
        <path class="sparkle" d="M48 27Q48 33 54 33Q48 33 48 39Q48 33 42 33Q48 33 48 27Z" />
      </template>
      <template v-else-if="kind === 'submission'">
        <path class="figure" d="M18 46L76 22L58 78L45 55Z" />
        <path class="detail" d="M76 22L45 55M14 66H24M18 74H28" />
      </template>
      <template v-else>
        <path class="figure" d="M48 20A4 4 0 0 1 52 24V26A18 18 0 0 1 66 44V56L72 64H24L30 56V44A18 18 0 0 1 44 26V24A4 4 0 0 1 48 20Z" />
        <path class="detail" d="M42 64A6 6 0 0 0 54 64M73 32Q77 37 76 43M23 32Q19 37 20 43" />
      </template>
      <path class="sparkle twinkle" d="M14 16Q14 22 20 22Q14 22 14 28Q14 22 8 22Q14 22 14 16Z" />
      <path class="sparkle twinkle late" d="M84 67Q84 72 89 72Q84 72 84 77Q84 72 79 72Q84 72 84 67Z" />
      <circle class="sparkle" cx="84" cy="30" r="2.5" />
    </svg>
    <p class="empty-state-title">{{ title }}</p>
    <p v-if="description" class="empty-state-description">{{ description }}</p>
    <div v-if="$slots.default" class="empty-state-actions"><slot /></div>
  </div>
</template>

<style scoped>
.empty-state { display: flex; flex-direction: column; align-items: center; padding-block: 32px 48px; text-align: center; word-break: auto-phrase; }
svg { width: 120px; height: 120px; margin-bottom: 16px; overflow: visible; }
.backdrop { fill: var(--color-accent-soft); }
.figure, .detail { stroke: var(--color-accent); stroke-width: 2.5; stroke-linecap: round; stroke-linejoin: round; }
.figure { fill: var(--color-paper); }
.detail { fill: none; }
.sparkle { fill: var(--color-accent); }
.twinkle { transform-box: fill-box; transform-origin: center; animation: twinkle 3.2s ease-in-out infinite; }
.twinkle.late { animation-delay: -1.6s; }
@keyframes twinkle { 0%, 100% { opacity: 1; transform: scale(1); } 50% { opacity: .45; transform: scale(.7); } }
@media (prefers-reduced-motion: reduce) { .twinkle { animation: none; } }
@media (max-height: 32rem) { .empty-state { padding-block: 16px 24px; } svg { width: 72px; height: 72px; margin-bottom: 8px; } }
.empty-state-title { margin: 0; color: var(--color-ink); font-weight: 600; }
.empty-state-description { max-width: 36rem; margin: 4px 0 0; color: var(--color-muted); font-size: .8125rem; line-height: 1.7; }
.empty-state-actions { display: flex; flex-wrap: wrap; justify-content: center; gap: 8px; margin-top: 20px; }
</style>
