<script setup lang="ts">
useSeoMeta({ title: 'マイページ | ShareOJ', robots: 'noindex, nofollow' })
const { profile } = useAccount()
const route = useRoute()
const router = useRouter()
const activeContent = computed({
  get: () => ['problems', 'testing', 'posts', 'contests', 'submissions', 'events'].includes(String(route.query.tab)) ? String(route.query.tab) : 'problems',
  set: tab => { void router.replace({ query: { ...route.query, tab } }) },
})
const joined = computed(() => profile.value ? new Date(profile.value.createdAt).toLocaleDateString('ja-JP', { year: 'numeric', month: 'long' }) : '')
</script>
<template>
  <section v-if="profile" class="my-page">
    <header class="profile-header">
      <UserAvatar :handle="profile.handle" :avatar="profile.avatar" :size="88" />
      <div class="profile-identity"><p class="eyebrow">マイページ</p><h1>{{ profile.handle }}</h1><p class="muted">{{ joined }}に登録</p></div>
      <div class="profile-actions"><NuxtLink class="editor-button" :to="`/users/${encodeURIComponent(profile.handle)}`" title="ほかのユーザーから見えるページ">公開ページを見る</NuxtLink><NuxtLink class="editor-button" to="/my/settings">プロフィールを編集</NuxtLink></div>
    </header>
    <ProfileAccounts :accounts="profile.accounts" />
    <nav class="content-menu" aria-label="マイページのコンテンツ">
      <button :aria-pressed="activeContent === 'problems'" @click="activeContent = 'problems'">自分の問題</button>
      <button :aria-pressed="activeContent === 'testing'" @click="activeContent = 'testing'">テスト中の問題</button>
      <button :aria-pressed="activeContent === 'posts'" @click="activeContent = 'posts'">記事</button>
      <button :aria-pressed="activeContent === 'contests'" @click="activeContent = 'contests'">コンテスト</button>
      <button :aria-pressed="activeContent === 'submissions'" @click="activeContent = 'submissions'">提出履歴</button>
      <button :aria-pressed="activeContent === 'events'" @click="activeContent = 'events'">イベント</button>
    </nav>
    <SavedProblems v-show="activeContent === 'problems'" />
    <SavedProblems v-if="activeContent === 'testing'" testing />
    <SavedPosts v-show="activeContent === 'posts'" />
    <ContestList v-if="activeContent === 'contests'" mine />
    <SubmissionHistory v-if="activeContent === 'submissions'" embedded />
    <EventHistory v-if="activeContent === 'events'" />
  </section>
</template>
<style scoped src="../../assets/css/profile.css"></style>
