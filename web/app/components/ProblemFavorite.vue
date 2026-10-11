<script setup lang="ts">
const props = defineProps<{ problemId: string, count: number }>()
const { user } = useAccount()
const count = ref(props.count)
const favorited = ref(false)
const loading = ref(false)
const ready = ref(false)
const message = ref('')
let requestVersion = 0
async function load() {
  const version = ++requestVersion
  ready.value = false
  message.value = ''
  favorited.value = false
  count.value = props.count
  if (!user.value) { loading.value = false; return }
  loading.value = true
  try {
    const result = await $fetch(`/api/my/favorites/${props.problemId}`)
    if (version !== requestVersion) return
    count.value = result.favoriteCount; favorited.value = result.favorited; ready.value = true
  } catch { if (version === requestVersion) message.value = 'お気に入りを取得できませんでした。' }
  finally { if (version === requestVersion) loading.value = false }
}
async function toggle() {
  if (!ready.value || loading.value) return
  const version = requestVersion
  loading.value = true; message.value = ''
  try {
    const result = await $fetch(`/api/my/favorites/${props.problemId}`, { method: 'PUT', body: { favorited: !favorited.value } })
    if (version !== requestVersion) return
    favorited.value = result.favorited; count.value = result.favoriteCount
  } catch { if (version === requestVersion) message.value = 'お気に入りを更新できませんでした。もう一度お試しください。' }
  finally { if (version === requestVersion) loading.value = false }
}
watch(() => user.value?.id, load)
onMounted(load)
onBeforeUnmount(() => { requestVersion++ })
</script>
<template>
  <div class="favorite-control">
    <button v-if="user" class="editor-button" :disabled="loading || !ready" :aria-pressed="favorited" @click="toggle"><span aria-hidden="true">{{ favorited ? '★' : '☆' }}</span> お気に入り {{ count }}</button>
    <NuxtLink v-else class="editor-button" :to="{ path: '/login', query: { next: `/problems/${problemId}` } }"><span aria-hidden="true">☆</span> お気に入り {{ count }}</NuxtLink>
    <LoadingSpinner v-if="loading" inline class="favorite-loading" />
    <p v-if="message" role="alert">{{ message }} <button v-if="!ready" class="editor-button" :disabled="loading" @click="load">再試行</button></p>
  </div>
</template>
<style scoped>
.favorite-control { text-align: right; margin-block: 16px; font-size: .875rem; }
.favorite-control button[aria-pressed="true"] { color: var(--color-accent); background: var(--color-accent-soft); }
.favorite-loading { margin-inline-start: 8px; }
</style>
