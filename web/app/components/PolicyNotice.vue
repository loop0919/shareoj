<script setup lang="ts">
// Change both when the terms or privacy policy change, so users registered before the change see the notice again.
const noticeId = '2026-10-16'
// Users registered from this time agreed to the documents on signup.
const effectiveAt = new Date('2026-10-16T00:00:00+09:00')
const storageKey = 'openoj.policy-notice'
const { profile } = useAccount()
// Hidden until mounted: the server knows neither the profile nor what this browser dismissed.
const dismissed = ref(true)
onMounted(() => {
  try { dismissed.value = localStorage.getItem(storageKey) === noticeId }
  catch { dismissed.value = false }
})
const visible = computed(() => !dismissed.value && !!profile.value && new Date(profile.value.createdAt) < effectiveAt)
function dismiss() {
  dismissed.value = true
  try { localStorage.setItem(storageKey, noticeId) }
  catch { /* The notice returns on the next visit. */ }
}
</script>

<template>
  <section v-if="visible" class="policy-notice" aria-label="利用規約とプライバシーポリシーのお知らせ">
    <p>2026年10月16日に<NuxtLink to="/terms">利用規約</NuxtLink>と<NuxtLink to="/privacy">プライバシーポリシー</NuxtLink>を定めました。
      今後のご利用に適用されますので、ご確認ください。</p>
    <button class="editor-button" type="button" @click="dismiss">閉じる</button>
  </section>
</template>

<style scoped>
.policy-notice { display: flex; border-inline-start: 3px solid var(--color-accent); padding: 16px 20px; background: var(--color-surface); font-size: .875rem; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 8px 16px; margin-top: 16px; }
.policy-notice p { flex: 1 1 20rem; margin: 0; color: var(--color-ink); line-height: 1.7; overflow-wrap: anywhere; }
</style>
