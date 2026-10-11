<script setup lang="ts">
const showProblemTags = useProblemTags()
</script>
<template>
  <section class="display-settings" aria-labelledby="problem-tags-title">
    <h2 id="problem-tags-title">問題のタグ</h2>
    <div class="switch-setting">
      <div>
        <p id="problem-tags-label" class="switch-label">問題のタグを表示する</p>
        <p id="problem-tags-help" class="hint">オンにすると、問題ページと問題一覧にタグを表示します。タグは解法のヒントになるため、初期設定では表示しません。定期便の開催中の問題とコンテストの問題には、この設定に関係なく表示しません。設定はこのブラウザに保存します。</p>
      </div>
      <span class="switch-control">
        <span class="switch-state" aria-hidden="true">{{ showProblemTags ? 'ON' : 'OFF' }}</span>
        <button type="button" role="switch" class="switch" aria-labelledby="problem-tags-label" aria-describedby="problem-tags-help" :aria-checked="showProblemTags" @click="showProblemTags = !showProblemTags" />
      </span>
    </div>
  </section>
</template>
<style scoped>
h2 { font-size: 1.125rem; margin-bottom: 16px; }
.switch-setting { display: flex; align-items: flex-start; justify-content: space-between; gap: 24px; }
.switch-label { margin: 0 0 4px; font-weight: 600; }
.hint { margin: 0; color: var(--color-muted); font-size: .85rem; }
.switch-control { display: inline-flex; align-items: center; gap: 8px; flex-shrink: 0; padding-top: 2px; }
.switch-state { min-width: 3ch; font-size: .75rem; color: var(--color-muted); text-align: end; }
.switch { position: relative; width: 44px; height: 24px; padding: 0; border: 1px solid var(--color-line); border-radius: 12px; background: var(--color-surface); cursor: pointer; }
.switch::after { content: ''; position: absolute; top: 2px; left: 2px; width: 18px; height: 18px; border-radius: 50%; background: var(--color-muted); transition: transform .15s; }
.switch[aria-checked="true"] { border-color: var(--color-accent); background: var(--color-accent); }
.switch[aria-checked="true"]::after { transform: translateX(20px); background: var(--color-paper); }
.switch:focus-visible { outline: 2px solid var(--color-accent); outline-offset: 2px; }
@media (prefers-reduced-motion: reduce) { .switch::after { transition: none; } }
</style>
