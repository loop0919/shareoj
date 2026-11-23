<script setup lang="ts">
useSeoMeta({ title: '設定 | ShareOJ', robots: 'noindex, nofollow' })
const { profile } = useAccount()
const route = useRoute()
const router = useRouter()
const { streaming } = useStreamingMode()
const allTabs = [['identity', 'プロフィール'], ['services', '外部サービス'], ['security', '認証設定'], ['notifications', 'プッシュ通知'], ['streaming', '配信モード'], ['deletion', 'アカウント削除']] as const
type Tab = typeof allTabs[number][0]
// Streaming mode leaves only its own item, whatever ?tab= asks for.
const tabs = computed(() => streaming.value ? allTabs.filter(([key]) => key === 'streaming') : allTabs)
const tab = computed<Tab>({
  get: () => streaming.value ? 'streaming' : allTabs.find(([key]) => key === route.query.tab)?.[0] ?? 'identity',
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
    <StreamingSettings v-if="tab === 'streaming'" class="settings-body" />
    <div v-else-if="profile" class="settings-body">
      <ProfileForm v-if="tab === 'identity'" :initial="profile" part="identity" />
      <ProfileForm v-else-if="tab === 'services'" :initial="profile" part="services" />
      <AccountSecurity v-else-if="tab === 'security'" />
      <BrowserNotificationSettings v-else-if="tab === 'notifications'" />
      <AccountDeletion v-else />
    </div>
  </section>
</template>
<style scoped src="../../assets/css/profile.css"></style>
<style scoped>
.settings-page { margin-block: 40px 80px; }
.settings-page h1 { margin-top: 16px; }
/* The page spans the usual width; forms keep a readable line length. */
.settings-body { max-width: 40rem; padding-top: 32px; }
</style>
