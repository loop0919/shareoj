<script setup lang="ts">
const { user, profile } = useAccount()
const confirmation = ref('')
const busy = ref(false)
const error = ref('')
async function remove() {
  if (busy.value || !profile.value || confirmation.value !== profile.value.handle) return
  busy.value = true; error.value = ''
  try {
    await $fetch('/api/my/account', { method: 'DELETE', body: { handle: confirmation.value } })
    user.value = null; profile.value = null
    await navigateTo('/')
  } catch (e) {
    const failure = e as { data?: { data?: { code?: string } } }
    const code = failure.data?.data?.code
    error.value = code === 'account_has_active_contest' ? '開催予定または開催中のコンテストがあるため削除できません。コンテストの終了後に削除してください。'
      : code === 'confirmation_mismatch' ? 'ユーザーIDが一致しません。'
        : '削除できませんでした。時間をおいて、もう一度お試しください。'
  } finally { busy.value = false }
}
</script>
<template>
  <section v-if="profile" class="account-deletion" aria-labelledby="deletion-title">
    <h2 id="deletion-title">アカウント削除</h2>
    <p>アカウントを削除すると、ログインできなくなります。この操作は取り消せません。</p>
    <h3>削除されるもの</h3>
    <ul>
      <li>プロフィール（ユーザーID・アイコン・外部サービス）とログイン情報</li>
      <li>公開していない問題・記事と、コンテストの下書き</li>
      <li>公開済みの問題・記事の、公開していない編集内容</li>
      <li>お気に入りと通知</li>
      <li>残る問題・記事・コンテストで使っていない画像</li>
    </ul>
    <h3>残るもの</h3>
    <p>次のものは、ほかのユーザーの順位表・提出一覧・問題ページから参照されているため残します。作成者や提出者は「退会したユーザー」と表示します。</p>
    <ul>
      <li>公開済みの問題・記事と、終了したコンテスト</li>
      <li>提出、コンテストへの参加登録、難易度の投票</li>
      <li>テスターとして担当した問題の記録</li>
    </ul>
    <p class="note">開催予定または開催中のコンテストがある間は削除できません。今のユーザーIDは、削除後にほかの人が使えるようになります。</p>
    <form @submit.prevent="remove">
      <label for="deletion-confirmation">確認のため、ユーザーID「{{ profile.handle }}」を入力してください</label>
      <input id="deletion-confirmation" v-model="confirmation" type="text" autocomplete="off" autocapitalize="none" spellcheck="false" :disabled="busy">
      <p v-if="error" class="editor-error" role="alert">{{ error }}</p>
      <button class="editor-button danger" type="submit" :disabled="busy || confirmation !== profile.handle" :aria-busy="busy">{{ busy ? '削除中…' : 'アカウントを削除' }}</button>
    </form>
  </section>
</template>
<style scoped>
h2 { font-size: 1.125rem; margin-bottom: 16px; }
h3 { font-size: 1rem; font-weight: 600; margin: 24px 0 8px; }
ul { margin: 0 0 16px; padding-left: 20px; }
li { margin-block: 4px; }
.note { color: var(--color-muted); font-size: .875rem; }
form { margin-top: 32px; padding-top: 24px; border-top: 1px solid var(--color-line); }
label { display: block; margin-bottom: 8px; font-weight: 600; }
input { width: 100%; border: 1px solid var(--color-line); border-radius: 4px; min-height: 44px; padding: 8px 12px; margin-bottom: 16px; font: inherit; color: var(--color-ink); background: var(--color-paper); }
</style>
