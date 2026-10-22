<script setup lang="ts">
const { settings, supported, permission, enable } = useBrowserNotifications()
const mounted = ref(false)
onMounted(() => { mounted.value = true })
const kinds = [
  ['clarifications', '質問と回答（全体公開を含む）'],
  ['contests', 'コンテストの開始と終了'],
  ['featured', '定期便の出題'],
  ['editorials', '定期便の解説公開'],
] as const
const receiving = computed(() => settings.value.enabled && permission.value === 'granted')
// A refused request leaves the switch's own checkbox ticked; a new key mounts it again to match the setting.
const switchKey = ref(0)
async function setReceiving(value: boolean) {
  if (value) {
    await enable()
    if (!receiving.value) switchKey.value++
  } else settings.value = { ...settings.value, enabled: false }
}
function setKind(kind: typeof kinds[number][0], value: boolean) {
  settings.value = { ...settings.value, kinds: { ...settings.value.kinds, [kind]: value } }
}
</script>
<template>
  <section class="notification-settings" aria-label="ブラウザの通知">
    <p v-if="mounted && !supported" class="hint">このブラウザでは通知を出せません。ベルで確かめてください。</p>
    <template v-else-if="mounted">
      <InlineSwitch :key="switchKey" :model-value="receiving" describedby="browser-notification-help" @update:model-value="setReceiving">このブラウザで通知を受け取る</InlineSwitch>
      <div id="browser-notification-help">
        <p class="hint">ShareOJを開いているあいだ、このブラウザに通知を出します。閉じているときは届きません。</p>
        <p class="hint">質問と回答と結果の公開は、ベルにも残ります。</p>
      </div>
      <p v-if="permission === 'denied'" class="hint denied">ブラウザで通知が拒否されています。ブラウザのサイトの設定で通知を許可してください。</p>
      <div v-else-if="receiving" class="kinds">
        <InlineSwitch v-for="[kind, label] in kinds" :key="kind" :model-value="settings.kinds[kind]" @update:model-value="setKind(kind, $event)">{{ label }}</InlineSwitch>
      </div>
    </template>
  </section>
</template>
<style scoped>
.notification-settings { display: flex; flex-direction: column; align-items: flex-start; gap: 8px; }
.hint { margin: 0; color: var(--color-muted); font-size: .85rem; }
.denied { color: var(--color-ink); }
.kinds { display: flex; flex-direction: column; align-items: flex-start; padding-top: 8px; }
</style>
