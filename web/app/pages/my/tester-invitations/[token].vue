<script setup lang="ts">
import { accountError } from '~/utils/account-problems'
useSeoMeta({ title: 'テスターへの招待 | ShareOJ', robots: 'noindex, nofollow', referrer: 'no-referrer' })
useResponseHeader('Cache-Control').value = 'no-store'
useResponseHeader('Referrer-Policy').value = 'no-referrer'
definePageMeta({ key: route => String(route.params.token) })
const route = useRoute()
const { profile } = useAccount()
type Invitation = { id: string, title: string, author: string, joined: boolean }
const invitation = ref<Invitation>()
const loading = ref(true)
const busy = ref(false)
const message = ref('')
async function load() {
  if (!profile.value) return
  loading.value = true; message.value = ''
  try { invitation.value = await $fetch<Invitation>(`/api/my/tester-invitations/${encodeURIComponent(String(route.params.token))}`) }
  catch { message.value = '招待リンクが無効か、問題が削除されています。接続を確認して、もう一度お試しください。' }
  finally { loading.value = false }
}
watch(profile, () => { void load() })
onMounted(() => { void load() })
async function accept() {
  if (busy.value) return
  busy.value = true; message.value = ''
  try {
    await $fetch(`/api/my/tester-invitations/${encodeURIComponent(String(route.params.token))}`, { method: 'POST' })
    await navigateTo('/my?tab=testing')
  } catch (error) { message.value = accountError(error) }
  finally { busy.value = false }
}
</script>
<template>
  <section class="tester-invitation">
    <h1>テスターへの招待</h1>
    <p v-if="loading" role="status">招待を確認しています…</p>
    <template v-else-if="invitation">
      <h2>{{ invitation.title.trim() || '無題の問題' }}</h2>
      <p>作成者 <UserLink :handle="invitation.author" /></p>
      <template v-if="invitation.joined"><p>この問題はすでに操作できます。</p><NuxtLink class="editor-button primary" :to="`/problems/new?problem=${invitation.id}`">問題を編集</NuxtLink></template>
      <template v-else>
        <p>テスターとして参加すると、非公開の問題文・解説・テストケースの閲覧や提出に加え、問題の編集・削除・リジャッジ・非公開に戻す操作、テスターリンクの発行、自分のコンテストへの登録ができます。公開と公開予約、定期便への応募は作者だけができます。</p>
        <p>参加した問題はマイページの「テスト中の問題」に表示されます。</p>
        <button class="editor-button primary" :disabled="busy" @click="accept">{{ busy ? '参加しています…' : '許可する' }}</button>
      </template>
    </template>
    <p v-if="message" role="alert" class="editor-error">{{ message }}</p>
    <button v-if="message && !invitation" class="editor-button" @click="load">再試行</button>
    <p><NuxtLink to="/my">マイページに戻る</NuxtLink></p>
  </section>
</template>
<style scoped>
.tester-invitation { max-width: 40rem; margin: 48px auto 80px; }
</style>
