<script setup lang="ts">
import { accountSchema } from '~~/shared/types/profile'
const account = ref<{ email: string, provider: string } | null>(null)
const loadError = ref('')
const current = ref('')
const proposed = ref('')
const confirmation = ref('')
const busy = ref(false)
const error = ref('')
const done = ref(false)
const providerName = computed(() => account.value?.provider === 'google' ? 'Google アカウント' : 'メールアドレスとパスワード')
onMounted(async () => {
  try { account.value = accountSchema.parse(await $fetch('/api/my/account')) }
  catch { loadError.value = 'ログイン方法を取得できませんでした。ページを再読み込みしてください。' }
})
async function change() {
  if (busy.value) return
  error.value = ''; done.value = false
  if (proposed.value !== confirmation.value) { error.value = '確認用のパスワードが一致しません。'; return }
  busy.value = true
  try {
    await $fetch('/api/my/password', { method: 'POST', body: { current: current.value, proposed: proposed.value } })
    current.value = ''; proposed.value = ''; confirmation.value = ''; done.value = true
  } catch (e) {
    const failure = e as { data?: { data?: { code?: string } }, statusCode?: number }
    const code = failure.data?.data?.code
    error.value = code === 'incorrect_password' ? '現在のパスワードが正しくありません。'
      : code === 'invalid_password' ? 'パスワードは12文字以上で、英大文字・英小文字・数字・記号を含めてください。空白は使えません。'
        : failure.statusCode === 429 ? '試行回数が多すぎます。しばらく待ってから、もう一度お試しください。'
          : '変更できませんでした。もう一度お試しください。'
  } finally { busy.value = false }
}
</script>
<template>
  <div class="account-security">
    <section aria-labelledby="sign-in-title">
      <h2 id="sign-in-title">ログイン方法</h2>
      <p v-if="loadError" class="editor-error" role="alert">{{ loadError }}</p>
      <p v-else-if="!account" role="status">読み込み中…</p>
      <dl v-else class="sign-in">
        <div><dt>方法</dt><dd>{{ providerName }}</dd></div>
        <div><dt>メールアドレス</dt><dd>{{ account.email || '—' }}</dd></div>
      </dl>
    </section>
    <section v-if="account?.provider === 'password'" aria-labelledby="password-title">
      <h2 id="password-title">パスワードの変更</h2>
      <form @submit.prevent="change">
        <fieldset :disabled="busy">
          <label for="current-password">現在のパスワード</label>
          <input id="current-password" v-model="current" type="password" autocomplete="current-password" required maxlength="256">
          <label for="new-password">新しいパスワード</label>
          <input id="new-password" v-model="proposed" type="password" autocomplete="new-password" required minlength="12" maxlength="256" aria-describedby="new-password-help">
          <p id="new-password-help" class="hint">12文字以上で、英大文字・英小文字・数字・記号を含めてください。空白は使えません。</p>
          <label for="confirm-password">新しいパスワード（確認）</label>
          <input id="confirm-password" v-model="confirmation" type="password" autocomplete="new-password" required maxlength="256">
          <p v-if="error" class="editor-error" role="alert">{{ error }}</p>
          <button class="editor-button primary" type="submit" :aria-busy="busy">{{ busy ? '変更中…' : 'パスワードを変更' }}</button>
          <span v-if="done" class="saved-status" role="status">パスワードを変更しました</span>
        </fieldset>
      </form>
    </section>
    <p v-else-if="account" class="provider-note">Google アカウントでログインしているため、ShareOJ のパスワードはありません。パスワードは Google アカウントで管理してください。</p>
  </div>
</template>
<style scoped>
section + section { margin-top: 40px; }
h2 { font-size: 1.125rem; margin-bottom: 16px; }
fieldset { border: 0; padding: 0; margin: 0; min-width: 0; }
label { display: block; margin-bottom: 8px; font-weight: 600; }
input { width: 100%; border: 1px solid var(--color-line); border-radius: 4px; min-height: 44px; padding: 8px 12px; margin-bottom: 20px; font: inherit; color: var(--color-ink); background: var(--color-paper); }
.hint { margin: -12px 0 20px; color: var(--color-muted); font-size: .85rem; }
.sign-in { display: grid; gap: 12px; margin: 0; }
.sign-in div { display: grid; grid-template-columns: 9rem minmax(0, 1fr); gap: 16px; }
.sign-in dt { color: var(--color-muted); }
.sign-in dd { margin: 0; overflow-wrap: anywhere; }
.provider-note { margin-top: 24px; color: var(--color-muted); font-size: .875rem; }
.saved-status { margin-left: 16px; color: var(--color-muted); font-size: .875rem; }
@media (max-width: 480px) { .sign-in div { grid-template-columns: minmax(0, 1fr); gap: 2px; } }
</style>
