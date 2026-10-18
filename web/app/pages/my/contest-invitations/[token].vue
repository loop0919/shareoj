<script setup lang="ts">
import type { SetterInvitation } from '~~/shared/types/contest'
import { shortId } from '~~/shared/utils/short-id'
import { accountError } from '~/utils/account-problems'
useSeoMeta({ title: '共同作成者への招待 | ShareOJ', robots: 'noindex, nofollow', referrer: 'no-referrer' })
useResponseHeader('Cache-Control').value = 'no-store'
useResponseHeader('Referrer-Policy').value = 'no-referrer'
definePageMeta({ key: route => String(route.params.token) })
const route = useRoute()
const { profile } = useAccount()
const invitation = ref<SetterInvitation>()
const loading = ref(true)
const busy = ref(false)
const message = ref('')
const endpoint = () => `/api/my/setter-invitations/${encodeURIComponent(String(route.params.token))}`
const missing = '招待リンクが無効か、作り直されています。コンテストの作成者に今のリンクを確認してください。'
async function load() {
  if (!profile.value) return
  loading.value = true; message.value = ''
  try { invitation.value = await $fetch<SetterInvitation>(endpoint()) }
  catch (error) { message.value = (error as { statusCode?: number }).statusCode === 404 ? missing : '招待を確認できませんでした。接続を確認して、もう一度お試しください。' }
  finally { loading.value = false }
}
watch(profile, () => { void load() })
onMounted(() => { void load() })
// The editor opens the draft or the contest; after the start it shows only コンテスト管理.
const editorPath = (id: string) => `/my/contests/${shortId(id)}`
async function accept() {
  if (busy.value || !invitation.value) return
  busy.value = true; message.value = ''
  try {
    const result = await $fetch<SetterInvitation>(endpoint(), { method: 'POST' })
    await navigateTo(editorPath(result.id))
  } catch (error) {
    const code = (error as { data?: { data?: { code?: string } } }).data?.data?.code
    message.value = code === 'contest_participant_locked'
      ? 'このコンテストに参加登録しているため、結果が確定するまで共同作成者に加われません。'
      : (error as { statusCode?: number }).statusCode === 404 ? missing : accountError(error)
  } finally { busy.value = false }
}
</script>
<template>
  <section class="setter-invitation">
    <h1>共同作成者への招待</h1>
    <p v-if="loading" role="status">招待を確認しています…</p>
    <template v-else-if="invitation">
      <h2>{{ invitation.title.trim() || '無題のコンテスト' }}</h2>
      <p class="muted">{{ invitation.published ? '公開済みのコンテスト' : '下書き（未公開）のコンテスト' }}</p>
      <p>オーナー <UserLink :handle="invitation.author" /></p>
      <template v-if="invitation.joined"><p>このコンテストの作成者として、すでに操作できます。</p><NuxtLink class="editor-button primary" :to="editorPath(invitation.id)">コンテストを開く</NuxtLink></template>
      <template v-else>
        <p>共同作成者になると、オーナーと同じように、開始前の設定の編集、下書きの保存と公開、問題の追加と削除、招待リンクの発行ができます。共同作成者を外すことと、下書きの削除は、オーナーだけができます。</p>
        <p>公開済みのコンテストに入っている問題は、結果が確定するまで、テスターと同じように編集やリジャッジができます。</p>
        <p>共同作成者はコンテストのスタッフなので、このコンテストに参加登録できません。抜けたあとも参加できません。加わると、オーナーに通知が届きます。</p>
        <button class="editor-button primary" :disabled="busy" :aria-busy="busy" @click="accept">{{ busy ? '加わっています…' : '共同作成者になる' }}</button>
      </template>
    </template>
    <p v-if="message" role="alert" class="editor-error">{{ message }}</p>
    <button v-if="message && !invitation" class="editor-button" @click="load">再試行</button>
    <p><NuxtLink to="/my?tab=contests">自分のコンテストに戻る</NuxtLink></p>
  </section>
</template>
<style scoped>
/* Hallmark · Workbench · Plain tokens, mirrors the tester invitation page */
.setter-invitation { max-width: 40rem; margin: 48px auto 80px; }
.setter-invitation > p:last-child { margin-top: 24px; }
</style>
