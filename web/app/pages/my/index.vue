<script setup lang="ts">
useSeoMeta({ title: 'マイページ | ShareOJ', robots: 'noindex, nofollow' })
const { profile } = useAccount()
const route = useRoute()
const router = useRouter()
// The folder view beta puts own problems, tested problems and posts in one tab. While it is off the page is as before.
const folderView = computed(() => !!profile.value?.folderView)
const activeContent = computed({
  get: () => {
    const tab = String(route.query.tab)
    // With the beta on, the old tabs' URLs open the library, which narrows itself to their kind; with it off, the
    // library's URL opens 自分の問題.
    if (folderView.value) return ['contests', 'events'].includes(tab) ? tab : 'library'
    return ['problems', 'testing', 'posts', 'contests', 'events'].includes(tab) ? tab : 'problems'
  },
  set: tab => { void router.replace({ query: { ...route.query, tab, status: undefined } }) },
})
// One status filter in the URL; only the shown tab reads it, and switching tabs clears it.
const statusFilter = computed({
  get: () => typeof route.query.status === 'string' ? route.query.status : '',
  set: status => { void router.replace({ query: { ...route.query, status: status || undefined } }) },
})
const tabStatus = (tab: string) => activeContent.value === tab ? statusFilter.value : ''
const joined = computed(() => profile.value ? new Date(profile.value.createdAt).toLocaleDateString('ja-JP', { year: 'numeric', month: 'long', timeZone: 'Asia/Tokyo' }) : '')

// The switch moves at once and the view follows once the API has saved it; a failed save puts the switch back.
const switchOn = ref(false)
watch(folderView, on => { switchOn.value = on }, { immediate: true })
const switching = ref(false)
const switchMessage = ref('')
const switchNote = useId()
async function switchFolderView(enabled: boolean) {
  if (switching.value || !profile.value) return
  switchOn.value = enabled
  switching.value = true
  switchMessage.value = ''
  try {
    const result = await $fetch<{ enabled: boolean }>('/api/my/folder-view', { method: 'PUT', body: { enabled } })
    if (profile.value) profile.value.folderView = result.enabled
    switchOn.value = result.enabled
  } catch {
    switchOn.value = !enabled
    switchMessage.value = '保存できませんでした。もう一度お試しください。何も変わっていません。'
  } finally { switching.value = false }
}
</script>
<template>
  <section v-if="profile" class="my-page">
    <header class="profile-header">
      <UserAvatar :handle="profile.handle" :avatar="profile.avatar" :size="88" />
      <div class="profile-identity"><p class="eyebrow">マイページ</p><h1>{{ profile.handle }}</h1><p class="muted">{{ joined }}に登録</p></div>
      <div class="profile-actions"><NuxtLink class="editor-button" :to="`/users/${encodeURIComponent(profile.handle)}`" title="ほかのユーザーから見えるページ">公開ページを見る</NuxtLink></div>
    </header>
    <ProfileAccounts :accounts="profile.accounts" />
    <div class="folder-view-switch">
      <InlineSwitch :model-value="switchOn" :describedby="switchNote" :disabled="switching" @update:model-value="switchFolderView">フォルダ表示（ベータ）</InlineSwitch>
      <p class="muted"><span :id="switchNote">自分の問題、テスト中の問題、記事を1つの一覧にまとめ、フォルダに分けて整理できます。</span><a href="https://github.com/loop0919/shareoj/issues/new?template=feature_request.yml" target="_blank" rel="noopener noreferrer">意見を送る ↗</a></p>
      <p v-if="switchMessage" role="alert" class="editor-error">{{ switchMessage }}</p>
    </div>
    <nav v-if="!folderView" class="content-menu" aria-label="マイページのコンテンツ">
      <button :aria-pressed="activeContent === 'problems'" @click="activeContent = 'problems'">自分の問題</button>
      <button :aria-pressed="activeContent === 'testing'" @click="activeContent = 'testing'">テスト中の問題</button>
      <button :aria-pressed="activeContent === 'posts'" @click="activeContent = 'posts'">記事</button>
      <button :aria-pressed="activeContent === 'contests'" @click="activeContent = 'contests'">コンテスト</button>
      <button :aria-pressed="activeContent === 'events'" @click="activeContent = 'events'">通知</button>
    </nav>
    <nav v-else class="content-menu" aria-label="マイページのコンテンツ">
      <button :aria-pressed="activeContent === 'library'" @click="activeContent = 'library'">問題と記事</button>
      <button :aria-pressed="activeContent === 'contests'" @click="activeContent = 'contests'">コンテスト</button>
      <button :aria-pressed="activeContent === 'events'" @click="activeContent = 'events'">通知</button>
    </nav>
    <template v-if="!folderView">
      <SavedProblems v-show="activeContent === 'problems'" :status="tabStatus('problems')" @update:status="statusFilter = $event" />
      <SavedProblems v-if="activeContent === 'testing'" v-model:status="statusFilter" testing />
      <SavedPosts v-show="activeContent === 'posts'" :status="tabStatus('posts')" @update:status="statusFilter = $event" />
    </template>
    <ContentLibrary v-else-if="activeContent === 'library'" />
    <ContestList v-if="activeContent === 'contests'" v-model:status="statusFilter" mine />
    <EventHistory v-if="activeContent === 'events'" />
  </section>
</template>
<style scoped src="../../assets/css/profile.css"></style>
<style scoped>
/* The beta's switch sits above the tabs, with a line on what it does and where to send feedback. */
.folder-view-switch { display: flex; flex-wrap: wrap; align-items: center; column-gap: 16px; margin-top: 24px; }
.folder-view-switch p { margin: 0; }
.folder-view-switch a { margin-left: 8px; white-space: nowrap; }
.folder-view-switch [role="alert"] { flex-basis: 100%; margin-top: 8px; }
</style>
