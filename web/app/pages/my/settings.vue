<script setup lang="ts">
useSeoMeta({ title: '設定 | ShareOJ', robots: 'noindex, nofollow' })
const { profile } = useAccount()
const route = useRoute()
const router = useRouter()
const tabs = [['identity', 'プロフィール'], ['services', '外部サービス'], ['security', '認証設定'], ['deletion', 'アカウント削除']] as const
type Tab = typeof tabs[number][0]
const tab = computed<Tab>({
  get: () => tabs.find(([key]) => key === route.query.tab)?.[0] ?? 'identity',
  set: value => { void router.replace({ query: { ...route.query, tab: value } }) },
})
</script>
<template>
  <section class="settings-page">
    <NuxtLink to="/my">マイページへ戻る</NuxtLink>
    <h1>設定</h1>
    <nav class="content-menu" aria-label="設定の項目">
      <button v-for="[key, label] in tabs" :key="key" :aria-pressed="tab === key" @click="tab = key">{{ label }}</button>
    </nav>
    <div v-if="profile" class="settings-body">
      <ProfileForm v-if="tab === 'identity'" :initial="profile" part="identity" />
      <ProfileForm v-else-if="tab === 'services'" :initial="profile" part="services" />
      <AccountSecurity v-else-if="tab === 'security'" />
      <AccountDeletion v-else />
    </div>
  </section>
</template>
<style scoped src="../../assets/css/profile.css"></style>
<style scoped>
.settings-page { max-width: 40rem; margin: 48px auto 80px; }
.settings-page h1 { margin-top: 16px; }
.settings-body { padding-top: 32px; }
</style>
