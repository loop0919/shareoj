<script setup lang="ts">
useSeoMeta({ title: 'ユーザーIDを登録 | ShareOJ', robots: 'noindex, nofollow' })
const { user } = useAccount()
</script>
<template>
  <section class="profile-page">
    <h1>ユーザーIDを登録</h1>
    <p class="muted">ShareOJで使うユーザーIDを決めてください。
      登録すると、<NuxtLink to="/terms">利用規約</NuxtLink>と<NuxtLink to="/privacy">プライバシーポリシー</NuxtLink>に同意したものとみなします。</p>
    <ProfileForm v-if="user" :initial="null" onboarding />
  </section>
</template>
<style scoped>
.profile-page { max-width: 36rem; margin: 48px auto 80px; }
</style>
