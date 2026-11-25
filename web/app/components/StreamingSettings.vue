<script setup lang="ts">
const { streaming, turnOn, askOff } = useStreamingMode()
// Turning off waits for the confirmation, so the switch's own checkbox is mounted again to match the mode.
const switchKey = ref(0)
function setStreaming(value: boolean) {
  if (value) turnOn()
  else { askOff(); switchKey.value++ }
}
</script>

<template>
  <section class="streaming-settings" aria-label="配信モード">
    <InlineSwitch :key="switchKey" :model-value="streaming" describedby="streaming-help" @update:model-value="setStreaming">このブラウザを配信モードにする</InlineSwitch>
    <ul id="streaming-help" class="hint">
      <li>配信モードのあいだ、このブラウザでは次のものを表示しません。マイページと作問・管理の画面、スタッフだけが見られる提出と凍結なしの順位表、通知。</li>
      <li>提出とコード実行は使えます。</li>
      <li>ほかのブラウザや端末には影響しません。</li>
    </ul>
  </section>
</template>

<style scoped>
.streaming-settings { display: flex; flex-direction: column; align-items: flex-start; gap: 8px; }
.hint { margin: 0; padding-inline-start: 1.25rem; color: var(--color-muted); font-size: .85rem; }
</style>
