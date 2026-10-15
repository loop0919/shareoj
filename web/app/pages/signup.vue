<script setup lang="ts">
const { data: providers } = await useFetch<{ google: boolean }>('/api/auth/providers')
useSeoMeta({ title: 'アカウントを作成 | ShareOJ', robots: 'noindex, nofollow' })
const email = ref('')
const password = ref('')
const repeatedPassword = ref('')
const code = ref('')
const route = useRoute()
const stage = ref<'signup' | 'confirm' | 'complete'>(route.query.confirm === '1' ? 'confirm' : 'signup')
const busy = ref(false)
const error = ref('')
const notice = ref('')
const cooldown = ref(0)
let timer: ReturnType<typeof setInterval> | undefined
function startCooldown() {
  clearInterval(timer)
  cooldown.value = 60
  timer = setInterval(() => { cooldown.value--; if (cooldown.value <= 0) clearInterval(timer) }, 1000)
}
onBeforeUnmount(() => { clearInterval(timer); password.value = ''; repeatedPassword.value = '' })
function registrationFailure(cause: unknown) {
  const status = (cause as { response?: { status?: number } }).response?.status
  if (status === 429) { startCooldown(); return '試行回数の上限に達しました。しばらく待ってからお試しください。' }
  if (status === 400 && stage.value === 'confirm') return 'メールアドレスと確認コードを確認してください。期限切れの場合はコードを再送できます。確認済みの場合はログインしてください。'
  if (status === 400) return 'メールアドレスとパスワードの条件を確認してください。'
  return '処理に失敗しました。接続を確認して、もう一度お試しください。登録済みの場合は確認コードの入力またはログインへ進んでください。'
}
function openConfirmation() {
  password.value = ''; repeatedPassword.value = ''; code.value = ''; error.value = ''; notice.value = ''
  stage.value = 'confirm'
}
async function signup() {
  if (busy.value || cooldown.value) return
  error.value = ''; notice.value = ''
  if (password.value !== repeatedPassword.value) { error.value = 'パスワードが一致しません。'; return }
  if (password.value.length < 12 || !/[a-z]/.test(password.value) || !/[A-Z]/.test(password.value) || !/[0-9]/.test(password.value) || !/[\p{P}\p{S}]/u.test(password.value) || /\s/.test(password.value)) {
    error.value = 'パスワードは12文字以上で、英大文字・英小文字・数字・記号を含めてください。空白は使えません。'; return
  }
  busy.value = true
  try {
    email.value = email.value.trim().toLowerCase()
    const result = await $fetch<{ confirmed: boolean }>('/api/auth/signup', { method: 'POST', body: { email: email.value, password: password.value } })
    openConfirmation()
    stage.value = result.confirmed ? 'complete' : 'confirm'
    if (!result.confirmed) startCooldown()
  } catch (cause) { error.value = registrationFailure(cause) }
  finally { busy.value = false }
}
async function confirm() {
  if (busy.value) return
  busy.value = true; error.value = ''; notice.value = ''
  try {
    await $fetch('/api/auth/confirm-signup', { method: 'POST', body: { email: email.value.trim().toLowerCase(), code: code.value.trim() } })
    code.value = ''; stage.value = 'complete'
  } catch (cause) { error.value = registrationFailure(cause) }
  finally { busy.value = false }
}
async function resend() {
  if (busy.value || cooldown.value) return
  busy.value = true; error.value = ''; notice.value = ''
  try {
    await $fetch('/api/auth/resend-confirmation', { method: 'POST', body: { email: email.value.trim().toLowerCase() } })
    notice.value = '確認が必要なアカウントには、新しいコードを送信しました。迷惑メールフォルダーも確認してください。'
    startCooldown()
  } catch (cause) { error.value = registrationFailure(cause) }
  finally { busy.value = false }
}
</script>

<template>
  <section class="account-signup">
    <h1>{{ stage === 'complete' ? 'メールアドレスの確認が完了しました' : stage === 'confirm' ? 'メールアドレスを確認' : 'アカウントを作成' }}</h1>
    <p v-if="stage === 'signup'" class="muted">アカウントを作成すると、<NuxtLink to="/terms">利用規約</NuxtLink>と<NuxtLink to="/privacy">プライバシーポリシー</NuxtLink>に同意したものとみなします。</p>
    <p v-if="providers?.google"><GoogleLoginLink>Googleで登録・ログイン</GoogleLoginLink></p>
    <AuthDivider v-if="providers?.google && stage === 'signup'" />
    <template v-if="stage === 'signup'">
      <p class="muted">問題を保存して、別の端末でも編集できます。</p>
      <form @submit.prevent="signup">
        <label for="signup-email">メールアドレス</label>
        <input id="signup-email" v-model="email" type="email" maxlength="128" autocomplete="email" required :disabled="busy">
        <label for="signup-password">パスワード</label>
        <input id="signup-password" v-model="password" type="password" minlength="12" maxlength="256" autocomplete="new-password" aria-describedby="password-help" required :disabled="busy">
        <p id="password-help" class="muted">12文字以上で、英大文字・英小文字・数字・記号を含めてください。空白は使えません。</p>
        <label for="signup-repeat">パスワード（確認）</label>
        <input id="signup-repeat" v-model="repeatedPassword" type="password" maxlength="256" autocomplete="new-password" required :disabled="busy">
        <button class="editor-button primary" type="submit" :disabled="busy || cooldown > 0">{{ busy ? '送信中…' : cooldown > 0 ? `${cooldown}秒後に再試行` : '確認コードを送信' }}</button>
      </form>
      <button type="button" class="editor-button resume-button" :disabled="busy" @click="openConfirmation">確認コードをお持ちの方</button>
    </template>
    <template v-else-if="stage === 'confirm'">
      <p class="muted">メールに届いた確認コードを入力してください。登録済みの方はログインへ進んでください。</p>
      <form @submit.prevent="confirm">
        <label for="confirm-email">メールアドレス</label>
        <input id="confirm-email" v-model="email" type="email" maxlength="128" autocomplete="email" required :disabled="busy">
        <label for="confirmation-code">確認コード</label>
        <input id="confirmation-code" v-model="code" inputmode="numeric" maxlength="2048" autocomplete="one-time-code" required :disabled="busy">
        <button class="editor-button primary" type="submit" :disabled="busy">{{ busy ? '確認中…' : 'メールアドレスを確認' }}</button>
        <button class="editor-button" type="button" :disabled="busy || cooldown > 0 || !email.trim()" @click="resend">{{ cooldown > 0 ? `再送まで${cooldown}秒` : '確認コードを再送' }}</button>
      </form>
    </template>
    <p v-else>登録したメールアドレスとパスワードでログインしてください。</p>
    <p v-if="error" role="alert" class="editor-error">{{ error }}</p>
    <p v-if="notice" role="status">{{ notice }}</p>
    <p><NuxtLink to="/login">ログインへ進む</NuxtLink></p>
  </section>
</template>

<style scoped>
.account-signup { max-width: 28rem; margin: 3rem auto; padding: 0 1rem; }
h1 { font-size: 1.7rem; }
form { display: grid; gap: .8rem; margin-top: 1.5rem; }
input { min-width: 0; width: 100%; min-height: 2.75rem; padding: .5rem; border: 1px solid #aaa; border-radius: .25rem; font: inherit; }
#password-help { margin: 0; font-size: .875rem; }
.resume-button { margin-top: 1rem; }
</style>
