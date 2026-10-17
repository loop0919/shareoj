<script setup lang="ts">
// Rejudging uses the saved problem, so unsaved edits block it instead of being silently ignored.
const props = defineProps<{ problemId: string, saved: boolean, disabled?: boolean }>()
const { pending, busy, message, refresh, rejudge } = useRejudge(() => props.problemId)
usePolling(refresh, 5000, () => pending.value > 0)
async function rejudgeAll() {
  if (!window.confirm('この問題への提出をすべてリジャッジしますか？\n保存済みの問題の内容で採点し直します。サンプル検証は対象外です。')) return
  await rejudge()
}
</script>

<template>
  <section class="management-row" aria-labelledby="rejudge-title">
    <div>
      <h2 id="rejudge-title">リジャッジ</h2>
      <p>テストケースや判定方法を直したあと、この問題への提出を保存済みの内容で採点し直します。コンテストでの提出も含みます。</p>
      <p v-if="pending" role="status">リジャッジ中：残り{{ pending.toLocaleString('en-US') }}件</p>
      <p v-if="!saved" class="muted">未保存の変更があります。保存してからリジャッジしてください。</p>
      <p v-if="message" role="status">{{ message }}</p>
    </div>
    <button type="button" class="editor-button" :disabled="!saved || busy || disabled" @click="rejudgeAll">{{ busy ? '依頼中…' : 'すべてリジャッジ' }}</button>
  </section>
</template>
